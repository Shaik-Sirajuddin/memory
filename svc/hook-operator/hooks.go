package hookoperator

import (
	"github.com/Shaik-Sirajuddin/memory/config"
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent/hooks"
)

// DefaultHook pairs a lifecycle event with the hook entry the operator registers
// on every codeagent.
type DefaultHook struct {
	Name  string
	Event hooks.HookID
	Entry config.HookEntry
}

// DefaultHooks returns the full set of hook entries (one per event) for a provider.
// Each entry routes the codeagent event to:
//
//	<binaryPath> hook --event <eventName>
//
// The agentID is resolved at callback time from the session_id in the payload.
func DefaultHooks(binaryPath string) []DefaultHook {
	entry := func(event hooks.HookID) config.HookEntry {
		return config.HookEntry{
			Command: &binaryPath,
			Args:    []string{"hook", "--event", string(event)},
		}
	}

	return []DefaultHook{
		{Name: "hookop.pre_tool_use", Event: hooks.PreToolUse, Entry: entry(hooks.PreToolUse)},
		{Name: "hookop.post_tool_use", Event: hooks.PostToolUse, Entry: entry(hooks.PostToolUse)},
		{Name: "hookop.post_tool_use_failure", Event: hooks.PostToolUseFailure, Entry: entry(hooks.PostToolUseFailure)},
		{Name: "hookop.pre_session_start", Event: hooks.PreSessionStart, Entry: entry(hooks.PreSessionStart)},
		{Name: "hookop.post_session_start", Event: hooks.PostSessionStart, Entry: entry(hooks.PostSessionStart)},
		{Name: "hookop.pre_prompt", Event: hooks.PrePrompt, Entry: entry(hooks.PrePrompt)},
		{Name: "hookop.post_prompt", Event: hooks.PostPrompt, Entry: entry(hooks.PostPrompt)},
	}
}

// DefaultHookNames returns the registration keys for all operator default hooks.
func DefaultHookNames() []string {
	names := make([]string, 0, len(hooks.Hooks))
	for _, id := range hooks.Hooks {
		names = append(names, "hookop."+string(id))
	}
	return names
}
