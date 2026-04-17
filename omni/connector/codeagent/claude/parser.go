package claude

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent/hooks"
)

// TODO : attach parser methods to ClaudeParser

// ClaudeParser implements [hooks.HookIOParser]
type ClaudeParser struct {
}

// ============================================================
// Claude stream-json wire format
// ============================================================
//
// `claude -p --output-format stream-json` emits newline-delimited JSON
// following the Anthropic Messages API streaming format:
//
//   message_start          → session metadata
//   content_block_start    → new content block
//   content_block_delta    → incremental text or tool input
//   content_block_stop     → block complete
//   message_delta          → stop reason / usage
//   message_stop           → stream finished

type claudeStreamEvent struct {
	Type  string             `json:"type"`
	Index int                `json:"index"`
	Delta *claudeStreamDelta `json:"delta,omitempty"`
}

type claudeStreamDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

// parseClaudeLine converts one raw JSON line from claude stream-json output
// into a codeagent.StreamEvent.
func parseClaudeLine(line string) codeagent.StreamEvent {
	var ev claudeStreamEvent
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		return codeagent.StreamEvent{Type: "text", Content: line}
	}

	switch ev.Type {
	case "content_block_delta":
		if ev.Delta == nil {
			return codeagent.StreamEvent{Type: "text", Content: ""}
		}
		switch ev.Delta.Type {
		case "text_delta":
			return codeagent.StreamEvent{Type: "text", Content: ev.Delta.Text}
		case "input_json_delta":
			return codeagent.StreamEvent{Type: "tool_use", Content: ev.Delta.PartialJSON}
		}
		return codeagent.StreamEvent{Type: "text", Content: ""}

	case "content_block_start", "message_start":
		return codeagent.StreamEvent{Type: "text", Content: ""}

	case "content_block_stop":
		return codeagent.StreamEvent{Type: "tool_result", Content: ""}

	case "message_delta":
		if ev.Delta != nil && ev.Delta.StopReason != "" {
			return codeagent.StreamEvent{Type: "stop", Done: true, Content: ev.Delta.StopReason}
		}
		return codeagent.StreamEvent{Type: "text", Content: ""}

	case "message_stop":
		return codeagent.StreamEvent{Type: "stop", Done: true}

	case "error":
		logger.Error("parseClaudeLine: stream error event", "line", line)
		return codeagent.StreamEvent{Type: "stop", Done: true, Content: line}

	default:
		return codeagent.StreamEvent{Type: ev.Type, Content: ""}
	}
}

// ============================================================
// Auth status parsing
// ============================================================

type claudeAuthStatus struct {
	LoggedIn    bool   `json:"logged_in"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

func parseAuthStatus(raw string) codeagent.UserIdentify {
	var s claudeAuthStatus
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		// Fallback: if exit 0 but can't parse JSON, assume authenticated.
		return codeagent.UserIdentify{Authenticated: true}
	}
	return codeagent.UserIdentify{
		Authenticated: s.LoggedIn,
		Email:         s.Email,
		DisplayName:   s.DisplayName,
	}
}

// ============================================================
// HookIOParser — parse Claude hook wire format → interface types
// ============================================================

func (a *ClaudeParser) PreToolUseParams(raw any) (*hooks.PreToolUseParams, error) {
	return parseHookInput[hooks.PreToolUseParams](raw)
}

func (a *ClaudeParser) PostToolUseParams(raw any) (*hooks.PostToolUseParams, error) {
	return parseHookInput[hooks.PostToolUseParams](raw)
}

func (a *ClaudeParser) PostToolUseFailureParams(raw any) (*hooks.PostToolUseFailureParams, error) {
	return parseHookInput[hooks.PostToolUseFailureParams](raw)
}

func (a *ClaudeParser) PreSessionStartParams(raw any) (*hooks.PreSessionStartParams, error) {
	return parseHookInput[hooks.PreSessionStartParams](raw)
}

func (a *ClaudeParser) PostSessionStartParams(raw any) (*hooks.PostSessionStartParams, error) {
	return parseHookInput[hooks.PostSessionStartParams](raw)
}

func (a *ClaudeParser) PrePromptInputParams(raw any) (*hooks.PrePromptInputParams, error) {
	return parseHookInput[hooks.PrePromptInputParams](raw)
}

func (a *ClaudeParser) PostPromptInputParams(raw any) (*hooks.PostPromptInputParams, error) {
	return parseHookInput[hooks.PostPromptInputParams](raw)
}

func (a *ClaudeParser) PreToolUseResult(raw any) (*hooks.PreToolUseResult, error) {
	return parseHookInput[hooks.PreToolUseResult](raw)
}

func (a *ClaudeParser) PostToolUseResult(raw any) (*hooks.PostToolUseResult, error) {
	return parseHookInput[hooks.PostToolUseResult](raw)
}

func (a *ClaudeParser) PostToolUseFailureResult(raw any) (*hooks.PostToolUseFailureResult, error) {
	return parseHookInput[hooks.PostToolUseFailureResult](raw)
}

func (a *ClaudeParser) PreSessionStartResult(raw any) (*hooks.PreSessionStartResult, error) {
	return parseHookInput[hooks.PreSessionStartResult](raw)
}

func (a *ClaudeParser) PostSessionStartResult(raw any) (*hooks.PostSessionStartResult, error) {
	return parseHookInput[hooks.PostSessionStartResult](raw)
}

func (a *ClaudeParser) PrePromptInputResult(raw any) (*hooks.PrePromptInputResult, error) {
	return parseHookInput[hooks.PrePromptInputResult](raw)
}

func (a *ClaudeParser) PostPromptInputResult(raw any) (*hooks.PostPromptInputResult, error) {
	return parseHookInput[hooks.PostPromptInputResult](raw)
}

// ============================================================
// Generic JSON round-trip parser
// ============================================================

func parseHookInput[T any](raw any) (*T, error) {
	var data []byte
	switch v := raw.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("claude: parser: marshal input: %w", err)
		}
		data = b
	}

	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("claude: parser: unmarshal %T: %w", result, err)
	}
	return &result, nil
}

// ============================================================
// Utilities
// ============================================================

func lookPath(name string) (string, error) {
	for _, candidate := range Config.Binary {
		if candidate == "" {
			continue
		}
		if candidate[0] == '/' {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return exec.LookPath(name)
}

func generateID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		logger.Error("generateID: crypto/rand failed", "err", err)
		return "claude-session"
	}
	return hex.EncodeToString(b)
}
