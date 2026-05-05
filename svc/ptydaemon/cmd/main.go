package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Shaik-Sirajuddin/memory/svc/ptydaemon/internal"
)

func main() {
	socketPath := envOr("PTYDAEMON_SOCKET", "/tmp/ptydaemon.sock")
	dbPath := envOr("PTYDAEMON_DB", "/tmp/ptydaemon.db")

	store, err := internal.NewStore(dbPath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer store.Close()

	daemon := internal.NewDaemon(store)

	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	srv := &http.Server{Handler: internal.NewHandler(daemon)}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		fmt.Printf("ptydaemon listening on %s\n", socketPath)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		log.Fatalf("serve: %v", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = daemon.Shutdown(shutdownCtx)

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
