// Package log provides a shared logger factory whose level is driven by
// config.OmniConfig.Dev.Debug.
//
//   - Dev.Debug == true  → slog.LevelDebug  (source attribution enabled)
//   - Dev.Debug == false → slog.LevelInfo   (source attribution disabled)
package log

import (
	"log/slog"
	"os"

	"github.com/Shaik-Sirajuddin/memory/config"
)

// NewLogger returns a structured logger tagged with the given component key and
// value. The log level is resolved once at construction time from the persisted
// OmniConfig:
//
//   - OmniConfig.Dev.Debug == true  → Debug level, source attribution on
//   - otherwise                     → Info level
//
// If the config file cannot be read the logger defaults to Info level so that
// misconfigured or first-run environments are never silenced.
func NewLogger(key, component string) *slog.Logger {
	level := resolveLevel()
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:     level,
		AddSource: level == slog.LevelDebug,
	})).With(key, component)
}

// resolveLevel reads OmniConfig and returns the appropriate slog.Level.
func resolveLevel() slog.Level {
	r := &config.DefaultOmniConfigResolver{}
	cfg, err := r.GetUserSettings()
	if err != nil {
		return slog.LevelInfo
	}
	if cfg.Dev != nil && cfg.Dev.Debug {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}
