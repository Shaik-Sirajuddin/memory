package store

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Cache matches the Redis-style cache contract described in the rule-merger docs.
type Cache interface {
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error)
	Del(ctx context.Context, key string) error
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
}

// FileUpdatedEvent is the merger event shape the store subscribes to.
type FileUpdatedEvent struct {
	FileID         string
	ModifiedSeqNos []int
}

// RuleMerger is the minimal merger contract the store relies on.
type RuleMerger interface {
	Enqueue(ctx context.Context, fileID string) error
	Subscribe() chan FileUpdatedEvent
	Unsubscribe(ch chan FileUpdatedEvent)
}

type fileWriteOp struct {
	seqno  int
	action string
	rule   string
	ts     int64
}

type subscriptionRecord struct {
	id         string
	target     string
	sessionID  string
	ignoreSelf bool
	ch         chan StoreEvent
}

// FileSystem implements Store on top of a Linux filesystem.
type FileSystem struct {
	rootDir string
	cache   Cache
	merger  RuleMerger

	mu              sync.RWMutex
	subscriptions   map[string]*subscriptionRecord
	originByFile    map[string]StoreEvent
	mergerEvents    chan FileUpdatedEvent
	done            chan struct{}
	closed          uint32
	closeOnce       sync.Once
	wg              sync.WaitGroup
	subID           uint64
	seqBySessionKey sync.Map
}

// NewFileSystem creates a filesystem-backed store.
func NewFileSystem(rootDir string, cache Cache, merger RuleMerger) (*FileSystem, error) {
	if strings.TrimSpace(rootDir) == "" {
		rootDir = "."
	}
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absRoot, 0o755); err != nil {
		return nil, err
	}

	fs := &FileSystem{
		rootDir:       absRoot,
		cache:         cache,
		merger:        merger,
		subscriptions: make(map[string]*subscriptionRecord),
		originByFile:  make(map[string]StoreEvent),
		done:          make(chan struct{}),
	}
	if merger != nil {
		fs.mergerEvents = merger.Subscribe()
		fs.wg.Add(1)
		go fs.consumeMergerEvents()
	}
	return fs, nil
}

// Close stops the background fan-out loop and closes subscriber channels.
func (f *FileSystem) Close() error {
	f.closeOnce.Do(func() {
		atomic.StoreUint32(&f.closed, 1)
		close(f.done)
		if f.merger != nil && f.mergerEvents != nil {
			f.merger.Unsubscribe(f.mergerEvents)
		}
		f.wg.Wait()

		f.mu.Lock()
		for id, sub := range f.subscriptions {
			close(sub.ch)
			delete(f.subscriptions, id)
		}
		f.mu.Unlock()
	})
	return nil
}

func (f *FileSystem) consumeMergerEvents() {
	defer f.wg.Done()
	for {
		select {
		case <-f.done:
			return
		case evt, ok := <-f.mergerEvents:
			if !ok {
				return
			}
			f.dispatchMergedEvent(evt)
		}
	}
}

func (f *FileSystem) dispatchMergedEvent(evt FileUpdatedEvent) {
	f.mu.Lock()
	origin := f.originByFile[evt.FileID]
	delete(f.originByFile, evt.FileID)
	subs := make([]*subscriptionRecord, 0, len(f.subscriptions))
	for _, sub := range f.subscriptions {
		subs = append(subs, sub)
	}
	f.mu.Unlock()

	storeEvt := StoreEvent{
		FileID:          evt.FileID,
		ModifiedSeqNos:  append([]int(nil), evt.ModifiedSeqNos...),
		Operation:       origin.Operation,
		OriginSessionID: origin.OriginSessionID,
	}

	for _, sub := range subs {
		if !sub.matches(storeEvt) {
			continue
		}
		select {
		case sub.ch <- storeEvt:
		default:
		}
	}
}

func (s *subscriptionRecord) matches(evt StoreEvent) bool {
	if s.ignoreSelf && s.sessionID != "" && s.sessionID == evt.OriginSessionID {
		return false
	}
	if s.target == "" {
		return true
	}
	return evt.FileID == s.target || strings.HasPrefix(evt.FileID, s.target+string(os.PathSeparator))
}

func (f *FileSystem) recordOrigin(fileID, sessionID, operation string) {
	if f == nil {
		return
	}
	f.mu.Lock()
	f.originByFile[fileID] = StoreEvent{
		FileID:          fileID,
		Operation:       operation,
		OriginSessionID: sessionID,
	}
	f.mu.Unlock()
}

