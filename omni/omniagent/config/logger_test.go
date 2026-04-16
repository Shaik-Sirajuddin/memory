package config

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewLoggerLevelTracksDebugConfig(t *testing.T) {
	t.Run("InfoByDefault", func(t *testing.T) {
		logger := NewLogger(OmniConfig{})
		assert.False(t, logger.Enabled(context.Background(), slog.LevelDebug), "Logger should not enable debug level when Dev.Debug is false")
		assert.True(t, logger.Enabled(context.Background(), slog.LevelInfo), "Logger should enable info level when Dev.Debug is false")
	})

	t.Run("DebugWhenEnabled", func(t *testing.T) {
		logger := NewLogger(OmniConfig{
			Dev: DevConfig{Debug: true},
		})
		assert.True(t, logger.Enabled(context.Background(), slog.LevelDebug), "Logger should enable debug level when Dev.Debug is true")
	})
}
