package codex

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
	sandbox "github.com/Shaik-Sirajuddin/memory/sandbox/provider"
)

const (
	Codex codeagent.Provider = "codex"
)

// ============================================================
// Session lifecycle
// ============================================================

// Create prepares a codex CLI session.
// Codex has no persistent server-side sessions; "creating" a session means
// (1) applying the caller's params, (2) verifying the codex binary is reachable
// and the user is authenticated, and (3) writing the resolved model into the
// workspace .codex/config.toml so that any interactive `codex` invocation in
// the same directory inherits the same defaults.
func (a *codexAgent) Create(p codeagent.CreateSessionParams) (*codeagent.CreateSessionResult, error) {
	a.mu.Lock()
	if p.WorkDir != "" {
		a.workDir = p.WorkDir
	}
	if p.Model != "" {
		a.model = p.Model
	}
	id := p.ID
	if id == "" {
		id = generateID()
	}
	a.sessionID = id
	workDir := a.workDir
	model := a.model
	a.mu.Unlock()

	// Verify the codex binary is reachable by running `codex --version`.
	out, err := captureOutput(workDir, "codex", "--version")
	if err != nil {
		return nil, fmt.Errorf("codex: create: binary unreachable: %w", err)
	}
	logger.Debug("Create: codex binary ok", "version", trimSpace(out))

	// Verify authentication via `codex auth status` (exit 0 = authenticated).
	authCmd := exec.Command("codex", "auth", "status")
	authCmd.Dir = workDir
	if err := authCmd.Run(); err != nil {
		logger.Warn("Create: user not authenticated", "err", err)
		return nil, fmt.Errorf("codex: create: not authenticated — run 'codex auth login' first")
	}

	// Persist model into .codex/config.toml so interactive sessions inherit it.
	if syncErr := syncModelConfig(workDir, model); syncErr != nil {
		// Non-fatal: log and continue — Exec/Stream always pass -m explicitly.
		logger.Warn("Create: could not sync model to config", "err", syncErr)
	}

	logger.Info("Create: session ready", "id", id, "model", model, "workDir", workDir)
	return &codeagent.CreateSessionResult{ID: id, Name: p.Name}, nil
}

func (a *codexAgent) Resume(p codeagent.ResumeSessionParams) (*codeagent.ResumeSessionResult, error) {
	a.mu.RLock()
	workDir := a.workDir
	a.mu.RUnlock()

	cmdName := "resume"
	if p.ForkSession {
		cmdName = "fork"
	}

	args := []string{cmdName, "-C", workDir}
	if p.ID != "" {
		args = append(args, p.ID)
	} else {
		args = append(args, "--last")
	}

	cmd := exec.Command("codex", args...)
	cmd.Dir = workDir
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("codex: resume: start process: %w", err)
	}

	pid := fmt.Sprintf("%d", cmd.Process.Pid)
	resolvedSessionID := p.ID
	if resolvedSessionID == "" {
		resolvedSessionID = "last"
	}

	a.mu.Lock()
	a.activeCmd = cmd
	a.sessionID = resolvedSessionID
	a.mu.Unlock()

	logger.Info("Resume: interactive session started", "pid", pid, "sessionID", resolvedSessionID, "fork", p.ForkSession)
	return &codeagent.ResumeSessionResult{ProcessID: pid, SessionID: resolvedSessionID}, nil
}

func (a *codexAgent) List(p codeagent.ListSessionsParams) (*codeagent.ListSessionsResult, error) {
	a.mu.RLock()
	defaultWorkDir := a.workDir
	defaultModel := a.model
	a.mu.RUnlock()

	workDir := strings.TrimSpace(p.WorkDir)
	if workDir == "" {
		workDir = defaultWorkDir
	}

	indexEntries, err := readSessionIndex()
	if err != nil {
		return nil, fmt.Errorf("codex: list sessions: %w", err)
	}

	sessions := make([]*codeagent.Session, 0, len(indexEntries))
	for _, entry := range indexEntries {
		meta, metaErr := readSessionMeta(entry.ID)
		if metaErr != nil {
			logger.Debug("List: skipping session with unreadable metadata", "sessionID", entry.ID, "err", metaErr)
			continue
		}
		if workDir != "" && meta.Cwd != "" && meta.Cwd != workDir {
			continue
		}

		name := strings.TrimSpace(entry.ThreadName)
		if name == "" {
			name = filepath.Base(meta.FilePath)
		}

		sessions = append(sessions, &codeagent.Session{
			ID:       entry.ID,
			Name:     name,
			Provider: Codex,
			Model:    defaultModel,
			WorkDir:  meta.Cwd,
		})
	}

	logger.Info("List: sessions resolved from codex session store", "count", len(sessions), "workDir", workDir)
	return &codeagent.ListSessionsResult{Sessions: sessions}, nil
}

