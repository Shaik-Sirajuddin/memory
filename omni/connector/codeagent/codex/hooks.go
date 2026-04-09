package codex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Shaik-Sirajuddin/memory/connector/codeagent/hooks"
	sandbox "github.com/Shaik-Sirajuddin/memory/sandbox/provider"
)

// ============================================================
// Codex hooks.json on-disk format
// ============================================================

// codexHookEntry is the per-hook record stored in .codex/hooks.json.
// The `uid` field is owned by this connector — codex CLI ignores it but
// it is required so we can match entries for deletion by UID.
type codexHookEntry struct {
	UID     string   `json:"uid"`
	Event   string   `json:"event"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	Timeout int      `json:"timeout,omitempty"`
	Url     *string  `json:"url,omitempty"`
}

type codexHooksFile struct {
	Hooks []codexHookEntry `json:"hooks"`
}

// ============================================================
// Path resolution
// ============================================================

// globalCodexDir returns ~/.codex, creating it if absent.
func globalCodexDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("codex: resolve home dir: %w", err)
	}
	return filepath.Join(home, ".codex"), nil
}

// globalConfigPath returns ~/.codex/config.toml.
func globalConfigPath() (string, error) {
	dir, err := globalCodexDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// hooksFilePath maps a HookPath to the absolute .codex/hooks.json path.
// Global → ~/.codex/hooks.json; workspace → <WorkspaceDir>/.codex/hooks.json.
func hooksFilePath(path hooks.HookPath, fallbackDir string) (string, error) {
	if path.Global {
		dir, err := globalCodexDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, "hooks.json"), nil
	}
	dir := fallbackDir
	if path.WorkspaceDir != nil && *path.WorkspaceDir != "" {
		dir = *path.WorkspaceDir
	}
	return filepath.Join(dir, ".codex", "hooks.json"), nil
}

// ============================================================
// Global config.toml read / write
// ============================================================

// readGlobalConfig reads the top-level scalar keys from ~/.codex/config.toml.
// Returns an empty map if the file does not exist.
func readGlobalConfig() (map[string]string, error) {
	path, err := globalConfigPath()
	if err != nil {
		return nil, err
	}
	cfg := map[string]string{}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("codex: read global config: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			cfg[strings.TrimSpace(parts[0])] = strings.Trim(strings.TrimSpace(parts[1]), "\"")
		}
	}
	return cfg, nil
}

// writeGlobalConfig serialises cfg to ~/.codex/config.toml.
func writeGlobalConfig(cfg map[string]string) error {
	path, err := globalConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("codex: mkdir global config dir: %w", err)
	}
	var sb strings.Builder
	for k, v := range cfg {
		sb.WriteString(fmt.Sprintf("%s = %q\n", k, v))
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return fmt.Errorf("codex: write global config: %w", err)
	}
	return nil
}

// ============================================================
// hooks.json read / write
// ============================================================

func readHooksFile(path string) (codexHooksFile, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return codexHooksFile{}, nil
	}
	if err != nil {
		return codexHooksFile{}, fmt.Errorf("codex: read hooks file %s: %w", path, err)
	}
	var f codexHooksFile
	if err := json.Unmarshal(data, &f); err != nil {
		return codexHooksFile{}, fmt.Errorf("codex: parse hooks file %s: %w", path, err)
	}
	return f, nil
}

func writeHooksFile(path string, f codexHooksFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("codex: mkdir hooks dir: %w", err)
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("codex: marshal hooks: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("codex: write hooks file %s: %w", path, err)
	}
	return nil
}

// ============================================================
// HookData ↔ codexHookEntry conversion
// ============================================================

func hookDataToEntry(h *hooks.HookData) codexHookEntry {
	return codexHookEntry{
		UID:     h.UID,
		Event:   string(h.Info.ID),
		Command: h.Info.Command,
		Args:    h.Info.Args,
		Timeout: h.Info.Timeout,
		Url:     h.Info.Url,
	}
}

func entryToHookData(e codexHookEntry, path hooks.HookPath) *hooks.HookData {
	return &hooks.HookData{
		UID:  e.UID,
		Path: path,
		Info: &hooks.HookInfo{
			ID:      hooks.HookID(e.Event),
			Command: e.Command,
			Args:    e.Args,
			Timeout: e.Timeout,
			Url:     e.Url,
		},
	}
}

// ============================================================
// Sandbox flag ↔ *sandbox.Config
// ============================================================

// sandboxFromFlag reconstructs a *sandbox.Config from the stored config flag value.
func sandboxFromFlag(flag string) *sandbox.Config {
	switch flag {
	case "danger-full-access":
		return &sandbox.Config{
			AgentPolicy: &sandbox.Policy{FSPolicy: sandbox.FSPolicy(sandbox.AllPermissiveRead)},
		}
	case "read-only":
		return &sandbox.Config{
			AgentPolicy: &sandbox.Policy{FSPolicy: sandbox.FSPolicy(sandbox.PermissiveRead)},
		}
	default:
		return nil
	}
}
