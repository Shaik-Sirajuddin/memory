package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

// OmniConfigResolver resolves default settings saved at global level.
type OmniConfigResolver interface {
	GetUserSettings() (*OmniConfig, error)
	SaveUserSettings(*OmniConfig) error
}

// DefaultOmniConfigResolver implements [OmniConfigResolver].
type DefaultOmniConfigResolver struct {
	// ConfigPath is optional and primarily useful for tests.
	// When empty, an XDG-compliant default path is used.
	ConfigPath string
}

// UserConfigPath resolves the persisted omni config file location.
func (r *DefaultOmniConfigResolver) UserConfigPath() (string, error) {
	if r != nil && r.ConfigPath != "" {
		return r.ConfigPath, nil
	}
	return xdg.ConfigFile("omni/config.json")
}

// GetUserSettings reads persisted user settings.
// Missing config file is treated as default config.
func (r *DefaultOmniConfigResolver) GetUserSettings() (*OmniConfig, error) {
	path, err := r.UserConfigPath()
	if err != nil {
		return nil, fmt.Errorf("config: resolve user config path: %w", err)
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ProvisionDefaultOmniConfig(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	var cfg OmniConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	return ApplyOmniConfigDefaults(&cfg), nil
}

// SaveUserSettings writes user settings to the persisted config path.
func (r *DefaultOmniConfigResolver) SaveUserSettings(cfg *OmniConfig) error {
	path, err := r.UserConfigPath()
	if err != nil {
		return fmt.Errorf("config: resolve user config path: %w", err)
	}

	normalized := ApplyOmniConfigDefaults(cfg)
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshal %s: %w", path, err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("config: mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("config: write %s: %w", path, err)
	}

	return nil
}
