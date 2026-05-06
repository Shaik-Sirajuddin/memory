package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/knadh/koanf/v2"
)

type defaultConfigParser struct{}

func (defaultConfigParser) Load(filePath string) (*Config, error) {
	path, err := validateConfigPath(filePath)
	if err != nil {
		return nil, err
	}
	k := koanf.New(".")
	if err := k.Load(jsonFileProvider{path: path}, jsonConfigParser{}); err != nil {
		return nil, fmt.Errorf("sandbox: load config %s: %w", path, err)
	}
	cfg := &Config{}
	if err := k.UnmarshalWithConf("", cfg, koanf.UnmarshalConf{Tag: "json"}); err != nil {
		return nil, fmt.Errorf("sandbox: unmarshal config %s: %w", path, err)
	}
	parser := defaultConfigParser{}
	if err := parser.Validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (defaultConfigParser) Validate(config *Config) error {
	if config == nil {
		return fmt.Errorf("sandbox: config is required")
	}
	if err := validatePolicy("workspace_policy", config.WorkSpacePolicy); err != nil {
		return err
	}
	if err := validatePolicy("agent_policy", config.AgentPolicy); err != nil {
		return err
	}
	return nil
}

func (defaultConfigParser) Save(config *Config, filePath string) error {
	path, err := validateConfigPath(filePath)
	if err != nil {
		return err
	}
	cfg := config
	if cfg == nil {
		cfg = &Config{}
	}
	parser := defaultConfigParser{}
	if err := parser.Validate(cfg); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("sandbox: marshal config %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("sandbox: create config dir %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("sandbox: write config %s: %w", path, err)
	}
	return nil
}

type jsonFileProvider struct {
	path string
}

func (p jsonFileProvider) ReadBytes() ([]byte, error) {
	raw, err := os.ReadFile(p.path)
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return []byte("{}"), nil
	}
	return raw, nil
}

func (p jsonFileProvider) Read() (map[string]any, error) {
	raw, err := p.ReadBytes()
	if err != nil {
		return nil, err
	}
	return jsonConfigParser{}.Unmarshal(raw)
}

type jsonConfigParser struct{}

func (jsonConfigParser) Unmarshal(raw []byte) (map[string]any, error) {
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (jsonConfigParser) Marshal(in map[string]any) ([]byte, error) {
	if in == nil {
		in = map[string]any{}
	}
	return json.Marshal(in)
}

func validateConfigPath(filePath string) (string, error) {
	if strings.TrimSpace(filePath) == "" {
		return "", fmt.Errorf("sandbox: file path is required")
	}
	return filepath.Clean(filePath), nil
}

func validatePolicy(name string, policy *Policy) error {
	if policy == nil {
		return nil
	}
	switch AgentFSPolicy(policy.FSPolicy) {
	case "", AllPermissiveRead, PermissiveRead, NonDependent, Inherit:
	default:
		return fmt.Errorf("sandbox: invalid %s fs_policy %q", name, policy.FSPolicy)
	}
	for _, dir := range policy.Config.AccessDirs {
		if strings.TrimSpace(dir) == "" {
			return fmt.Errorf("sandbox: %s access_dirs contains empty path", name)
		}
	}
	for _, dir := range policy.Config.BlockedDirs {
		if strings.TrimSpace(dir) == "" {
			return fmt.Errorf("sandbox: %s blocked_dirs contains empty path", name)
		}
	}
	return nil
}
