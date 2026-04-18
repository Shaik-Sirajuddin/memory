package claude

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
	claudelog "github.com/Shaik-Sirajuddin/memory/connector/codeagent/log"
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent/claude/settings"
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent/hooks"
	rootsandbox "github.com/Shaik-Sirajuddin/memory/sandbox"
	sandbox "github.com/Shaik-Sirajuddin/memory/sandbox/provider"
)

var Config codeagent.ConfigPaths = codeagent.ConfigPaths{
	GlobalConfigDirs: []string{
		".claude",
		".config/claude",
	},
	WorkspaceConfigDirs: []string{
		".claude",
	},
	Binary: []string{
		"claude",
		"/usr/local/bin/claude",
		"/opt/homebrew/bin/claude",
	},
}

// logger is the package-level structured logger for the claude connector.
var logger = claudelog.NewLogger("claude")

// ============================================================
// Available models
// ============================================================

// ModelID is a Claude model identifier.
type ModelID = string

const (
	ModelOpus4   ModelID = "claude-opus-4-6"
	ModelSonnet4 ModelID = "claude-sonnet-4-6"
	ModelHaiku45 ModelID = "claude-haiku-4-5-20251001"
)

// StaticModels is the curated list of Claude models.
var StaticModels = []ModelID{ModelOpus4, ModelSonnet4, ModelHaiku45}

// DefaultModel is used when the caller does not specify a model.
const DefaultModel = ModelSonnet4

// ============================================================
// Agent struct
// ============================================================

type claudeAgent struct {
	*ClaudeParser
	resolver        *settings.Resolver
	mu              sync.RWMutex
	workDir         string
	model           string
	permMode        codeagent.PermissionMode
	systemPrompt    string
	sessionID       string
	sbx             *sandbox.Config
	sbxProvisioner  sandbox.SandboxProvisioner
	sbxRuntime      sandbox.SandboxRuntime
	info            codeagent.CodeAgentInfo
	registeredHooks []*hooks.HookData
}

// New returns a CodeAgent backed by the local claude CLI binary.
// workDir defaults to the process working directory; model defaults to DefaultModel.
func New(workDir, model string) (codeagent.CodeAgent, error) {
	binPath, err := lookPath("claude")
	if err != nil {
		return nil, fmt.Errorf("claude: binary not found in PATH: %w", err)
	}
	logger.Debug("claude binary located", "path", binPath)

	if workDir == "" {
		workDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("claude: resolve workdir: %w", err)
		}
	}
	if model == "" {
		model = DefaultModel
	}

	ver, _ := captureOutput(workDir, "claude", "--version")
	ver = strings.TrimSpace(ver)
	logger.Info("claude agent initialised", "workDir", workDir, "model", model, "version", ver)

	return &claudeAgent{
		ClaudeParser: &ClaudeParser{},
		resolver:     settings.New(Claude),
		workDir:      workDir,
		model:        model,
		permMode:     codeagent.PermissionDefault,
		info:         codeagent.CodeAgentInfo{Provider: Claude, Name: "claude", Version: ver},
	}, nil
}

// ============================================================
// Info / Identity
// ============================================================

func (a *claudeAgent) Info() *codeagent.CodeAgentInfo { return &a.info }

// GetUserIdentity checks login status via `claude auth status`.
// Exit code 0 means authenticated; non-zero means not logged in.
func (a *claudeAgent) GetUserIdentity() codeagent.UserIdentify {
	a.mu.RLock()
	workDir := a.workDir
	a.mu.RUnlock()

	cmd := exec.Command("claude", "auth", "status")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		logger.Debug("GetUserIdentity: not authenticated", "err", err)
		return codeagent.UserIdentify{Authenticated: false}
	}

	// `claude auth status` outputs JSON; parse email if present.
	raw := strings.TrimSpace(string(out))
	identity := parseAuthStatus(raw)
	logger.Debug("GetUserIdentity: authenticated", "email", identity.Email)
	return identity
}

// ============================================================
// Capabilities / Defaults / UpdateDefaults
// ============================================================

func (a *claudeAgent) Capabilities() (*codeagent.Capabilities, error) {
	return &codeagent.Capabilities{
		Hooks: &hooks.Capabilities{
			PreToolUse:         true,
			PostToolUse:        true,
			PostToolUseFailure: true,
			PreSessionStart:    true,
			PostSessionStart:   true,
			PrePrompt:          true,
			PostPrompt:         false,
		},
		Streaming:  true,
		MCPSupport: true,
		Worktrees:  true,
		Subagents:  true,
	}, nil
}

