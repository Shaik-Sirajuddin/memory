package hooks

type HookID string

// Hooks is the full set of declared hook IDs.
var Hooks = []HookID{
	PreToolUse,
	PostToolUse,
	PostToolUseFailure,
	PrePrompt,
	PostPrompt,
	PreSessionStart,
	PostSessionStart,
}

type Capabilities struct {
	PrePrompt          bool
	PostPrompt         bool
	PreToolUse         bool
	PostToolUse        bool
	PostToolUseFailure bool
	PreSessionStart    bool
	PostSessionStart   bool
}

// type Hook struct {
// 	Input  *HookInput
// 	Output *HookOuput
// }

// HookInput holds fields present in every provider's hook input payload.
type HookInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	HookEventName  HookID `json:"hook_event_name"`
}

// HookOuput holds fields present in every provider's hook output payload.
type HookOuput struct {
	Continue       bool    `json:"continue"`
	StopReason     *string `json:"stopReason,omitempty"`
	SuppressOutput bool    `json:"suppressOutput"`
	SystemMessage  *string `json:"systemMessage,omitempty"`
}
