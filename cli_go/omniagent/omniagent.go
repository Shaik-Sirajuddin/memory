package omniagent

import (
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
	"github.com/Shaik-Sirajuddin/memory/connector/sandbox"
)

type CodeAgentInfo struct {
	Name          string
	Workspace     string
	ActiveSession *CodeSession
}

type CodeSession struct {
	Model          *codeagent.Model
	Idx            int
	Id             string
	IsActive       bool
	Prompts        int
	LastSyncPrompt int
}

type PersistentMemory struct {
	// agent write memory
	Dir string
}

type Settings struct {
	Sandbox      *sandbox.Sandbox
	DefaultModel *codeagent.Model
}

type Data struct {
	ActiveWorkSpace *sandbox.Workspace
	Info            *CodeAgentInfo
	Settings        *Settings
	Sessions        []*CodeSession
	Memory          *PersistentMemory
}

type UpdateSettingsParams struct {
	ID       string
	Settings *Settings
}

// OmniAgent is active only in one working dir at a time
// Forking an agent is allowed
type OmniAgent interface {
	New()
	// UpdateSettings updates agent settings , the settings are reflected after completion of an ongoing command execution
	UpdateSettings(UpdateSettingsParams) error
	// SyncMemory syncronizes session memory to persistant store from current model
	SyncMemory()
	// NewCodeSession creates a new session optionally can specify a different provider , model to use
	NewCodeSession()
}

type OmniAgentEntrypoint interface {
	PreToolUse()
	PrePrompt()
	PostPrompt()
	PostToolUse()
	PreSessionStart()
	PostSessionStart()
}