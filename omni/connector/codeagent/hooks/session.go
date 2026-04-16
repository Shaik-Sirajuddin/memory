package hooks

const (
	PreSessionStart  HookID = "pre_session_start"
	PostSessionStart HookID = "post_session_start"
)

type PreSessionStartParams struct {
	HookInput
	Source string `json:"source"` // startup | resume | clear | compact
}

type PostSessionStartParams struct {
	HookInput
}

type PreSessionStartResult struct {
	HookOuput
}

type PostSessionStartResult struct {
	HookOuput
}
