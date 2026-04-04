package gemini

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Shaik-Sirajuddin/memory/connector/codeagent/hooks"
	"github.com/Shaik-Sirajuddin/memory/connector/sandbox"
)

type geminiHookDefinition struct {
	UID     string   `json:"uid,omitempty"`
	Type    string   `json:"type"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	URL     string   `json:"url,omitempty"`
	Timeout int      `json:"timeout,omitempty"`
}

type geminiHookMatcher struct {
	Matcher    string                 `json:"matcher,omitempty"`
	Sequential bool                   `json:"sequential,omitempty"`
	Hooks      []geminiHookDefinition `json:"hooks"`
}

type geminiSettingsFile struct {
	Model        string                         `json:"model,omitempty"`
	ApprovalMode string                         `json:"approvalMode,omitempty"`
	Sandbox      string                         `json:"sandbox,omitempty"`
	Hooks        map[string][]geminiHookMatcher `json:"hooks,omitempty"`
	RawExtra     map[string]json.RawMessage     `json:"-"`
}

var eventNameByHookID = map[hooks.HookID]string{
	hooks.PreToolUse:         "BeforeTool",
	hooks.PostToolUse:        "AfterTool",
	hooks.PostToolUseFailure: "AfterTool",
	hooks.PreSessionStart:    "SessionStart",
	hooks.PostSessionStart:   "SessionStart",
	hooks.PrePrompt:          "BeforeAgent",
	hooks.PostPrompt:         "AfterAgent",
}

var hookIDByEventName = map[string]hooks.HookID{
	"BeforeTool":   hooks.PreToolUse,
	"AfterTool":    hooks.PostToolUse,
	"SessionStart": hooks.PreSessionStart,
	"BeforeAgent":  hooks.PrePrompt,
	"AfterAgent":   hooks.PostPrompt,
}

func hookIDFromEvent(eventName string) (hooks.HookID, bool) {
	id, ok := hookIDByEventName[eventName]
	return id, ok
}

func globalGeminiDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("gemini: resolve home dir: %w", err)
	}
	return filepath.Join(home, ".gemini"), nil
}

func globalSettingsPath() (string, error) {
	dir, err := globalGeminiDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

func settingsPathForHookPath(path hooks.HookPath, fallbackDir string) string {
	if path.Global {
		if p, err := globalSettingsPath(); err == nil {
			return p
		}
	}
	dir := fallbackDir
	if path.WorkspaceDir != nil && *path.WorkspaceDir != "" {
		dir = *path.WorkspaceDir
	}
	return filepath.Join(dir, ".gemini", "settings.json")
}

func readGlobalSettings() (map[string]string, error) {
	path, err := globalSettingsPath()
	if err != nil {
		return nil, err
	}
	f, err := readSettingsFile(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	cfg := map[string]string{}
	if f.Model != "" {
		cfg["model"] = f.Model
	}
	if f.ApprovalMode != "" {
		cfg["approvalMode"] = f.ApprovalMode
	}
	if f.Sandbox != "" {
		cfg["sandbox"] = f.Sandbox
	}
	return cfg, nil
}

func writeGlobalSettings(cfg map[string]string) error {
	path, err := globalSettingsPath()
	if err != nil {
		return err
	}
	f := geminiSettingsFile{Hooks: map[string][]geminiHookMatcher{}}
	if existing, err := readSettingsFile(path); err == nil {
		f = existing
		if f.Hooks == nil {
			f.Hooks = map[string][]geminiHookMatcher{}
		}
	}

	f.Model = cfg["model"]
	f.ApprovalMode = cfg["approvalMode"]
	f.Sandbox = cfg["sandbox"]
	return writeSettingsFile(path, f)
}

func readSettingsFile(path string) (geminiSettingsFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return geminiSettingsFile{}, err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return geminiSettingsFile{}, fmt.Errorf("gemini: parse settings %s: %w", path, err)
	}

	f := geminiSettingsFile{Hooks: map[string][]geminiHookMatcher{}, RawExtra: map[string]json.RawMessage{}}
	for k, v := range raw {
		switch k {
		case "model":
			_ = json.Unmarshal(v, &f.Model)
		case "approvalMode":
			_ = json.Unmarshal(v, &f.ApprovalMode)
		case "sandbox":
			_ = json.Unmarshal(v, &f.Sandbox)
		case "hooks":
			_ = json.Unmarshal(v, &f.Hooks)
		default:
			f.RawExtra[k] = v
		}
	}
	if f.Hooks == nil {
		f.Hooks = map[string][]geminiHookMatcher{}
	}
	return f, nil
}

func writeSettingsFile(path string, f geminiSettingsFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("gemini: mkdir settings dir: %w", err)
	}

	raw := map[string]any{}
	for k, v := range f.RawExtra {
		var x any
		if err := json.Unmarshal(v, &x); err == nil {
			raw[k] = x
		}
	}
	if strings.TrimSpace(f.Model) != "" {
		raw["model"] = f.Model
	}
	if strings.TrimSpace(f.ApprovalMode) != "" {
		raw["approvalMode"] = f.ApprovalMode
	}
	if strings.TrimSpace(f.Sandbox) != "" {
		raw["sandbox"] = f.Sandbox
	}
	if len(f.Hooks) > 0 {
		raw["hooks"] = f.Hooks
	} else {
		delete(raw, "hooks")
	}

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("gemini: marshal settings: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("gemini: write settings %s: %w", path, err)
	}
	return nil
}

func syncHooksToSettings(workDir string, registered []*hooks.HookData) error {
	path := filepath.Join(workDir, ".gemini", "settings.json")
	f, err := readSettingsFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if f.Hooks == nil {
		f.Hooks = map[string][]geminiHookMatcher{}
	}

	newHooks := map[string][]geminiHookMatcher{}
	for _, h := range registered {
		if h == nil || h.Info == nil {
			continue
		}
		eventName, ok := eventNameByHookID[h.Info.ID]
		if !ok {
			continue
		}
		def := hookDataToDefinition(h)
		newHooks[eventName] = append(newHooks[eventName], geminiHookMatcher{Hooks: []geminiHookDefinition{def}})
	}
	f.Hooks = newHooks
	return writeSettingsFile(path, f)
}

func hookDataToDefinition(h *hooks.HookData) geminiHookDefinition {
	def := geminiHookDefinition{UID: h.UID, Timeout: h.Info.Timeout}
	if h.Info.Type == hooks.Webhook {
		def.Type = "http"
		if h.Info.Url != nil {
			def.URL = *h.Info.Url
		}
	} else {
		def.Type = "command"
		def.Command = h.Info.Command
		def.Args = append([]string{}, h.Info.Args...)
	}
	return def
}

func hookDefinitionToData(uid string, id hooks.HookID, d geminiHookDefinition, path hooks.HookPath) *hooks.HookData {
	info := &hooks.HookInfo{ID: id, Timeout: d.Timeout}
	if d.Type == "http" {
		info.Type = hooks.Webhook
		if d.URL != "" {
			u := d.URL
			info.Url = &u
		}
	} else {
		info.Type = hooks.CMD
		info.Command = d.Command
		info.Args = append([]string{}, d.Args...)
	}
	return &hooks.HookData{UID: uid, Path: path, Info: info}
}

// sandboxFromFlag reconstructs sandbox policy from stored settings.
func sandboxFromFlag(flag string) *sandbox.Sandbox {
	switch strings.TrimSpace(flag) {
	case "danger-full-access", "full-access":
		return &sandbox.Sandbox{ExtendedPolicy: &sandbox.ExtendedPolicy{}}
	case "read-only":
		return &sandbox.Sandbox{}
	default:
		return nil
	}
}