func (a *codexAgent) Delete(p codeagent.DeleteSessionParams) (*codeagent.DeleteSessionResult, error) {
	sessionID := strings.TrimSpace(p.ID)
	if sessionID == "" {
		return nil, errors.New("codex: delete session: empty session id")
	}

	meta, err := readSessionMeta(sessionID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logger.Warn("Delete: session not found", "sessionID", sessionID)
			return &codeagent.DeleteSessionResult{Deleted: false}, nil
		}
		return nil, fmt.Errorf("codex: delete session %q: %w", sessionID, err)
	}

	if err := os.Remove(meta.FilePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("codex: delete session %q file: %w", sessionID, err)
	}
	if err := removeSessionIndexEntry(sessionID); err != nil {
		return nil, fmt.Errorf("codex: delete session %q index: %w", sessionID, err)
	}

	a.mu.Lock()
	if a.sessionID == sessionID {
		a.sessionID = ""
	}
	a.mu.Unlock()

	logger.Info("Delete: session removed from codex session store", "sessionID", sessionID)
	return &codeagent.DeleteSessionResult{Deleted: true}, nil
}

func (a *codexAgent) Stop() {
	a.mu.Lock()
	cmd := a.activeCmd
	a.activeCmd = nil
	a.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		logger.Info("Stop: no active codex process")
		return
	}
	if err := cmd.Process.Kill(); err != nil {
		logger.Warn("Stop: failed to kill active process", "err", err)
		return
	}
	logger.Info("Stop: active codex process terminated")
}

// ============================================================
// GetSessionConfig
// ============================================================

func (a *codexAgent) GetSessionConfig(_ codeagent.GetSessionConfigParams) (*codeagent.GetSessionConfigResult, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return &codeagent.GetSessionConfigResult{
		Model:          codeagent.Model{Provider: Codex, Model: a.model},
		PermissionMode: codeagent.PermissionDefault,
		WorkDir:        a.workDir,
	}, nil
}

// ============================================================
// Exec
// ============================================================

func (a *codexAgent) Exec(p codeagent.ExecuteParams) (*codeagent.ExecuteResult, error) {
	a.mu.RLock()
	workDir := a.workDir
	model := a.model
	sbx := a.sbx
	a.mu.RUnlock()

	args := buildExecArgs(p.Prompt, model, p.OutputFormat, p.MaxTurns, sbx)
	logger.Debug("Exec", "workDir", workDir, "args", args)

	cmd := exec.Command("codex", args...)
	cmd.Dir = workDir

	out, err := cmd.Output()
	if err != nil {
		return nil, wrapExitError("codex exec", err)
	}

	response := strings.TrimSpace(string(out))
	logger.Debug("Exec completed", "responseLen", len(response))

	return &codeagent.ExecuteResult{
		PromptID:   p.PromptId,
		Response:   response,
		StopReason: "stop",
	}, nil
}

// ============================================================
// Stream
// ============================================================

func (a *codexAgent) Stream(p codeagent.StreamParams) (*codeagent.StreamResult, error) {
	a.mu.RLock()
	workDir := a.workDir
	model := a.model
	sbx := a.sbx
	a.mu.RUnlock()

	args := buildStreamArgs(p.Prompt, model, p.MaxTurns, sbx)
	logger.Debug("Stream", "workDir", workDir, "args", args)

	cmd := exec.Command("codex", args...)
	cmd.Dir = workDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("codex stream: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("codex stream: start process: %w", err)
	}

	ch := make(chan codeagent.StreamEvent, 32)

	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			ev := parseCodexLine(line)
			ch <- ev
			if ev.Done {
				return
			}
		}
		if err := cmd.Wait(); err != nil {
			msg := wrapExitError("codex stream", err).Error()
			logger.Error("Stream: process exited with error", "err", msg)
			ch <- codeagent.StreamEvent{Type: "stop", Done: true, Content: msg}
			return
		}
		ch <- codeagent.StreamEvent{Type: "stop", Done: true}
		logger.Debug("Stream completed")
	}()

	return &codeagent.StreamResult{Events: ch}, nil
}