func (f *FileSystem) nextSequence(ctx context.Context, sessionID string) int64 {
	if strings.TrimSpace(sessionID) == "" {
		return time.Now().UnixMilli()
	}

	if f.cache != nil {
		key := "store:sequence:" + sessionID
		raw, err := f.cache.Get(ctx, key)
		if err == nil {
			if n, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil {
				next := n + 1
				_ = f.cache.Set(ctx, key, strconv.FormatInt(next, 10), 24*time.Hour)
				return next
			}
		}
		seq := time.Now().UnixMilli()
		_ = f.cache.Set(ctx, key, strconv.FormatInt(seq, 10), 24*time.Hour)
		return seq
	}

	key := sessionID
	if current, ok := f.seqBySessionKey.Load(key); ok {
		if n, ok := current.(int64); ok {
			next := n + 1
			f.seqBySessionKey.Store(key, next)
			return next
		}
	}
	seq := time.Now().UnixMilli()
	f.seqBySessionKey.Store(key, seq)
	return seq
}

func (f *FileSystem) resolveMainPath(id InstructionID) (string, error) {
	account := strings.TrimSpace(id.AccountPrefix)
	if account == "" {
		account = "u_default"
	}
	bucket := strings.TrimSpace(id.Bucket)
	if bucket == "" {
		bucket = "default"
	}

	parts := []string{f.rootDir, sanitizePathPart(account), sanitizePathPart(bucket)}
	if pathParts, err := safeRelativeParts(id.Path); err != nil {
		return "", err
	} else {
		parts = append(parts, pathParts...)
	}
	if name := strings.TrimSpace(id.FileName); name != "" {
		parts = append(parts, sanitizePathPart(name))
	}

	if len(parts) == 3 {
		return "", errors.New("instruction path is empty")
	}
	return filepath.Join(parts...), nil
}

func (f *FileSystem) resolveFolderPath(id InstructionID) (string, error) {
	account := strings.TrimSpace(id.AccountPrefix)
	if account == "" {
		account = "u_default"
	}
	bucket := strings.TrimSpace(id.Bucket)
	if bucket == "" {
		bucket = "default"
	}

	parts := []string{f.rootDir, sanitizePathPart(account), sanitizePathPart(bucket)}
	if pathParts, err := safeRelativeParts(id.Path); err != nil {
		return "", err
	} else {
		parts = append(parts, pathParts...)
	}
	return filepath.Join(parts...), nil
}

func safeRelativeParts(p string) ([]string, error) {
	p = strings.TrimSpace(p)
	if p == "" || p == "." {
		return nil, nil
	}
	cleaned := filepath.Clean(filepath.FromSlash(p))
	if cleaned == "." {
		return nil, nil
	}
	if strings.HasPrefix(cleaned, "..") {
		return nil, fmt.Errorf("invalid relative path: %s", p)
	}
	segments := strings.Split(cleaned, string(os.PathSeparator))
	out := make([]string, 0, len(segments))
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" || seg == "." {
			continue
		}
		if seg == ".." {
			return nil, fmt.Errorf("invalid relative path: %s", p)
		}
		out = append(out, sanitizePathPart(seg))
	}
	return out, nil
}

func sanitizePathPart(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return v
	}
	v = strings.ReplaceAll(v, string(os.PathSeparator), "_")
	v = strings.ReplaceAll(v, "/", "_")
	return v
}

func branchPattern(mainPath string) string {
	ext := filepath.Ext(mainPath)
	base := strings.TrimSuffix(mainPath, ext)
	if ext == "" {
		return base + ":*"
	}
	return base + ":*" + ext
}

func branchPath(mainPath, sessionID string, seq int64) string {
	ext := filepath.Ext(mainPath)
	base := strings.TrimSuffix(mainPath, ext)
	tag := sanitizePathPart(sessionID)
	if tag == "" {
		tag = "session"
	}
	if ext == "" {
		return fmt.Sprintf("%s:%s:%d", base, tag, seq)
	}
	return fmt.Sprintf("%s:%s:%d%s", base, tag, seq, ext)
}

func parseMainRules(content string) []Rule {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var rules []Rule
	seq := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
		line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
		if line == "" {
			continue
		}
		rules = append(rules, Rule{Command: line, Seqno: seq})
		seq++
	}
	return rules
}

func encodeMainRules(rules []Rule) string {
	if len(rules) == 0 {
		return ""
	}
	var b strings.Builder
	for i, rule := range rules {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("- ")
		b.WriteString(strings.TrimSpace(rule.Command))
	}
	b.WriteByte('\n')
	return b.String()
}

