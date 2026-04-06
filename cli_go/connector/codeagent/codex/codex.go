package codex

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent/hooks"
	"github.com/Shaik-Sirajuddin/memory/connector/sandbox"
)

type ConfigPaths struct {
	GlobalConfigDirs    []string
	WorkspaceConfigDirs []string
}

var config ConfigPaths = ConfigPaths{
	GlobalConfigDirs: []string{
		".codex",
	},
	WorkspaceConfigDirs: []string{
		".codex",
	},
}

// logger is the package-level structured logger for the codex connector.
var logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
	Level:     slog.LevelDebug,
	AddSource: true,
})).With("connector", "codex")

// ============================================================
// Available models
// ============================================================

// ModelID is a Codex-compatible OpenAI model identifier.
type ModelID = string

const (
	ModelO4Mini    ModelID = "o4-mini"
	ModelO3        ModelID = "o3"
	ModelGPT41     ModelID = "gpt-4.1"
	ModelGPT41Mini ModelID = "gpt-4.1-mini"
	ModelGPT4o     ModelID = "gpt-4o"
)

// StaticModels is the built-in list of models known to work with Codex.
// Returned by FetchModels when the CLI cannot provide a dynamic list.
var StaticModels = []ModelID{ModelO4Mini, ModelO3, ModelGPT41, ModelGPT41Mini, ModelGPT4o}

// DefaultModel is used when the caller does not specify a model.
const DefaultModel = ModelO4Mini

// ============================================================
// Agent struct
// ============================================================

type codexAgent struct {
	mu              sync.RWMutex
	workDir         string
	model           string
	sessionID       string
	sbx             *sandbox.Sandbox
	info            codeagent.CodeAgentInfo
	registeredHooks []*hooks.HookData
}

// New returns a CodeAgent backed by the local codex CLI binary.
// workDir defaults to the process working directory; model defaults to DefaultModel.
func New(workDir, model string) (codeagent.CodeAgent, error) {
	binPath, err := lookPath("codex")
	if err != nil {
		return nil, fmt.Errorf("codex: binary not found in PATH: %w", err)
	}
	logger.Debug("codex binary located", "path", binPath)

	if workDir == "" {
		workDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("codex: resolve workdir: %w", err)
		}
	}
	if model == "" {
		model = DefaultModel
	}

	ver, _ := captureOutput(workDir, "codex", "--version")
	ver = trimSpace(ver)
	logger.Info("codex agent initialised", "workDir", workDir, "model", model, "version", ver)

	return &codexAgent{
		workDir: workDir,
		model:   model,
		info:    codeagent.CodeAgentInfo{Provider: Codex, Name: "codex", Version: ver},
	}, nil
}

// ============================================================
// Info / Identity
// ============================================================

func (a *codexAgent) Info() *codeagent.CodeAgentInfo { return &a.info }

// GetUserIdentity verifies login status by running `codex auth status`.
// Exit code 0 means the user is authenticated; any non-zero exit means not logged in.
// No API keys or credential files are read.
func (a *codexAgent) GetUserIdentity() codeagent.UserIdentify {
	a.mu.RLock()
	workDir := a.workDir
	a.mu.RUnlock()

	cmd := exec.Command("codex", "auth", "status")
	cmd.Dir = workDir
	if err := cmd.Run(); err != nil {
		logger.Debug("GetUserIdentity: not authenticated", "err", err)
		return codeagent.UserIdentify{Authenticated: false}
	}

	logger.Debug("GetUserIdentity: authenticated via codex auth status")
	return codeagent.UserIdentify{Authenticated: true}
}

// ============================================================
// Capabilities / Defaults / UpdateDefaults
// ============================================================

func (a *codexAgent) Capabilities() (*codeagent.Capabilities, error) {
	return &codeagent.Capabilities{
		Hooks: &hooks.Capabilities{
			PreToolUse: true, PostToolUse: true, PostToolUseFailure: true,
			PreSessionStart: true, PostSessionStart: false,
			PrePrompt: true, PostPrompt: false,
		},
		Streaming: true, MCPSupport: false, Worktrees: false, Subagents: false,
	}, nil
}

