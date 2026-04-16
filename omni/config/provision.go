package config

// CLIConfigGetFlags is a koanf-compatible flag schema for `config get`.
type CLIConfigGetFlags struct {
	Output string `koanf:"output" json:"output"`
}

// CLITeamListFlags is a koanf-compatible flag schema for `team list`.
type CLITeamListFlags struct {
	Output string `koanf:"output" json:"output"`
}

// CLITeamInitFlags is a koanf-compatible flag schema for `team init`.
type CLITeamInitFlags struct {
	RepoURL string `koanf:"repo_url" json:"repo_url"`
}

// CLITeamGetFlags is a koanf-compatible flag schema for `team get`.
type CLITeamGetFlags struct {
	ID     string `koanf:"id" json:"id"`
	Output string `koanf:"output" json:"output"`
}

// CLIConfigSetFlags is a koanf-compatible flag schema for `config set`.
// Empty string means "do not update this field".
type CLIConfigSetFlags struct {
	Memory   string `koanf:"memory" json:"memory"`
	AutoSync string `koanf:"autosync" json:"autosync"`
}

// CLIAgentListFlags is a koanf-compatible flag schema for `agent list`.
type CLIAgentListFlags struct {
	Workspace string `koanf:"workspace" json:"workspace"`
	Output    string `koanf:"output" json:"output"`
}

// CLIAgentCreateFlags is a koanf-compatible flag schema for `agent init`.
type CLIAgentCreateFlags struct {
	Workspace          string `koanf:"workspace" json:"workspace"`
	Name               string `koanf:"name" json:"name"`
	Provider           string `koanf:"provider" json:"provider"`
	Model              string `koanf:"model" json:"model"`
	AllowGeneratedName bool   `koanf:"allow_generated_name" json:"allow_generated_name"`
	Interactive        bool   `koanf:"interactive" json:"interactive"`
}

// CLIAgentDeleteFlags is a koanf-compatible flag schema for `agent delete`.
type CLIAgentDeleteFlags struct {
	ID string `koanf:"id" json:"id"`
}

// CLIAgentDiscoverFlags is a koanf-compatible flag schema for `agent discover`.
type CLIAgentDiscoverFlags struct {
	Output string `koanf:"output" json:"output"`
}

// CLIAgentResumeFlags is a koanf-compatible flag schema for `agent resume`.
type CLIAgentResumeFlags struct {
	Workspace string `koanf:"workspace" json:"workspace"`
}

// CLIAgentUpgradeFlags is a koanf-compatible flag schema for `agent upgrade`.
type CLIAgentUpgradeFlags struct {
	ID      string `koanf:"id" json:"id"`
	Version string `koanf:"version" json:"version"`
}

func ProvisionConfigGetFlags() CLIConfigGetFlags {
	return CLIConfigGetFlags{Output: "yaml"}
}

func ProvisionTeamListFlags() CLITeamListFlags {
	return CLITeamListFlags{Output: "table"}
}

func ProvisionTeamInitFlags() CLITeamInitFlags {
	return CLITeamInitFlags{}
}

func ProvisionTeamGetFlags() CLITeamGetFlags {
	return CLITeamGetFlags{Output: "table"}
}

func ProvisionConfigSetFlags() CLIConfigSetFlags {
	return CLIConfigSetFlags{}
}

func ProvisionAgentListFlags() CLIAgentListFlags {
	return CLIAgentListFlags{Output: "table"}
}

func ProvisionAgentCreateFlags() CLIAgentCreateFlags {
	return CLIAgentCreateFlags{
		Interactive: true,
	}
}

func ProvisionAgentDeleteFlags() CLIAgentDeleteFlags {
	return CLIAgentDeleteFlags{}
}

func ProvisionAgentDiscoverFlags() CLIAgentDiscoverFlags {
	return CLIAgentDiscoverFlags{Output: "table"}
}

func ProvisionAgentResumeFlags() CLIAgentResumeFlags {
	return CLIAgentResumeFlags{}
}

func ProvisionAgentUpgradeFlags() CLIAgentUpgradeFlags {
	return CLIAgentUpgradeFlags{}
}

// ProvisionDefaultOmniConfig returns a new default OmniConfig instance.
func ProvisionDefaultOmniConfig() *OmniConfig {
	return &OmniConfig{
		Features: &Features{
			AutoSync:         true,
			RandomAgentNames: true,
		},
		Dev: &Developer{
			Debug: false,
		},
	}
}

// ApplyOmniConfigDefaults ensures optional OmniConfig sections are initialized.
func ApplyOmniConfigDefaults(cfg *OmniConfig) *OmniConfig {
	if cfg == nil {
		return ProvisionDefaultOmniConfig()
	}

	if cfg.Features == nil {
		cfg.Features = &Features{
			AutoSync:         true,
			RandomAgentNames: true,
		}
	}

	if cfg.Dev == nil {
		cfg.Dev = &Developer{
			Debug: false,
		}
	}

	return cfg
}
