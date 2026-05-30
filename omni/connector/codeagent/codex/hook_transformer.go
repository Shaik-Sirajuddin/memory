package codex

import (
	"fmt"
	"strings"
	"sync"

	omniconfig "github.com/Shaik-Sirajuddin/memory/config"
	confhooks "github.com/Shaik-Sirajuddin/memory/config/hooks"
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
	codehooks "github.com/Shaik-Sirajuddin/memory/connector/codeagent/hooks"
)

type codexHookTransformer struct {
	mu    sync.RWMutex
	index map[string]omniconfig.HookEntry
	order []string
}

func NewHookTransformer() codeagent.HookTransformer {
	return &codexHookTransformer{
		index: map[string]omniconfig.HookEntry{},
	}
}

func (t *codexHookTransformer) Add(name string, entry omniconfig.HookEntry) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, ok := t.index[name]; ok {
		return false
	}
	omniEvent := resolveEventName(name, entry)
	codexEvent, ok := codexEventName(omniEvent)
	if !ok {
		return false
	}

	def := omniEntryToHookDef(entry)
	if def == nil {
		return false // URL-only entries not expressible as Codex command hooks
	}

	configPath, err := globalConfigPath()
	if err != nil {
		return false
	}
	hooksByEvent, err := readHooksConfig(configPath)
	if err != nil {
		return false
	}

	// Deduplicate by command string.
	for _, m := range hooksByEvent[codexEvent] {
		for _, d := range m.Hooks {
			if d.Command == def.Command {
				return false
			}
		}
	}

	if hooksByEvent == nil {
		hooksByEvent = map[string][]codexHookMatcher{}
	}
	hooksByEvent[codexEvent] = append(hooksByEvent[codexEvent], codexHookMatcher{
		Hooks: []codexHookDef{*def},
	})

	if err := writeHooksConfig(configPath, hooksByEvent); err != nil {
		return false
	}

	t.index[name] = entry
	t.order = append(t.order, name)
	return true
}

func (t *codexHookTransformer) GetHooks() []confhooks.Hook {
	configPath, err := globalConfigPath()
	if err != nil {
		return nil
	}
	hooksByEvent, err := readHooksConfig(configPath)
	if err != nil {
		return nil
	}

	var out []confhooks.Hook
	for codexEvent, matchers := range hooksByEvent {
		omniEvent, ok := omniEventFromCodex(codexEvent)
		if !ok {
			continue
		}
		for i, m := range matchers {
			for j, def := range m.Hooks {
				out = append(out, confhooks.Hook{
					Name:    fmt.Sprintf("codex.global.%s.%d.%d", codexEvent, i, j),
					Entry:   hookDefToOmniEntry(def),
					Schemas: schemaForEvent(omniEvent),
				})
			}
		}
	}
	return out
}

func (t *codexHookTransformer) GetHookResponse(eventName string, payload any) (confhooks.HookResponseSchema, error) {
	omniEvent := toOmniEvent(eventName)
	response, err := parseCodexHookPayload(omniEvent, payload)
	if err != nil {
		return confhooks.HookResponseSchema{}, err
	}
	return confhooks.HookResponseSchema{EventName: omniEvent, Response: response}, nil
}

func (t *codexHookTransformer) GetHookResult(eventName string, raw any) (confhooks.HookResultSchema, error) {
	omniEvent := toOmniEvent(eventName)
	response, err := parseCodexHookPayload(omniEvent, raw)
	if err != nil {
		return confhooks.HookResultSchema{}, err
	}
	return confhooks.HookResultSchema{EventName: omniEvent, Result: response}, nil
}

// toOmniEvent normalises an incoming event name to the omni standard.
// Accepts either a Codex CLI event name ("Stop") or an omni event name ("SessionEnd").
func toOmniEvent(event string) string {
	if omni, ok := omniEventFromCodex(event); ok {
		return omni
	}
	return event
}