// Defaults reads the current defaults from ~/.codex/config.yaml, falling back
// to in-memory values when the file is absent or a key is missing.
func (a *codexAgent) Defaults() (*codeagent.Config, error) {
	a.mu.RLock()
	model := a.model
	sbx := a.sbx
	a.mu.RUnlock()

	cfg, err := readGlobalConfig()
	if err != nil {
		logger.Warn("Defaults: could not read global config, using in-memory values", "err", err)
	} else {
		if m, ok := cfg["model"]; ok && m != "" {
			model = m
		}
		if s, ok := cfg["sandbox"]; ok {
			sbx = sandboxFromFlag(s)
		}
	}

	return &codeagent.Config{
		Model:          codeagent.Model{Provider: Codex, Model: model},
		PermissionMode: codeagent.PermissionDefault,
		Sandbox:        sbx,
	}, nil
}

// UpdateDefaults applies cfg to in-memory state and persists the changes to
// ~/.codex/config.yaml so future interactive codex sessions inherit them.
func (a *codexAgent) UpdateDefaults(cfg *codeagent.Config) error {
	if cfg == nil {
		return fmt.Errorf("codex: UpdateDefaults: nil config")
	}
	a.mu.Lock()
	if cfg.Model.Model != "" {
		a.model = cfg.Model.Model
	}
	if cfg.Sandbox != nil {
		a.sbx = cfg.Sandbox
	}
	model := a.model
	sbx := a.sbx
	a.mu.Unlock()

	globalCfg, err := readGlobalConfig()
	if err != nil {
		globalCfg = map[string]string{}
		logger.Warn("UpdateDefaults: could not read global config, will overwrite", "err", err)
	}
	if model != "" {
		globalCfg["model"] = model
	}
	if sf := sandboxFlagValue(sbx); sf != "" {
		globalCfg["sandbox"] = sf
	} else {
		delete(globalCfg, "sandbox")
	}
	if err := writeGlobalConfig(globalCfg); err != nil {
		return fmt.Errorf("codex: UpdateDefaults: persist config: %w", err)
	}

	logger.Info("UpdateDefaults applied", "model", model)
	return nil
}

// ============================================================
// FetchModels — dynamic model list via codex CLI
//
// Codex CLI does not expose a dedicated models list command.
// We attempt `codex models --json` and `codex model list`; on any
// failure we return the curated StaticModels list.
// No HTTP calls are made.
// ============================================================

func (a *codexAgent) FetchModels(ctx context.Context) ([]ModelID, error) {
	a.mu.RLock()
	workDir := a.workDir
	a.mu.RUnlock()

	// Attempt 1: codex models --json
	if ids, err := fetchModelsViaCmd(ctx, workDir, "codex", "models", "--json"); err == nil {
		logger.Info("FetchModels: resolved via 'codex models --json'", "count", len(ids))
		return ids, nil
	}

	// Attempt 2: codex model list
	if ids, err := fetchModelsViaCmd(ctx, workDir, "codex", "model", "list"); err == nil {
		logger.Info("FetchModels: resolved via 'codex model list'", "count", len(ids))
		return ids, nil
	}

	logger.Warn("FetchModels: CLI model listing unavailable, using static list")
	return StaticModels, nil
}

// fetchModelsViaCmd runs the given CLI command and parses model IDs from its
// output. Accepts both JSON array `["m1","m2"]` and plain newline-delimited output.
func fetchModelsViaCmd(ctx context.Context, workDir string, name string, args ...string) ([]ModelID, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("codex: fetch models cmd: %w", err)
	}

	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, errors.New("codex: fetch models cmd: empty output")
	}

	// Try JSON array first.
	if strings.HasPrefix(raw, "[") {
		if ids := parseJSONStringSlice(raw); len(ids) > 0 {
			return ids, nil
		}
	}

	// Fall back to one model ID per line.
	var ids []ModelID
	for _, line := range strings.Split(raw, "\n") {
		if id := strings.TrimSpace(line); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, errors.New("codex: fetch models cmd: no models in output")
	}
	return ids, nil
}

// ============================================================
// HookManager
// ============================================================

func (a *codexAgent) SupportedHooks() (*hooks.Capabilities, error) {
	return &hooks.Capabilities{
		PreToolUse: true, PostToolUse: true, PostToolUseFailure: true,
		PreSessionStart: true, PostSessionStart: false,
		PrePrompt: true, PostPrompt: false,
	}, nil
}

