package hookoperator

import (
	applog "github.com/Shaik-Sirajuddin/memory/log"
	"log/slog"
)

var logger *slog.Logger = applog.NewLogger("component", "hook-operator")
