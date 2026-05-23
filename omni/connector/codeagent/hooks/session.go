package hooks

const (
	PreSessionStart  HookID = "PreSessionStart"
	PostSessionStart HookID = "PostSessionStart"
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
