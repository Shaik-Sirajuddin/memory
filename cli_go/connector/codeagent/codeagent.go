package codeagent

import "github.com/Shaik-Sirajuddin/memory/connector/sandbox"

type Provider string

const (
	Gemini Provider = "gemini"
	Claude Provider = "claude"
	Codex  Provider = "codex"
)

type Model struct {
	Provider Provider
	Model    string
}

type CreateSessionResult struct {
	ID string
}

type CreateSessionParams struct {
	ID       string
	ParentID string
}

type ExecuteParams struct {
	PromptId string
	Prompt   string
}
type ExecuteResult struct {
	PromptID string
	Response string
}
type ResumeSessionParams struct {
	ID string
}
type ResumeSessionResult struct {
	ProcessID string
}

type GetSessionConfigParams struct {
	ID string
}

type GetSessionConfigResult struct {
}

type UpdateSessionSandboxParams struct {
	Sandbox *sandbox.Sandbox
}
type UpdateSessionSandboxResult struct {
	Sandbox *sandbox.Sandbox
}

type GetSessionSandboxParams struct {
}

type GetSessionSandboxResult struct {
}

type CodeAgentInfo struct {
	Provider Provider
	Version  string
}

type UserIdentify struct {
	Authenticated bool
}

type GetHooksParams struct {
}
type GetHooksResult struct {
}

type HooksCapabalities struct {
	PreToolUse       bool
	PrePrompt        bool
	PostPrompt       bool
	PostToolUse      bool
	PreSessionStart  bool
	PostSessionStart bool
}

type Capabilities struct {
	Hooks *HooksCapabalities
}

// Config denotes
type Config struct {
	EnabledHooks HooksCapabalities
}

// CodeAgent implements Model
// CodeAgent Provides access to sessions
// All operations of CodeAgent are concurrent safe
type CodeAgent interface {
	// create a non interactive session and return
	// concurrent safe
	Create(CreateSessionParams) (*CreateSessionResult, error)
	// GPT:declare a new method to stream
	Exec(ExecuteParams) (*ExecuteResult, error)
	// Resume the sesion and return the process id , defaults to interactive
	Resume(ResumeSessionParams) (*ResumeSessionResult, error)

	GetSessionConfig(GetSessionConfigParams) (*GetSessionConfigResult, error)
	GetSessionSandbox(GetSessionSandboxParams) (*GetSessionSandboxResult, error)
	UpdateSessionSandbox(UpdateSessionSandboxParams) (*UpdateSessionSandboxResult, error)

	GetHooks(GetHooksParams) (GetHooksResult, error)
	UpdateHooks()

	Info() *CodeAgentInfo
	Capabilities() (*Capabilities, error)
	Config() (*Config, error)

	GetUserIdentity() UserIdentify
	// stop the interactive terminal session for agent
	Stop()
}
