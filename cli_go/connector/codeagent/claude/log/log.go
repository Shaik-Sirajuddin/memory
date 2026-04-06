package log

import (
	"log/slog"
	"os"
)

func newLogger(connector string) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: true,
	})).With("connector", connector)
}
