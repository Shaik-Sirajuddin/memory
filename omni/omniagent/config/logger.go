package config

import (
	"log/slog"
	"os"
)

// NewLogger returns a slog.Logger configured for the given OmniConfig.
// When OmniConfig.Dev.Debug is true the level is set to Debug, otherwise Info.
func NewLogger(cfg OmniConfig) *slog.Logger {
	level := slog.LevelInfo
	if cfg.Dev.Debug {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))
}
