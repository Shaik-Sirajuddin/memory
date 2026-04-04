package codeagent

import (
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent/hooks"
	"github.com/Shaik-Sirajuddin/memory/connector/sandbox"
)

type Provider string

type Model struct {
	Provider Provider
	Model    string
}

// PermissionMode mirrors the permission modes supported across agents.
type PermissionMode string

const (
	PermissionDefault           PermissionMode = "default"
	PermissionPlan              PermissionMode = "plan"
	PermissionAcceptEdits       PermissionMode = "acceptEdits"
	PermissionAuto              PermissionMode = "auto"
	PermissionDontAsk           PermissionMode = "dontAsk"
	PermissionBypassPermissions PermissionMode = "bypassPermissions"
)

// OutputFormat controls the format of Exec responses.
type OutputFormat string

const (
	OutputFormatText       OutputFormat = "text"
	OutputFormatJSON       OutputFormat = "json"
	OutputFormatStreamJSON OutputFormat = "stream-json"
)

// Session holds metadata about a persisted agent session.
type Session struct {
	ID       string
	Name     string
	Provider Provider
	Model    string
	WorkDir  string
}

// --- Create ---

type CreateSessionParams struct {
	ID             string
	ParentID       string
	Model          string
	Name           string
	WorkDir        string
	PermissionMode PermissionMode
	SystemPrompt   string
}

type CreateSessionResult struct {
	ID   string
	Name string
}

// --- Execute ---

type ExecuteParams struct {
	PromptId     string
	Prompt       string
	OutputFormat OutputFormat
	MaxTurns     int
}

type Usage struct {
	InputTokens  int
	OutputTokens int
	CostUSD      float64
}

type ExecuteResult struct {
	PromptID   string
	SessionID  string
	Response   string
	StopReason string
	Usage      *Usage
}

// --- Stream ---

type StreamParams struct {
	PromptId string
	Prompt   string
	MaxTurns int
}

type StreamEvent struct {
	Type    string // text | tool_use | tool_result | stop
	Content string
	Done    bool
}

type StreamResult struct {
	Events    <-chan StreamEvent
	SessionID string
}

// --- Resume ---

type ResumeSessionParams struct {
	ID          string
	ForkSession bool
}

type ResumeSessionResult struct {
	ProcessID string
	SessionID string
}

// --- List Sessions ---

type ListSessionsParams struct {
	WorkDir  string
	Provider Provider
}

type ListSessionsResult struct {
	Sessions []*Session
}

// --- Delete Session ---

type DeleteSessionParams struct {
	ID string
}

type DeleteSessionResult struct {
	Deleted bool
}

// --- Session Config ---

type GetSessionConfigParams struct {
	ID string
}

type GetSessionConfigResult struct {
	Model          Model
	PermissionMode PermissionMode
	WorkDir        string
	SystemPrompt   string
}

// --- Sandbox ---

type UpdateSessionSandboxParams struct {
	Sandbox *sandbox.Sandbox
}
type UpdateSessionSandboxResult struct {
	Sandbox *sandbox.Sandbox
}

type GetSessionSandboxParams struct {
	ID string
}

type GetSessionSandboxResult struct {
	Sandbox *sandbox.Sandbox
}

// --- Capabilities & Config ---

type Capabilities struct {
	Hooks      *hooks.Capabilities
	Streaming  bool
	MCPSupport bool
	Worktrees  bool
	Subagents  bool
}

type Config struct {
	Model          Model
	PermissionMode PermissionMode
	Hooks          *hooks.HookData
	Sandbox        *sandbox.Sandbox
}

// --- Info & Identity ---

type CodeAgentInfo struct {
	Provider Provider
	Name     string
	Version  string
}

type UserIdentify struct {
	Authenticated bool
	Email         string
	DisplayName   string
}

// CodeAgent implements Model
// CodeAgent Provides access to sessions
// All operations of CodeAgent are concurrent safe
type CodeAgent interface {
	hooks.HookIOParser
	hooks.HookManager

	// Create a non-interactive session and return its ID.
	Create(CreateSessionParams) (*CreateSessionResult, error)
	// Exec runs a prompt to completion and returns the full response.
	Exec(ExecuteParams) (*ExecuteResult, error)
	// Stream runs a prompt and returns a channel of incremental events.
	Stream(StreamParams) (*StreamResult, error)
	// Resume the session and return the process ID; defaults to interactive.
	Resume(ResumeSessionParams) (*ResumeSessionResult, error)
	// List returns persisted sessions matching the given filter.
	List(ListSessionsParams) (*ListSessionsResult, error)
	// Delete removes a persisted session.
	Delete(DeleteSessionParams) (*DeleteSessionResult, error)

	GetSessionConfig(GetSessionConfigParams) (*GetSessionConfigResult, error)
	GetSessionSandbox(GetSessionSandboxParams) (*GetSessionSandboxResult, error)
	UpdateSessionSandbox(UpdateSessionSandboxParams) (*UpdateSessionSandboxResult, error)

	Capabilities() (*Capabilities, error)
	Defaults() (*Config, error)
	UpdateDefaults(*Config) error

	Info() *CodeAgentInfo
	GetUserIdentity() UserIdentify
	// Stop terminates the active interactive session.
	Stop()
}
