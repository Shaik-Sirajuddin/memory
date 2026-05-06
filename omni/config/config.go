package config

import (
	"github.com/Shaik-Sirajuddin/memory/omniagent"
)

type Developer struct {
	Debug bool `json:"debug"`
}
type Features struct {
	Memory bool `json:"memory"`
	// Auto Sync when enabled propgates config changes from any agent to all configured agents default true
	AutoSync         bool `json:"auto_sync"`
	RandomAgentNames bool `json:"random_agent_names"`
}

// OmniConfig specific settings for omni
type OmniConfig struct {
	*Features
	Agent *omniagent.Settings `json:"agent,omitempty"`
	Dev   *Developer          `json:"dev,omitempty"`
}
