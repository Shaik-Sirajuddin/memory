package rulemerger

import (
	"fmt"
	"strconv"
	"strings"
)

// MainRule represents a rule in the main file format.
type MainRule struct {
	Rule string
}

// TempAction represents the action described in the temp file.
type TempAction string

const (
	ActionAdd    TempAction = "add"
	ActionUpdate TempAction = "update"
	ActionRemove TempAction = "remove"
)

// TempRule represents a single rule operation inside a temp branch file.
type TempRule struct {
	TimestampMillis int64
	SeqNo           int
	Action          TempAction
	Rule            string
}

// ParseTempFile parses contents of a temp_file_format.txt 
// where lines match: unix_timestamp_milli:seqno:action:<rule>
func ParseTempFile(content string) ([]TempRule, error) {
	lines := strings.Split(content, "\n")
	var rules []TempRule
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 4)
		if len(parts) != 4 {
			return nil, fmt.Errorf("invalid temp line format: %s", line)
		}

		ts, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid timestamp: %w", err)
		}

		seqno, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid seqno: %w", err)
		}

		rules = append(rules, TempRule{
			TimestampMillis: ts,
			SeqNo:           seqno,
			Action:          TempAction(parts[2]),
			Rule:            parts[3],
		})
	}
	return rules, nil
}

// ParseMainFile parses contents of file_format.txt
// where lines match: - rule
func ParseMainFile(content string) ([]MainRule, error) {
	lines := strings.Split(content, "\n")
	var rules []MainRule
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.HasPrefix(line, "- ") {
			return nil, fmt.Errorf("invalid main line format: %s", line)
		}
		rules = append(rules, MainRule{
			Rule: strings.TrimPrefix(line, "- "),
		})
	}
	return rules, nil
}

// FormatMainFile stringifies rules back into main file format
func FormatMainFile(rules []MainRule) string {
	var builder strings.Builder
	for _, r := range rules {
		builder.WriteString("- ")
		builder.WriteString(r.Rule)
		builder.WriteString("\n")
	}
	return builder.String()
}