func parseTempOps(content string) []fileWriteOp {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var ops []fileWriteOp
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 4)
		if len(parts) != 4 {
			continue
		}
		ts, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			continue
		}
		seq, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		ops = append(ops, fileWriteOp{
			ts:     ts,
			seqno:  seq,
			action: strings.ToLower(strings.TrimSpace(parts[2])),
			rule:   strings.TrimSpace(parts[3]),
		})
	}
	return ops
}

func encodeTempOps(ops []fileWriteOp) string {
	var b strings.Builder
	for i, op := range ops {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(strconv.FormatInt(op.ts, 10))
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(op.seqno))
		b.WriteByte(':')
		b.WriteString(strings.ToLower(strings.TrimSpace(op.action)))
		b.WriteByte(':')
		b.WriteString(strings.TrimSpace(op.rule))
	}
	b.WriteByte('\n')
	return b.String()
}

func sortTempOps(ops []fileWriteOp) {
	sort.SliceStable(ops, func(i, j int) bool {
		if ops[i].ts == ops[j].ts {
			return ops[i].seqno < ops[j].seqno
		}
		return ops[i].ts < ops[j].ts
	})
}

func applyTempOps(rules []Rule, ops []fileWriteOp) ([]Rule, []int) {
	merged := append([]Rule(nil), rules...)
	modified := make([]int, 0, len(ops))
	for _, op := range ops {
		switch op.action {
		case "add":
			idx := op.seqno
			if idx < 0 || idx > len(merged) {
				idx = len(merged)
			}
			merged = append(merged, Rule{})
			copy(merged[idx+1:], merged[idx:])
			merged[idx] = Rule{Command: op.rule, Seqno: idx}
			modified = append(modified, idx)
		case "update":
			idx := op.seqno
			if idx < 0 || idx >= len(merged) {
				idx = len(merged)
				merged = append(merged, Rule{Command: op.rule, Seqno: idx})
			} else {
				merged[idx] = Rule{Command: op.rule, Seqno: idx}
			}
			modified = append(modified, idx)
		case "remove":
			idx := op.seqno
			if idx >= 0 && idx < len(merged) {
				merged = append(merged[:idx], merged[idx+1:]...)
				modified = append(modified, idx)
			}
		}
	}
	merged = dedupeAdjacentRules(merged)
	for i := range merged {
		merged[i].Seqno = i
	}
	return merged, uniqueSortedInts(modified)
}

func dedupeAdjacentRules(rules []Rule) []Rule {
	if len(rules) < 2 {
		return rules
	}
	out := make([]Rule, 0, len(rules))
	for _, rule := range rules {
		if len(out) == 0 || strings.TrimSpace(out[len(out)-1].Command) != strings.TrimSpace(rule.Command) {
			out = append(out, rule)
		}
	}
	return out
}

func uniqueSortedInts(vals []int) []int {
	if len(vals) == 0 {
		return nil
	}
	sort.Ints(vals)
	out := vals[:0]
	var prev *int
	for i := range vals {
		if prev != nil && *prev == vals[i] {
			continue
		}
		out = append(out, vals[i])
		prev = &out[len(out)-1]
	}
	return append([]int(nil), out...)
}

