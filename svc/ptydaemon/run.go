package ptydaemon

import (
	"context"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/Shaik-Sirajuddin/memory/svc/ptydaemon/internal"
	ptylog "github.com/Shaik-Sirajuddin/memory/svc/ptydaemon/log"
	"github.com/Shaik-Sirajuddin/memory/svc/ptydaemon/ptyunix"
)

// DefaultSocketPath returns the per-user Unix socket path for the PTY daemon,
// honouring OMNI_PTY_SOCKET when set.
// Resolves to /run/omni-<username>/omni-pty.sock.
func DefaultSocketPath() string {
	if v := strings.TrimSpace(os.Getenv("OMNI_PTY_SOCKET")); v != "" {
		return v
	}
	if u, err := user.Current(); err == nil && u.Username != "" {
		return "/run/omni-" + u.Username + "/omni-pty.sock"
	}
	return "/run/omni-pty/omni-pty.sock"
}

// Run wires the store, daemon, and unix socket server, then blocks until ctx
// is cancelled. A 10-second graceful shutdown is attempted before returning.
func Run(ctx context.Context, socketPath, dbPath string) error {
	store, err := internal.NewStore(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	daemon := internal.NewDaemon(store)
	unixDaemon := ptyunix.NewDaemonWithInner(daemon)

	errCh := make(chan error, 1)
	go func() {
		if err := unixDaemon.ListenAndServe(ctx, socketPath); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		ptylog.Logger.Info("ptydaemon: shutdown signal received")
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := daemon.Shutdown(shutdownCtx); err != nil {
		ptylog.Logger.Error("ptydaemon: shutdown error", "err", err)
	}
	return nil
}
