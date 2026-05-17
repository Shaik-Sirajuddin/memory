package main

import (
	"context"
	"os"
	"os/signal"
	"os/user"
	"syscall"

	ptydaemon "github.com/Shaik-Sirajuddin/memory/svc/ptydaemon"
	ptylog "github.com/Shaik-Sirajuddin/memory/svc/ptydaemon/log"
)

func main() {
	socketPath := ptydaemon.DefaultSocketPath()
	dbPath := envOr("PTYDAEMON_DB", "/var/lib/omni-"+currentUsername()+"/ptydaemon.db")

	ptylog.Logger.Info("ptydaemon starting", "socket", socketPath, "db", dbPath)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := ptydaemon.Run(ctx, socketPath, dbPath); err != nil {
		ptylog.Logger.Error("ptydaemon error", "err", err)
		os.Exit(1)
	}

	ptylog.Logger.Info("ptydaemon stopped")
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
	return "pty"
}
