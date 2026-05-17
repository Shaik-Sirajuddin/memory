// Package log provides a shared logger whose level mirrors omni/log:
// resolved from OmniConfig.Dev.Debug, with DEV env var as an override.
package log

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
)

// Logger is the module-wide structured logger for the ptydaemon component.
var Logger = NewLogger("component", "ptydaemon")

// NewLogger returns a structured logger tagged with the given key/value pair.
// Matches the omni/log.NewLogger signature so both modules share one convention.
func NewLogger(key, component string) *slog.Logger {
	level := resolveLevel()
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:     level,
		AddSource: level == slog.LevelDebug,
	})).With(key, component)
}

// resolveLevel checks DEV env var first (used by the systemd unit when DEBUG=1
// is passed at install time), then falls back to OmniConfig.Dev.Debug so it
// stays in sync with 'omni config set dev.debug true'.
func resolveLevel() slog.Level {
	if os.Getenv("DEV") != "" {
		return slog.LevelDebug
	}
	if debug, err := readOmniDebug(); err == nil && debug {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}

// readOmniDebug reads Dev.Debug from the same config file that omni/log reads.
func readOmniDebug() (bool, error) {
	dir, err := xdgConfigHome()
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "omni", "config.json"))
	if err != nil {
		return false, err
	}
	var cfg struct {
		Dev *struct {
			Debug bool `json:"debug"`
		} `json:"dev,omitempty"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return false, err
	}
	return cfg.Dev != nil && cfg.Dev.Debug, nil
}

func xdgConfigHome() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config"), nil
}
