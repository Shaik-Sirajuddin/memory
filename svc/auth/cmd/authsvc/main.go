package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.com/m/v2/internal/app"
)

func main() {
	cfg := app.MustLoadConfig()
	server, err := app.New(cfg)
	if err != nil {
		log.Fatalf("bootstrap failed: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run()
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			log.Fatalf("server stopped: %v", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
