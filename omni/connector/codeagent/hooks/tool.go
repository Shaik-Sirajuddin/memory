package hooks

const (
	PreToolUse         HookID = "PreToolUse"
	PostToolUse        HookID = "PostToolUse"
	PostToolUseFailure HookID = "PostToolUseFailure"
)

type PreToolUseParams struct {
	HookInput
	ToolName   string         `json:"tool_name"`
	ToolInput  map[string]any `json:"tool_input"`
	ToolUseID  string         `json:"tool_use_id"`
}

type PostToolUseParams struct {
	HookInput
	ToolName     string         `json:"tool_name"`
	ToolInput    map[string]any `json:"tool_input"`
	ToolUseID    string         `json:"tool_use_id"`
	ToolResponse any            `json:"tool_response"`
}

type PostToolUseFailureParams struct {
	HookInput
	ToolName  string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
	ToolUseID string         `json:"tool_use_id"`
	Error     string         `json:"error"`
}

type PreToolUseResult struct {
	HookOuput
}

type PostToolUseResult struct {
	HookOuput
}

type PostToolUseFailureResult struct {
	HookOuput
}