// Register adds the hook to the in-memory list and appends it to the
// appropriate .codex/hooks.json file (global or workspace).
func (a *codexAgent) Register(p hooks.RegisterHookParams) error {
	if p.Data == nil {
		return errors.New("codex: register hook: nil HookData")
	}
	a.mu.Lock()
	a.registeredHooks = append(a.registeredHooks, p.Data)
	workDir := a.workDir
	a.mu.Unlock()

	filePath, err := hooksFilePath(p.Data.Path, workDir)
	if err != nil {
		return fmt.Errorf("codex: register hook: resolve path: %w", err)
	}
	hf, err := readHooksFile(filePath)
	if err != nil {
		return fmt.Errorf("codex: register hook: read hooks file: %w", err)
	}
	hf.Hooks = append(hf.Hooks, hookDataToEntry(p.Data))
	if err := writeHooksFile(filePath, hf); err != nil {
		return fmt.Errorf("codex: register hook: write hooks file: %w", err)
	}

	logger.Info("Register: hook registered", "uid", p.Data.UID, "id", p.Data.Info.ID, "file", filePath)
	return nil
}

// GetRegisteredHooks reads hooks from both the global ~/.codex/hooks.json and
// the workspace .codex/hooks.json, merges them with the in-memory list, and
// returns a deduplicated slice (by UID). File entries take precedence so that
// hooks registered outside this agent instance are also visible.
func (a *codexAgent) GetRegisteredHooks() []*hooks.HookData {
	a.mu.RLock()
	workDir := a.workDir
	inMem := make([]*hooks.HookData, len(a.registeredHooks))
	copy(inMem, a.registeredHooks)
	a.mu.RUnlock()

	seen := map[string]struct{}{}
	var merged []*hooks.HookData

	// Helper: append entries from a hooks file under a given path tag.
	addFromFile := func(filePath string, path hooks.HookPath) {
		hf, err := readHooksFile(filePath)
		if err != nil {
			logger.Warn("GetRegisteredHooks: could not read hooks file", "path", filePath, "err", err)
			return
		}
		for _, e := range hf.Hooks {
			if _, dup := seen[e.UID]; dup {
				continue
			}
			seen[e.UID] = struct{}{}
			merged = append(merged, entryToHookData(e, path))
		}
	}

	// 1. Global hooks.
	if globalDir, err := globalCodexDir(); err == nil {
		addFromFile(filepath.Join(globalDir, "hooks.json"), hooks.HookPath{Global: true})
	}

	// 2. Workspace hooks.
	addFromFile(filepath.Join(workDir, ".codex", "hooks.json"), hooks.HookPath{WorkspaceDir: &workDir})

	// 3. In-memory hooks not already found in files.
	for _, h := range inMem {
		if _, dup := seen[h.UID]; dup {
			continue
		}
		seen[h.UID] = struct{}{}
		merged = append(merged, h)
	}

	return merged
}

// DeleteHook removes the hook matching p.UID from the in-memory list and from
// the appropriate .codex/hooks.json file (global or workspace per p flags).
func (a *codexAgent) DeleteHook(p hooks.DeleteHookParams) (bool, error) {
	a.mu.Lock()
	workDir := a.workDir
	found := false
	for i, h := range a.registeredHooks {
		if h.UID == p.UID {
			a.registeredHooks = append(a.registeredHooks[:i], a.registeredHooks[i+1:]...)
			found = true
			break
		}
	}
	a.mu.Unlock()

	// Always attempt to remove from the hooks file regardless of in-memory state,
	// so hooks registered outside this process are also cleaned up.
	path := hooks.HookPath{Global: p.Global, WorkspaceDir: p.WorkspaceDir}
	filePath, err := hooksFilePath(path, workDir)
	if err != nil {
		logger.Warn("DeleteHook: could not resolve hooks file path", "err", err)
	} else {
		hf, readErr := readHooksFile(filePath)
		if readErr == nil {
			before := len(hf.Hooks)
			filtered := hf.Hooks[:0]
			for _, e := range hf.Hooks {
				if e.UID != p.UID {
					filtered = append(filtered, e)
				}
			}
			hf.Hooks = filtered
			if len(hf.Hooks) < before {
				found = true
				if writeErr := writeHooksFile(filePath, hf); writeErr != nil {
					logger.Error("DeleteHook: could not write hooks file", "err", writeErr)
				}
			}
		}
	}

	if !found {
		logger.Warn("DeleteHook: hook not found", "uid", p.UID)
		return false, fmt.Errorf("codex: delete hook: uid %q not found", p.UID)
	}
	logger.Info("DeleteHook: removed", "uid", p.UID)
	return true, nil
}