// TODO: read defaults from ~/.claude/settings.json
func (a *claudeAgent) Defaults() (*codeagent.Config, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return &codeagent.Config{
		Model:          codeagent.Model{Provider: Claude, Model: a.model},
		PermissionMode: a.permMode,
		Hooks:          nil,
		Sandbox:        a.sbx,
	}, nil
}

// TODO: persist to ~/.claude/settings.json
func (a *claudeAgent) UpdateDefaults(cfg *codeagent.Config) error {
	if cfg == nil {
		return errors.New("claude: UpdateDefaults: nil config")
	}
	a.mu.Lock()
	if cfg.Model.Model != "" {
		a.model = cfg.Model.Model
	}
	if cfg.PermissionMode != "" {
		a.permMode = cfg.PermissionMode
	}
	if cfg.Sandbox != nil {
		a.sbx = cfg.Sandbox
	}
	a.mu.Unlock()
	logger.Info("UpdateDefaults applied", "model", a.model, "permMode", a.permMode)
	return nil
}

// ============================================================
// SettingsResolver — delegated to settings.Resolver
// ============================================================

func (a *claudeAgent) GetUserSettings() (*codeagent.Settings, error) {
	path, err := claudeUserSettingsPath()
	if err != nil {
		return nil, err
	}
	return readClaudeSettings(path)
}

func (a *claudeAgent) GetWorkspaceSettings(dir sandbox.WorkspaceDir) (*codeagent.Settings, error) {
	return readClaudeSettings(claudeWorkspaceSettingsPath(string(dir)))
}

func (a *claudeAgent) SaveDefaultSettings(s *codeagent.Settings) error {
	path, err := claudeUserSettingsPath()
	if err != nil {
		return err
	}
	return writeClaudeSettings(path, s)
}

func (a *claudeAgent) WatchDefaultSettings(fn func(*codeagent.Settings)) error {
	return a.resolver.WatchDefaultSettings(func(_ *codeagent.Settings) {
		cfg, err := a.GetUserSettings()
		if err == nil {
			fn(cfg)
		}
	})
}

// Discover returns models listed in the user settings availableModels field.
// Falls back to StaticModels when the field is absent or empty.
func (a *claudeAgent) Discover() (codeagent.DiscoverResult, error) {
	models, err := a.resolver.DiscoverModels()
	if err != nil {
		logger.Warn("Discover: could not read settings, using static models", "err", err)
	}
	if len(models) == 0 {
		models = make([]string, len(StaticModels))
		copy(models, StaticModels)
	}
	ids := make([]codeagent.ModelID, len(models))
	for i, m := range models {
		ids[i] = codeagent.ModelID(m)
	}
	return codeagent.DiscoverResult{Models: ids}, nil
}

func (a *claudeAgent) syncSandboxRuntimeLocked() error {
	if a.sbx == nil {
		a.sbxRuntime = nil
		a.sbxProvisioner = nil
		return nil
	}

	if a.sbxRuntime != nil {
		return a.sbxRuntime.Sync(a.sbx)
	}

	supported := rootsandbox.SupportedProvisioners(runtime.GOOS)
	if len(supported) == 0 {
		return fmt.Errorf("claude: sandbox runtime: no supported provisioner for %s", runtime.GOOS)
	}

	provisioner, err := rootsandbox.NewProvisioner(supported[0], &sandbox.Sandbox{
		Config: a.sbx,
	}, rootsandbox.ProvisionerOptions{
		WorkDir: a.workDir,
	})
	if err != nil {
		return fmt.Errorf("claude: sandbox runtime: create provisioner: %w", err)
	}

	id := a.sessionID
	if strings.TrimSpace(id) == "" {
		id = "claude-sandbox-" + generateID()
	}
	rt, err := provisioner.Create(rootsandbox.CreateSandboxParams{
		ID:     id,
		Config: a.sbx,
	})
	if err != nil {
		return fmt.Errorf("claude: sandbox runtime: create runtime: %w", err)
	}

	a.sbxProvisioner = provisioner
	a.sbxRuntime = rt
	return nil
}