// ============================================================
// Arg builders
// ============================================================

func buildExecArgs(prompt, model string, format codeagent.OutputFormat, maxTurns int, sbx *sandbox.Config) []string {
	args := []string{"exec", prompt, "-m", model}
	if format == codeagent.OutputFormatJSON || format == codeagent.OutputFormatStreamJSON {
		args = append(args, "--json")
	}
	if maxTurns > 0 {
		args = append(args, fmt.Sprintf("--max-turns=%d", maxTurns))
	}
	if sf := sandboxFlagValue(sbx); sf != "" {
		args = append(args, "--sandbox", sf)
	}
	return args
}

func buildStreamArgs(prompt, model string, maxTurns int, sbx *sandbox.Config) []string {
	args := []string{"exec", prompt, "-m", model, "--json"}
	if maxTurns > 0 {
		args = append(args, fmt.Sprintf("--max-turns=%d", maxTurns))
	}
	if sf := sandboxFlagValue(sbx); sf != "" {
		args = append(args, "--sandbox", sf)
	}
	return args
}

// ============================================================
// Helpers
// ============================================================

func wrapExitError(op string, err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("%s: exit %d: %s", op, exitErr.ExitCode(), strings.TrimSpace(string(exitErr.Stderr)))
	}
	return fmt.Errorf("%s: %w", op, err)
}

func captureOutput(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

func trimSpace(s string) string {
	return strings.TrimSpace(s)
}

type sessionIndexEntry struct {
	ID         string `json:"id"`
	ThreadName string `json:"thread_name"`
	UpdatedAt  string `json:"updated_at"`
}

type sessionMetaLine struct {
	Type    string             `json:"type"`
	Payload sessionMetaPayload `json:"payload"`
}

type sessionMetaPayload struct {
	ID  string `json:"id"`
	Cwd string `json:"cwd"`
}

type sessionMeta struct {
	Cwd      string
	FilePath string
}

func readSessionIndex() ([]sessionIndexEntry, error) {
	sessionDir, err := globalCodexDir()
	if err != nil {
		return nil, err
	}
	indexPath := filepath.Join(sessionDir, "session_index.jsonl")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var entries []sessionIndexEntry
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry sessionIndexEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			logger.Debug("readSessionIndex: skipping malformed line", "err", err)
			continue
		}
		if strings.TrimSpace(entry.ID) == "" {
			continue
		}
		entries = append(entries, entry)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		ti, errI := time.Parse(time.RFC3339Nano, entries[i].UpdatedAt)
		tj, errJ := time.Parse(time.RFC3339Nano, entries[j].UpdatedAt)
		if errI == nil && errJ == nil {
			return ti.After(tj)
		}
		return entries[i].UpdatedAt > entries[j].UpdatedAt
	})

	return entries, nil
}

func readSessionMeta(sessionID string) (*sessionMeta, error) {
	filePath, err := findSessionFile(sessionID)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return &sessionMeta{FilePath: filePath}, nil
	}

	var line sessionMetaLine
	if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
		return nil, fmt.Errorf("parse session meta %s: %w", filePath, err)
	}
	return &sessionMeta{
		Cwd:      line.Payload.Cwd,
		FilePath: filePath,
	}, nil
}

func findSessionFile(sessionID string) (string, error) {
	codexDir, err := globalCodexDir()
	if err != nil {
		return "", err
	}

	searchRoot := filepath.Join(codexDir, "sessions")
	var match string
	walkErr := filepath.WalkDir(searchRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.Contains(filepath.Base(path), sessionID) {
			match = path
			return fs.SkipAll
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, fs.SkipAll) {
		return "", walkErr
	}
	if match == "" {
		return "", os.ErrNotExist
	}
	return match, nil
}

func removeSessionIndexEntry(sessionID string) error {
	codexDir, err := globalCodexDir()
	if err != nil {
		return err
	}

	indexPath := filepath.Join(codexDir, "session_index.jsonl")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	lines := strings.Split(string(data), "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var entry sessionIndexEntry
		if err := json.Unmarshal([]byte(trimmed), &entry); err != nil || entry.ID != sessionID {
			filtered = append(filtered, trimmed)
		}
	}

	content := strings.Join(filtered, "\n")
	if content != "" {
		content += "\n"
	}
	return os.WriteFile(indexPath, []byte(content), 0o600)
}
