package gemini

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
	"github.com/Shaik-Sirajuddin/memory/connector/sandbox"
)

// sandboxFlagValue maps sandbox policy to Gemini settings value.
func sandboxFlagValue(s *sandbox.Sandbox) string {
	if s == nil {
		return ""
	}
	if s.AgentPolicy != nil && sandbox.AgentFSPolicy(s.AgentPolicy.FSPolicy) == sandbox.Inherit {
		return "danger-full-access"
	}
	return "read-only"
}

func (a *geminiAgent) GetSessionSandbox(_ codeagent.GetSessionSandboxParams) (*codeagent.GetSessionSandboxResult, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return &codeagent.GetSessionSandboxResult{Sandbox: a.sbx}, nil
}

func (a *geminiAgent) UpdateSessionSandbox(p codeagent.UpdateSessionSandboxParams) (*codeagent.UpdateSessionSandboxResult, error) {
	a.mu.Lock()
	a.sbx = p.Sandbox
	a.mu.Unlock()

	if err := a.syncSandboxConfig(); err != nil {
		return nil, fmt.Errorf("gemini: update sandbox: sync config: %w", err)
	}

	return &codeagent.UpdateSessionSandboxResult{Sandbox: p.Sandbox}, nil
}

func syncModelAndModeConfig(workDir, model string, mode codeagent.PermissionMode) error {
	return writeWorkspaceSettings(workDir, func(s *geminiSettingsFile) {
		if model != "" {
			s.Model = model
		}
		if am := approvalModeFlag(mode); am != "" {
			s.ApprovalMode = am
		}
	})
}

func (a *geminiAgent) syncSandboxConfig() error {
	a.mu.RLock()
	workDir := a.workDir
	sbx := a.sbx
	a.mu.RUnlock()

	flag := sandboxFlagValue(sbx)
	return writeWorkspaceSettings(workDir, func(s *geminiSettingsFile) {
		s.Sandbox = flag
	})
}

func writeWorkspaceSettings(workDir string, mutateFn func(*geminiSettingsFile)) error {
	settingsDir := filepath.Join(workDir, ".gemini")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		return fmt.Errorf("gemini: mkdir %s: %w", settingsDir, err)
	}

	settingsPath := filepath.Join(settingsDir, "settings.json")
	settings, err := readSettingsFile(settingsPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if settings.Hooks == nil {
		settings.Hooks = map[string][]geminiHookMatcher{}
	}

	mutateFn(&settings)
	return writeSettingsFile(settingsPath, settings)
}
