package hooks

const (
	PrePrompt  HookID = "pre_prompt"
	PostPrompt HookID = "post_prompt"
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
