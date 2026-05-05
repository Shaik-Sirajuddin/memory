package operator

import (
	"log/slog"

	applog "github.com/Shaik-Sirajuddin/memory/log"
)

var logger = newLogger()

func newLogger() *slog.Logger {
	return applog.NewLogger("component", "operator")
}
