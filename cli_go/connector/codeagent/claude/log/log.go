package log

import (
	"log/slog"
	"os"

	"github.com/Shaik-Sirajuddin/memory/config"
)

// level is the shared, mutable log level for all loggers created by this package.
// It defaults to LevelInfo and is promoted to LevelDebug when dev.debug is true.
var level = &slog.LevelVar{} // default: LevelInfo (0)

func init() {
	level.Set(slog.LevelInfo)
}

// Configure updates the package-level log level from OmniConfig.
// Call this once after the config is loaded; all existing loggers pick up the change.
func Configure(cfg *config.OmniConfig) {
	if cfg != nil && cfg.Dev != nil && cfg.Dev.Debug {
		level.Set(slog.LevelDebug)
	} else {
		level.Set(slog.LevelInfo)
	}
}

// NewLogger returns a structured logger tagged with the given connector name.
// The log level is controlled by the package-level LevelVar; call Configure to
// set it from OmniConfig.Dev.Debug after config load.
//
// connector-specific key pairs: "connector" = connector name.
func NewLogger(connector string) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	})).With("connector", connector)
}
