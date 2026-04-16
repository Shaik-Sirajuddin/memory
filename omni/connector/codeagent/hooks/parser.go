package hooks

type InputParser interface {
	PreToolUseParams(any) (*PreToolUseParams, error)
	PostToolUseParams(any) (*PostToolUseParams, error)
	PostToolUseFailureParams(any) (*PostToolUseFailureParams, error)
	PreSessionStartParams(any) (*PreSessionStartParams, error)
	PostSessionStartParams(any) (*PostSessionStartParams, error)
	PrePromptInputParams(any) (*PrePromptInputParams, error)
	PostPromptInputParams(any) (*PostPromptInputParams, error)
}

type OutputParser interface {
	PreToolUseResult(any) (*PreToolUseResult, error)
	PostToolUseResult(any) (*PostToolUseResult, error)
	PostToolUseFailureResult(any) (*PostToolUseFailureResult, error)
	PreSessionStartResult(any) (*PreSessionStartResult, error)
	PostSessionStartResult(any) (*PostSessionStartResult, error)
	PrePromptInputResult(any) (*PrePromptInputResult, error)
	PostPromptInputResult(any) (*PostPromptInputResult, error)
}

type HookIOParser interface {
	InputParser
	OutputParser
}
