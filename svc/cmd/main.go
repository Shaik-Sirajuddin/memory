package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"syscall"

	omnilog "github.com/Shaik-Sirajuddin/memory/log"
)

var Version = "dev"

func main() {
	disablePTY := flag.Bool("disable-ptydaemon", false, "Disable the PTY daemon service")
	disableHook := flag.Bool("disable-hook-operator", false, "Disable the hook operator service")
	printVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *printVersion {
		fmt.Println(Version)
		os.Exit(0)
	}

	log := omnilog.NewLogger("component", "svc")
	username := currentUsername()

	mux := &ServiceMux{
		PTYDaemon: PTYDaemonConfig{
			ServiceConfig: ServiceConfig{Enabled: !*disablePTY},
			SocketPath:    envOr("OMNI_PTY_SOCKET", "/run/omni-"+username+"/omni-pty.sock"),
			DBPath:        envOr("PTYDAEMON_DB", "/var/lib/omni-"+username+"/ptydaemon.db"),
		},
		HookOperator: HookOperatorConfig{
			ServiceConfig: ServiceConfig{Enabled: !*disableHook},
			SocketPath:    envOr("HOOK_OPERATOR_SOCKET", "/run/omni-"+username+"/hook-operator.sock"),
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := mux.Run(ctx, log); err != nil {
		log.Error("service error", "err", err)
		os.Exit(1)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func currentUsername() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	if v := os.Getenv("USER"); v != "" {
		return v
	}
	return "omni"
}