func (f *FileSystem) readFileLocked(path string) (string, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_SH); err != nil {
		return "", err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	data, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (f *FileSystem) loadMergedDocument(ctx context.Context, id InstructionID) (InstructionDocument, error) {
	mainPath, err := f.resolveMainPath(id)
	if err != nil {
		return InstructionDocument{}, err
	}
	_ = ctx

	mainContent, err := f.readFileLocked(mainPath)
	if err != nil {
		return InstructionDocument{}, err
	}
	mainRules := parseMainRules(mainContent)

	branchOps, err := f.collectBranchOps(mainPath)
	if err != nil {
		return InstructionDocument{}, err
	}
	sortTempOps(branchOps)
	rules, _ := applyTempOps(mainRules, branchOps)
	raw := encodeMainRules(rules)
	return InstructionDocument{
		ID:        id,
		Raw:       raw,
		Rules:     rules,
		UpdatedAt: time.Now().UTC(),
	}, nil
}

func (f *FileSystem) collectBranchOps(mainPath string) ([]fileWriteOp, error) {
	matches, err := filepath.Glob(branchPattern(mainPath))
	if err != nil {
		return nil, err
	}
	var ops []fileWriteOp
	for _, match := range matches {
		content, err := f.readFileLocked(match)
		if err != nil {
			return nil, err
		}
		ops = append(ops, parseTempOps(content)...)
	}
	return ops, nil
}

func (f *FileSystem) writeTempFile(ctx context.Context, mainPath string, sessionID string, ops []fileWriteOp) (string, error) {
	if err := os.MkdirAll(filepath.Dir(mainPath), 0o755); err != nil {
		return "", err
	}
	seq := f.nextSequence(ctx, sessionID)
	tempPath := branchPath(mainPath, sessionID, seq)
	for i := range ops {
		ops[i].ts = time.Now().UnixMilli() + int64(i)
		if ops[i].seqno < 0 {
			ops[i].seqno = i
		}
	}

	content := encodeTempOps(ops)
	if err := os.WriteFile(tempPath, []byte(content), 0o644); err != nil {
		return "", err
	}
	return tempPath, nil
}

func (f *FileSystem) enqueueMerge(ctx context.Context, mainPath, sessionID, operation string) error {
	if f.merger == nil {
		return nil
	}
	f.recordOrigin(mainPath, sessionID, operation)
	return f.merger.Enqueue(ctx, mainPath)
}

func (f *FileSystem) mutateFromRules(ctx context.Context, id InstructionID, operation string, ops []fileWriteOp) (InstructionDocument, error) {
	mainPath, err := f.resolveMainPath(id)
	if err != nil {
		return InstructionDocument{}, err
	}
	if _, err := f.writeTempFile(ctx, mainPath, id.SessionID, ops); err != nil {
		return InstructionDocument{}, err
	}
	if err := f.enqueueMerge(ctx, mainPath, id.SessionID, operation); err != nil {
		return InstructionDocument{}, err
	}
	if f.merger == nil {
		f.recordOrigin(mainPath, id.SessionID, operation)
		f.dispatchMergedEvent(FileUpdatedEvent{FileID: mainPath, ModifiedSeqNos: seqnosFromOps(ops)})
	}
	return f.loadMergedDocument(ctx, id)
}

func seqnosFromOps(ops []fileWriteOp) []int {
	out := make([]int, 0, len(ops))
	for _, op := range ops {
		out = append(out, op.seqno)
	}
	return uniqueSortedInts(out)
}

// GetInstructions implements Store.
func (f *FileSystem) GetInstructions(ctx context.Context, id InstructionID) (InstructionDocument, error) {
	return f.loadMergedDocument(ctx, id)
}

// GetInstructionsMeta implements Store.
func (f *FileSystem) GetInstructionsMeta(ctx context.Context, id InstructionID) (InstructionMeta, error) {
	mainPath, err := f.resolveMainPath(id)
	if err != nil {
		return InstructionMeta{}, err
	}
	info, err := os.Stat(mainPath)
	if err != nil {
		return InstructionMeta{}, err
	}
	_ = ctx
	return InstructionMeta{
		ID:        id,
		Path:      mainPath,
		Size:      info.Size(),
		Mode:      info.Mode().String(),
		CreatedAt: info.ModTime().UTC(),
		UpdatedAt: info.ModTime().UTC(),
	}, nil
}

// GetInstructionsPartial implements Store.
func (f *FileSystem) GetInstructionsPartial(ctx context.Context, id InstructionID, params GetPartialInstructionParams) (InstructionDocument, error) {
	doc, err := f.loadMergedDocument(ctx, id)
	if err != nil {
		return InstructionDocument{}, err
	}
	start := params.Start
	end := params.End
	if start < 0 {
		start = 0
	}
	if end <= 0 || end > len(doc.Rules) {
		end = len(doc.Rules)
	}
	if start > end {
		start = end
	}
	doc.Rules = append([]Rule(nil), doc.Rules[start:end]...)
	for i := range doc.Rules {
		doc.Rules[i].Seqno = i
	}
	doc.Raw = encodeMainRules(doc.Rules)
	return doc, nil
}

// GetFolderIndex implements Store.
func (f *FileSystem) GetFolderIndex(ctx context.Context, id InstructionID) ([]FolderEntry, error) {
	dirPath, err := f.resolveFolderPath(id)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	out := make([]FolderEntry, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		out = append(out, FolderEntry{
			Name:      entry.Name(),
			Format:    folderEntryFormat(entry),
			Size:      info.Size(),
			UpdatedAt: info.ModTime().UTC(),
			CreatedAt: info.ModTime().UTC(),
			IsDir:     entry.IsDir(),
			Path:      filepath.Join(dirPath, entry.Name()),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	_ = ctx
	return out, nil
}

func folderEntryFormat(entry os.DirEntry) string {
	if entry.IsDir() {
		return "dir"
	}
	ext := strings.TrimPrefix(filepath.Ext(entry.Name()), ".")
	if ext == "" {
		return "file"
	}
	return ext
}

// AppendInstructions implements Store.
func (f *FileSystem) AppendInstructions(ctx context.Context, id InstructionID, params AppendInstructionParams) (InstructionDocument, error) {
	ops := make([]fileWriteOp, 0, len(params.Rules))
	for i, rule := range params.Rules {
		seq := rule.Seqno
		if seq < 0 {
			seq = i
		}
		ops = append(ops, fileWriteOp{
			seqno:  seq,
			action: "add",
			rule:   strings.TrimSpace(rule.Command),
		})
	}
	return f.mutateFromRules(ctx, id, "append", ops)
}

// UpdateInstructions implements Store.
func (f *FileSystem) UpdateInstructions(ctx context.Context, id InstructionID, params UpdateInstructionParams) (InstructionDocument, error) {
	ops := make([]fileWriteOp, 0, len(params.Rules))
	for i, rule := range params.Rules {
		seq := rule.Seqno
		if seq < 0 {
			seq = i
		}
		ops = append(ops, fileWriteOp{
			seqno:  seq,
			action: "update",
			rule:   strings.TrimSpace(rule.Command),
		})
	}
	return f.mutateFromRules(ctx, id, "update", ops)
}

// DeleteInstructions implements Store.
func (f *FileSystem) DeleteInstructions(ctx context.Context, id InstructionID, params DeleteInstructionParams) (InstructionDocument, error) {
	ops := make([]fileWriteOp, 0, len(params.SeqNo))
	for _, seq := range params.SeqNo {
		ops = append(ops, fileWriteOp{
			seqno:  seq,
			action: "remove",
			rule:   "",
		})
	}
	return f.mutateFromRules(ctx, id, "delete", ops)
}

// DiscardInstructions implements Store.
func (f *FileSystem) DiscardInstructions(ctx context.Context, id InstructionID, params DiscardInstructionParams) (InstructionDocument, error) {
	mainPath, err := f.resolveMainPath(id)
	if err != nil {
		return InstructionDocument{}, err
	}
	matches, err := filepath.Glob(branchPattern(mainPath))
	if err != nil {
		return InstructionDocument{}, err
	}
	targetSeqs := make(map[int]struct{}, len(params.SeqNo))
	for _, seq := range params.SeqNo {
		targetSeqs[seq] = struct{}{}
	}
	for _, match := range matches {
		content, err := os.ReadFile(match)
		if err != nil {
			return InstructionDocument{}, err
		}
		ops := parseTempOps(string(content))
		if len(targetSeqs) == 0 {
			_ = os.Remove(match)
			continue
		}
		filtered := ops[:0]
		for _, op := range ops {
			if _, drop := targetSeqs[op.seqno]; drop {
				continue
			}
			filtered = append(filtered, op)
		}
		if len(filtered) == 0 {
			_ = os.Remove(match)
			continue
		}
		if err := os.WriteFile(match, []byte(encodeTempOps(filtered)), 0o644); err != nil {
			return InstructionDocument{}, err
		}
	}
	if f.merger != nil {
		if err := f.merger.Enqueue(ctx, mainPath); err != nil {
			return InstructionDocument{}, err
		}
	}
	return f.loadMergedDocument(ctx, id)
}

// SubscribeInstructions implements Store.
func (f *FileSystem) SubscribeInstructions(ctx context.Context, id InstructionID, params SubscriptionParams) (Subscription, error) {
	mainPath, err := f.resolveMainPath(id)
	if err != nil {
		return Subscription{}, err
	}

	subID := atomic.AddUint64(&f.subID, 1)
	ch := make(chan StoreEvent, 32)
	record := &subscriptionRecord{
		id:         strconv.FormatUint(subID, 10),
		target:     mainPath,
		sessionID:  id.SessionID,
		ignoreSelf: params.IgnoreSelf,
		ch:         ch,
	}

	f.mu.Lock()
	f.subscriptions[record.id] = record
	f.mu.Unlock()

	closeFn := func() error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if sub, ok := f.subscriptions[record.id]; ok {
			delete(f.subscriptions, record.id)
			close(sub.ch)
		}
		return nil
	}

	_ = ctx
	return Subscription{ID: record.id, Events: ch, Close: closeFn}, nil
}

var _ Store = (*FileSystem)(nil)
