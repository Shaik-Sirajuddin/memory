package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	applog "github.com/Shaik-Sirajuddin/memory/mcp/log"
	"github.com/Shaik-Sirajuddin/memory/mcp/server"
)

func main() {
	addr := flag.String("addr", envDefault("MCP_ADDR", ":8100"), "server listen address")
	interval := flag.Duration("interval", time.Minute, "interval for automatic hello sampling requests")
	transport := flag.String("transport", envDefault("MCP_TRANSPORT", "http"), "server transport: http or stdio")
	flag.Parse()
	intervalSet := flagWasSet("interval")

	if *transport == "stdio" {
		if !intervalSet {
			*interval = 10 * time.Second
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		applog.Logger.Info("mcp stdio server starting", "interval", interval.String(), "delivery_mode", server.DeliveryModeFromEnv(), "log_file", applog.LogFilePath)
		if err := server.NewStdio(*interval).Serve(ctx, os.Stdin, os.Stdout); err != nil && !errors.Is(err, context.Canceled) {
			applog.Logger.Error("mcp stdio server failed", "err", err)
			os.Exit(1)
		}
		return
	}
	if *transport != "http" {
		_, _ = fmt.Fprintf(os.Stderr, "unsupported transport %q\n", *transport)
		os.Exit(2)
	}

	srv := server.New(*interval)
	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		applog.Logger.Info("mcp server starting", "addr", *addr, "interval", interval.String(), "log_file", applog.LogFilePath)
		errCh <- httpServer.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-stop:
		applog.Logger.Info("mcp server stopping", "signal", sig.String())
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			applog.Logger.Error("mcp server failed", "err", err)
			os.Exit(1)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		applog.Logger.Error("mcp server shutdown failed", "err", err)
		os.Exit(1)
	}
	applog.Logger.Info("mcp server stopped")
}

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func flagWasSet(name string) bool {
	wasSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			wasSet = true
		}
	})
	return wasSet
}