func parseCodexHookPayload(eventName string, raw any) (confhooks.Response, error) {
	switch eventName {
	case string(codehooks.PreToolUse):
		result, err := parseHookInput[codehooks.PreToolUseResult](raw)
		if err != nil {
			return confhooks.Response{}, err
		}
		return responseFromOutput(result.HookOuput), nil
	case string(codehooks.PostToolUse):
		result, err := parseHookInput[codehooks.PostToolUseResult](raw)
		if err != nil {
			return confhooks.Response{}, err
		}
		return responseFromOutput(result.HookOuput), nil
	case string(codehooks.PostToolUseFailure):
		result, err := parseHookInput[codehooks.PostToolUseFailureResult](raw)
		if err != nil {
			return confhooks.Response{}, err
		}
		return responseFromOutput(result.HookOuput), nil
	case string(codehooks.SessionStart):
		result, err := parseHookInput[codehooks.SessionStartResult](raw)
		if err != nil {
			return confhooks.Response{}, err
		}
		return responseFromOutput(result.HookOuput), nil
	case string(codehooks.SessionEnd):
		result, err := parseHookInput[codehooks.SessionEndResult](raw)
		if err != nil {
			return confhooks.Response{}, err
		}
		return responseFromOutput(result.HookOuput), nil
	case string(codehooks.PrePrompt):
		result, err := parseHookInput[codehooks.PrePromptInputResult](raw)
		if err != nil {
			return confhooks.Response{}, err
		}
		return responseFromOutput(result.HookOuput), nil
	case string(codehooks.PostPrompt):
		result, err := parseHookInput[codehooks.PostPromptInputResult](raw)
		if err != nil {
			return confhooks.Response{}, err
		}
		return responseFromOutput(result.HookOuput), nil
	default:
		return confhooks.Response{}, fmt.Errorf("codexhooks: unknown hook event: %s", eventName)
	}
}

func responseFromOutput(output codehooks.HookOuput) confhooks.Response {
	return confhooks.Response{
		Continue:       output.Continue,
		StopReason:     output.StopReason,
		SuppressOutput: output.SuppressOutput,
		SystemMessage:  output.SystemMessage,
	}
}

// ============================================================
// omniconfig.HookEntry ↔ codexHookDef conversion
// ============================================================

// omniEntryToHookDef converts an omni HookEntry to a Codex hook definition.
// Returns nil for URL-only entries — Codex CLI only supports command hooks.
func omniEntryToHookDef(entry omniconfig.HookEntry) *codexHookDef {
	if entry.Command == nil {
		return nil
	}
	parts := append([]string{*entry.Command}, entry.Args...)
	def := &codexHookDef{Type: "command", Command: strings.Join(parts, " ")}
	if entry.Timeout != nil {
		def.Timeout = int(*entry.Timeout)
	}
	return def
}

// hookDefToOmniEntry converts a Codex hook definition back to an omni HookEntry.
// The Codex YAML stores command + args as one shell string, so we split on
// whitespace to restore the original Command + Args fields — preserving the
// entryKey invariant used by the registrar's verify() check.
func hookDefToOmniEntry(def codexHookDef) omniconfig.HookEntry {
	parts := strings.Fields(def.Command)
	var cmd string
	var args []string
	if len(parts) > 0 {
		cmd = parts[0]
		args = parts[1:]
	}
	var timeout *float64
	if def.Timeout > 0 {
		t := float64(def.Timeout)
		timeout = &t
	}
	entry := omniconfig.HookEntry{Command: &cmd, Timeout: timeout}
	if len(args) > 0 {
		entry.Args = args
	}
	return entry
}

// ============================================================
// Event name helpers
// ============================================================

func resolveEventName(name string, entry omniconfig.HookEntry) string {
	for i, arg := range entry.Args {
		if arg == "--event" && i+1 < len(entry.Args) {
			return entry.Args[i+1]
		}
		if value, ok := strings.CutPrefix(arg, "--event="); ok {
			return value
		}
	}
	if value, ok := strings.CutPrefix(name, "omni."); ok {
		return value
	}
	return name
}

// supportedEvent returns true when omniEvent has a Codex CLI equivalent.
func supportedEvent(event string) bool {
	_, ok := codexEventName(event)
	return ok
}

func schemaForEvent(event string) confhooks.HookSchema {
	switch event {
	case string(codehooks.PreToolUse):
		return &confhooks.PreToolUseSchema{}
	case string(codehooks.PostToolUse):
		return &confhooks.PostToolUseSchema{}
	case string(codehooks.PostToolUseFailure):
		return &confhooks.PostToolUseFailureSchema{}
	case string(codehooks.SessionStart):
		return &confhooks.SessionStartSchema{}
	case string(codehooks.SessionEnd):
		return &confhooks.SessionEndSchema{}
	case string(codehooks.PrePrompt):
		return &confhooks.PrePromptSchema{}
	case string(codehooks.PostPrompt):
		return &confhooks.PostPromptSchema{}
	default:
		return nil
	}
}
