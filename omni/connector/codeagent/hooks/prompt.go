package hooks

const (
	PrePrompt  HookID = "UserPromptSubmit"
	PostPrompt HookID = "Stop"
)

type PrePromptInputParams struct {
	HookInput
	Prompt string `json:"prompt"`
}

type PostPromptInputParams struct {
	HookInput
	Prompt   string `json:"prompt"`
	Response string `json:"response"`
}

type PrePromptInputResult struct {
	HookOuput
}

type PostPromptInputResult struct {
	HookOuput
}
