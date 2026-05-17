package config

// DevConfig holds development / debug flags.
type DevConfig struct {
	Debug bool `json:"debug"`
}

// Settings holds the common configuration properties shared across all
// supported code agents (claude, codex, gemini).
type Settings struct {
	// Model is the AI model identifier to use for the session.
	Model *string `json:"model,omitempty" jsonschema:"title=Model,description=AI model identifier (e.g. claude-sonnet-4-6 / o4-mini / gemini-2.5-pro)"`

	// Timeout is the per-hook / per-invocation timeout in seconds.
	Timeout *float64 `json:"timeout,omitempty" jsonschema:"title=Timeout,description=Timeout in seconds for individual tool or hook invocations"`

	// MaxTurns caps the number of agentic turns per execution.
	MaxTurns *float64 `json:"maxTurns,omitempty" jsonschema:"title=Max Turns,description=Maximum number of agentic turns allowed per prompt execution"`

	// Sandbox controls the execution sandbox / permission mode.
	// Accepted values vary by agent: 'read-only', 'danger-full-access', etc.
	Sandbox *string `json:"sandbox,omitempty" jsonschema:"title=Sandbox,description=Sandbox or permission mode for tool execution"`

	// Env is a key-value map of environment variables injected into the agent process.
	Env map[string]string `json:"env,omitempty" jsonschema:"title=Environment Variables,description=Environment variables passed to the agent or spawned tools"`

	// Hooks contains per-event hook command definitions.
	Hooks map[string][]HookEntry `json:"hooks,omitempty" jsonschema:"title=Hooks,description=Lifecycle hooks mapped by event name to a list of hook commands"`

	// Theme is the UI theme name to apply (e.g. 'dark', 'light', or a custom theme id).
	Theme *string `json:"theme,omitempty" jsonschema:"title=Theme,description=UI theme name applied to the agent terminal output"`

	// Cwd is the working directory for the agent process.
	Cwd *string `json:"cwd,omitempty" jsonschema:"title=Working Directory,description=Working directory used when spawning the agent or tool processes"`

	// LogPrompts enables logging of user prompts to the agent's log output.
	LogPrompts *bool `json:"logPrompts,omitempty" jsonschema:"title=Log Prompts,description=Whether to log user prompts in the agent session output"`
}

// HookEntry represents a single hook command registered for a lifecycle event.
type HookEntry struct {
	// Command is the shell command or executable to invoke.
	Command *string `json:"command,omitempty" jsonschema:"title=Command,description=Shell command to run when the hook event fires"`

	// Args are additional arguments passed to the hook command.
	Args []string `json:"args,omitempty" jsonschema:"title=Args,description=Arguments passed to the hook command"`

	// Timeout overrides the global timeout for this specific hook.
	Timeout *float64 `json:"timeout,omitempty" jsonschema:"title=Timeout,description=Timeout in seconds for this hook (overrides global timeout)"`

	// Url is the HTTP endpoint for webhook-style hooks.
	Url *string `json:"url,omitempty" jsonschema:"title=URL,description=HTTP endpoint invoked for webhook-style hooks"`
}