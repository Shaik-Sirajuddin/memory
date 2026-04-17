package codeagent

import (
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent/hooks"
)

type Provider string

type Model struct {
	Provider Provider
	Model    string
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

// --- Capabilities & Config ---

type Capabilities struct {
	Hooks      *hooks.Capabilities
	Streaming  bool
	MCPSupport bool
	Worktrees  bool
	Subagents  bool
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

type ConfigPaths struct {
	GlobalConfigDirs    []string
	WorkspaceConfigDirs []string
	Binary              []string
}

// CodeAgent implements Model
// CodeAgent Provides access to sessions
// All operations of CodeAgent are concurrent safe
type CodeAgent interface {
	hooks.HookIOParser
	hooks.HookManager
	SettingsResolver

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

	Info() *CodeAgentInfo
	GetUserIdentity() UserIdentify
	// Stop terminates the active interactive session.
	Stop()
}

// TODO : all codeagent should accept a *sandbox.Config in new

// CodeAgentTakes a shell with new command where it can execute
