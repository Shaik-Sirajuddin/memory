// Package log provides the claude connector logger.
// Level is driven by config.OmniConfig.Dev.Debug via the shared log package.
package log

import (
	"log/slog"

	applog "github.com/Shaik-Sirajuddin/memory/log"
)

// NewLogger returns a connector-tagged logger whose level reflects the debug
// flag in the persisted OmniConfig (Debug→LevelDebug, else LevelInfo).
func NewLogger(connector string) *slog.Logger {
	return applog.NewLogger("connector", connector)
}
