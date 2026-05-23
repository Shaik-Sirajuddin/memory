package operator

import (
	"log/slog"

	applog "github.com/Shaik-Sirajuddin/memory/pkg/log"
)

var logger = newLogger()

func newLogger() *slog.Logger {
	return applog.NewLogger("component", "operator")
}
