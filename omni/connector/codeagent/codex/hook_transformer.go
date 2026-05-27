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
	event := resolveEventName(name, entry)
	if !supportedEvent(event) {
		return false
	}

	globalDir, err := globalCodexDir()
	if err != nil {
		return false
	}
	filePath := globalDir + "/hooks.json"

	hf, err := readHooksFile(filePath)
	if err != nil {
		return false
	}

	uid := name
	newEntry := configEntryToCodex(uid, event, entry)
	for _, e := range hf.Hooks {
		if sameCodexEntry(e, newEntry) {
			return false
		}
	}
	hf.Hooks = append(hf.Hooks, newEntry)
	if err := writeHooksFile(filePath, hf); err != nil {
		return false
	}

	t.index[name] = entry
	t.order = append(t.order, name)
	return true
}

func (t *codexHookTransformer) GetHooks() []confhooks.Hook {
	globalDir, err := globalCodexDir()
	if err != nil {
		return nil
	}
	filePath := globalDir + "/hooks.json"

	hf, err := readHooksFile(filePath)
	if err != nil {
		return nil
	}

	out := []confhooks.Hook{}
	for i, e := range hf.Hooks {
		if !supportedEvent(e.Event) {
			continue
		}
		entry := codexEntryToConfig(e)
		out = append(out, confhooks.Hook{
			Name:    fmt.Sprintf("codex.global.%s.%d", e.Event, i),
			Entry:   entry,
			Schemas: schemaForEvent(e.Event),
		})
	}
	return out
}

func (t *codexHookTransformer) GetHookResponse(eventName string, payload any) (confhooks.HookResponseSchema, error) {
	response, err := parseCodexHookPayload(eventName, payload)
	if err != nil {
		return confhooks.HookResponseSchema{}, err
	}
	return confhooks.HookResponseSchema{EventName: eventName, Response: response}, nil
}

func (t *codexHookTransformer) GetHookResult(eventName string, raw any) (confhooks.HookResultSchema, error) {
	response, err := parseCodexHookPayload(eventName, raw)
	if err != nil {
		return confhooks.HookResultSchema{}, err
	}
	return confhooks.HookResultSchema{EventName: eventName, Result: response}, nil
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
// omniconfig.HookEntry ↔ codexHookEntry conversion
// ============================================================

func configEntryToCodex(uid, event string, entry omniconfig.HookEntry) codexHookEntry {
	e := codexHookEntry{UID: uid, Event: event}
	if entry.Timeout != nil {
		e.Timeout = int(*entry.Timeout)
	}
	if entry.Url != nil {
		e.Url = entry.Url
		return e
	}
	if entry.Command != nil {
		e.Command = *entry.Command
	}
	e.Args = entry.Args
	return e
}

func codexEntryToConfig(e codexHookEntry) omniconfig.HookEntry {
	var timeout *float64
	if e.Timeout > 0 {
		t := float64(e.Timeout)
		timeout = &t
	}
	if e.Url != nil {
		return omniconfig.HookEntry{Url: e.Url, Timeout: timeout}
	}
	cmd := e.Command
	return omniconfig.HookEntry{Command: &cmd, Args: e.Args, Timeout: timeout}
}

func sameCodexEntry(a, b codexHookEntry) bool {
	return a.Event == b.Event &&
		a.Command == b.Command &&
		a.Timeout == b.Timeout &&
		ptrStringEq(a.Url, b.Url)
}

func ptrStringEq(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
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

func supportedEvent(event string) bool {
	switch event {
	case string(codehooks.PreToolUse),
		string(codehooks.PostToolUse),
		string(codehooks.PostToolUseFailure),
		string(codehooks.SessionStart),
		string(codehooks.SessionEnd),
		string(codehooks.PrePrompt),
		string(codehooks.PostPrompt):
		return true
	}
	return false
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
