package rulemerger

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// actionMergeFile performs the logic to merge temporary branch files into the main file.
func (m *RuleMerger) actionMergeFile(ctx context.Context, mainFilePath string) {
	// 1. Glob specific directory for all matching temp files
	dir := filepath.Dir(mainFilePath)
	base := filepath.Base(mainFilePath)
	ext := filepath.Ext(base)
	nameWithoutExt := strings.TrimSuffix(base, ext)

	// We assume branch files use the format: nameWithoutExt:someAdditionalTags[ext]
	pattern := filepath.Join(dir, nameWithoutExt+":*"+ext)
	tempFilePaths, err := filepath.Glob(pattern)
	if err != nil || len(tempFilePaths) == 0 {
		return // No branches to merge
	}

	// 2. Read all branch contents and sort by Unix Timestamp
	var allTempRules []TempRule
	for _, tp := range tempFilePaths {
		content, err := os.ReadFile(tp)
		if err != nil {
			log.Printf("failed to read temp file %s: %v", tp, err)
			continue
		}
		rules, err := ParseTempFile(string(content))
		if err != nil {
			log.Printf("failed to parse temp file %s: %v", tp, err)
			continue
		}
		allTempRules = append(allTempRules, rules...)
	}

	// Sort rules by TimestampMillis
	sort.SliceStable(allTempRules, func(i, j int) bool {
		return allTempRules[i].TimestampMillis < allTempRules[j].TimestampMillis
	})

	// 3. Acquire file lock on the main file
	f, err := os.OpenFile(mainFilePath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Printf("failed to open main file string %s for locking: %v", mainFilePath, err)
		return
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		log.Printf("failed to acquire flock on %s: %v", mainFilePath, err)
		return
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	// Process Main File
	contentBytes, _ := io.ReadAll(f)
	mainRules, _ := ParseMainFile(string(contentBytes))

	mainRules, finalModifiedSeqNos := m.computeMergedRules(ctx, mainRules, allTempRules)

	// Write back
	if err := f.Truncate(0); err == nil {
		f.Seek(0, 0)
		f.WriteString(FormatMainFile(mainRules))
	}

	// Cleanup process
	for _, tp := range tempFilePaths {
		os.Remove(tp)
	}

	// Event Bus
	m.emitEvent(FileUpdatedEvent{
		FileID:         mainFilePath,
		ModifiedSeqNos: finalModifiedSeqNos,
	})
}

// ActionMergeFilePartial performs merge logic without saving permanently.
func (m *RuleMerger) ActionMergeFilePartial(ctx context.Context, mainFilePath string, changesOnly bool) ([]MainRule, error) {
	dir := filepath.Dir(mainFilePath)
	base := filepath.Base(mainFilePath)
	ext := filepath.Ext(base)
	nameWithoutExt := strings.TrimSuffix(base, ext)

	pattern := filepath.Join(dir, nameWithoutExt+":*"+ext)
	tempFilePaths, err := filepath.Glob(pattern)
	var allTempRules []TempRule
	if err == nil && len(tempFilePaths) > 0 {
		for _, tp := range tempFilePaths {
			content, err := os.ReadFile(tp)
			if err != nil {
				continue
			}
			rules, err := ParseTempFile(string(content))
			if err != nil {
				continue
			}
			allTempRules = append(allTempRules, rules...)
		}
		sort.SliceStable(allTempRules, func(i, j int) bool {
			return allTempRules[i].TimestampMillis < allTempRules[j].TimestampMillis
		})
	}

	f, err := os.OpenFile(mainFilePath, os.O_RDONLY, 0644)
	var mainRules []MainRule
	if err == nil {
		defer f.Close()
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err == nil {
			defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		}
		contentBytes, _ := io.ReadAll(f)
		mainRules, _ = ParseMainFile(string(contentBytes))
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	mergedRules, modifiedSeqNos := m.computeMergedRules(ctx, mainRules, allTempRules)

	if changesOnly {
		var diff []MainRule
		for _, idx := range modifiedSeqNos {
			if idx >= 0 && idx < len(mergedRules) {
				diff = append(diff, mergedRules[idx])
			}
		}
		return diff, nil
	}
	return mergedRules, nil
}

func (m *RuleMerger) computeMergedRules(ctx context.Context, mainRules []MainRule, allTempRules []TempRule) ([]MainRule, []int) {
	// Track modified seqnos and index shifts
	// original_seqno -> current_seqno mapping
	seqMap := make(map[int]int)

	// Pre-fill identity map for up to a reasonable list size based on current elements and temp rules
	maxLen := len(mainRules) + len(allTempRules)
	for i := 0; i < maxLen+10; i++ {
		seqMap[i] = i
	}

	modifiedCurrentSeqNos := make(map[int]bool)

	// 4. Apply actions sequentially
	for _, tr := range allTempRules {
		idx := seqMap[tr.SeqNo] // current mapped index

		switch tr.Action {
		case ActionAdd:
			if idx > len(mainRules) {
				idx = len(mainRules)
			}
			// Insert after idx
			insertAt := idx + 1
			if insertAt > len(mainRules) {
				insertAt = len(mainRules)
			}

			// Shift elements to make space
			mainRules = append(mainRules[:insertAt], append([]MainRule{{Rule: tr.Rule}}, mainRules[insertAt:]...)...)

			// Update subsequent sequence map mapping to reflect push right
			for orig, curr := range seqMap {
				if curr >= insertAt {
					seqMap[orig] = curr + 1
				}
			}
			modifiedCurrentSeqNos[insertAt] = true

		case ActionUpdate:
			if idx < 0 || idx >= len(mainRules) {
				continue
			}
			mainRules[idx] = MainRule{Rule: tr.Rule}
			modifiedCurrentSeqNos[idx] = true

		case ActionRemove:
			if idx < 0 || idx >= len(mainRules) {
				continue
			}
			// Delete element
			mainRules = append(mainRules[:idx], mainRules[idx+1:]...)

			// Update subsequent sequence map mapping to reflect shift left
			for orig, curr := range seqMap {
				if curr > idx {
					seqMap[orig] = curr - 1
				} else if curr == idx {
					seqMap[orig] = -1 // marked as deleted
				}
			}
			// Removing an element shouldn't trigger an LLM deduplication on it directly,
			// but we can mark the index taking its place.
			if idx < len(mainRules) {
				modifiedCurrentSeqNos[idx] = true
			}
		}
	}

	// 5. Iterate over modified indexes using LLM uniquely rules
	// Convert map to sorted slice of indices affected
	var modIndices []int
	for k := range modifiedCurrentSeqNos {
		modIndices = append(modIndices, k)
	}
	sort.Ints(modIndices)

	finalModifiedSeqNos := []int{}

	// To avoid colliding shifts during evaluation, we evaluate left to right
	for _, i := range modIndices {
		// Note: since lists shrink, `i` might no longer be valid or already processed.
		// We re-evaluate bounds dynamically
		if i < 0 || i >= len(mainRules) {
			continue
		}

		start := i - 1
		if start < 0 {
			start = 0
		}
		end := i + 1
		if end >= len(mainRules) {
			end = len(mainRules) - 1
		}

		ruleSet := []string{}
		for j := start; j <= end; j++ {
			ruleSet = append(ruleSet, mainRules[j].Rule)
		}

		if len(ruleSet) == 0 {
			continue
		}

		// Agent invocation
		if m.agent != nil {
			uniqueSet, err := m.agent.IdentifyUniqueRules(ctx, ruleSet)
			if err == nil && len(uniqueSet) > 0 {
				// Replace
				numReplaced := end - start + 1

				var newRules []MainRule
				for _, us := range uniqueSet {
					newRules = append(newRules, MainRule{Rule: us})
				}

				head := mainRules[:start]
				tail := mainRules[end+1:]

				mainRules = append(head, append(newRules, tail...)...)

				// Update remaining modIndices due to shifts
				shift := len(newRules) - numReplaced
				if shift != 0 {
					for mi := 0; mi < len(modIndices); mi++ {
						if modIndices[mi] > end {
							modIndices[mi] += shift
						}
					}
				}
			}
		}
		finalModifiedSeqNos = append(finalModifiedSeqNos, start) // record approximated area
	}

	return mainRules, finalModifiedSeqNos
}
