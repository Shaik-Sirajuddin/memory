package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
	sandbox "github.com/Shaik-Sirajuddin/memory/sandbox/provider"
)

// sandboxFlagValue maps a *sandbox.Config to the --sandbox flag value codex accepts.
// nil → "" (flag omitted); AgentPolicy AllPermissiveRead → "danger-full-access"; else "read-only".
func sandboxFlagValue(s *sandbox.Config) string {
	if s == nil {
		return ""
	}
	if s.AgentPolicy != nil && s.AgentPolicy.FSPolicy == sandbox.FSPolicy(sandbox.AllPermissiveRead) {
		return "danger-full-access"
	}
	return "read-only"
}

// ============================================================
// GetSessionSandbox / UpdateSessionSandbox
// ============================================================

func (a *codexAgent) GetSessionSandbox(_ codeagent.GetSessionSandboxParams) (*codeagent.GetSessionSandboxResult, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	logger.Debug("GetSessionSandbox", "sandbox", a.sbx)
	return &codeagent.GetSessionSandboxResult{Sandbox: a.sbx}, nil
}

func (a *codexAgent) UpdateSessionSandbox(p codeagent.UpdateSessionSandboxParams) (*codeagent.UpdateSessionSandboxResult, error) {
	a.mu.Lock()
	a.sbx = p.Sandbox
	if err := a.syncSandboxRuntimeLocked(); err != nil {
		a.mu.Unlock()
		logger.Error("UpdateSessionSandbox: runtime sync failed", "err", err)
		return nil, fmt.Errorf("codex: update sandbox: sync runtime: %w", err)
	}
	a.mu.Unlock()

	if err := a.syncSandboxConfig(); err != nil {
		logger.Error("UpdateSessionSandbox: config sync failed", "err", err)
		return nil, fmt.Errorf("codex: update sandbox: sync config: %w", err)
	}

	logger.Info("UpdateSessionSandbox: updated", "flag", sandboxFlagValue(p.Sandbox))
	return &codeagent.UpdateSessionSandboxResult{Sandbox: p.Sandbox}, nil
}

// syncModelConfig writes the chosen model into .codex/config.yaml so that
// interactive `codex` sessions launched in workDir inherit the same model.
func syncModelConfig(workDir, model string) error {
	return writeCodexConfig(workDir, func(cfg map[string]string) {
		if model != "" {
			cfg["model"] = model
		} else {
			delete(cfg, "model")
		}
	})
}

// syncSandboxConfig performs the outbound sync: writes the resolved sandbox flag
// into .codex/config.toml in the active working directory so that interactive
// codex sessions launched later inherit the same sandbox policy.
func (a *codexAgent) syncSandboxConfig() error {
	a.mu.RLock()
	workDir := a.workDir
	sbx := a.sbx
	a.mu.RUnlock()

	flag := sandboxFlagValue(sbx)
	err := writeCodexConfig(workDir, func(cfg map[string]string) {
		if flag != "" {
			cfg["sandbox_mode"] = flag
		} else {
			delete(cfg, "sandbox_mode")
		}
	})
	if err != nil {
		return err
	}
	logger.Debug("syncSandboxConfig: wrote", "workDir", workDir, "sandbox", flag)
	return nil
}

// writeCodexConfig reads .codex/config.toml, applies mutateFn to the simple
// top-level string key/value map, and writes it back.
func writeCodexConfig(workDir string, mutateFn func(map[string]string)) error {
	configDir := filepath.Join(workDir, ".codex")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", configDir, err)
	}

	configPath := filepath.Join(configDir, "config.toml")

	existing := map[string]string{}
	if data, err := os.ReadFile(configPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.Trim(strings.TrimSpace(parts[1]), "\"")
				existing[key] = value
			}
		}
	}

	mutateFn(existing)

	var sb strings.Builder
	for k, v := range existing {
		sb.WriteString(fmt.Sprintf("%s = %q\n", k, v))
	}

	if err := os.WriteFile(configPath, []byte(sb.String()), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", configPath, err)
	}
	return nil
}
