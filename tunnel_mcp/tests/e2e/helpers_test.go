//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"os"
	"sync"
	"testing"
)

type testConfig struct {
	exec      CommandExecutor
	omniPath  string
	workspace string
	container string
}

func newConfig(t *testing.T) testConfig {
	t.Helper()
	target := envOr("E2E_TARGET", "docker")
	ctr := envOr("E2E_CONTAINER", "omni-e2e-ubuntu-1")

	var ex CommandExecutor
	switch target {
	case "local":
		ex = &HostExecutor{}
	default:
		ex = newDockerExecutor(t, ctr)
	}

	workspace := envOr("OMNI_WORKSPACE", "/build")
	if target == "local" {
		if wd, err := os.Getwd(); err == nil {
			workspace = envOr("OMNI_WORKSPACE", wd)
		}
	}

	return testConfig{
		exec:      ex,
		omniPath:  envOr("OMNI_BIN", "omni"),
		workspace: workspace,
		container: ctr,
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// syncBuffer is a bytes.Buffer safe for concurrent reads and writes.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *syncBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf.Bytes()...)
}

// captureLog starts streaming `journalctl -fu omni@root` via the executor into
// a buffer. Returns a stop func (cancels the stream, waits for goroutine) and
// the live buffer. Always called first in each test — journalctl is the primary
// observation pipe.
func captureLog(t *testing.T, cfg testConfig) (stop func(), buf *syncBuffer) {
	t.Helper()
	buf = &syncBuffer{}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = cfg.exec.StreamCommand(ctx, buf, []string{"journalctl", "-f", "--no-pager", "-t", "omni-server"})
	}()

	stop = func() {
		cancel()
		<-done
	}
	t.Cleanup(stop)
	return stop, buf
}

// runOmni executes an omni subcommand and returns the output.
// It fails the test if the exit code is non-zero.
func runOmni(t *testing.T, cfg testConfig, args ...string) string {
	t.Helper()
	cmd := append([]string{cfg.omniPath}, args...)
	exitCode, out, err := cfg.exec.RunCommand(context.Background(), cmd)
	t.Logf("omni %v → exit=%d\n%s", args, exitCode, out)
	if err != nil || exitCode != 0 {
		t.Fatalf("omni %v failed (exit=%d): %s", args, exitCode, out)
	}
	return string(out)
}

// runOmniAllowFail runs an omni subcommand and returns output + exit code
// without failing the test. Used for teardown and negative assertions.
func runOmniAllowFail(t *testing.T, cfg testConfig, args ...string) (string, int) {
	t.Helper()
	cmd := append([]string{cfg.omniPath}, args...)
	exitCode, out, _ := cfg.exec.RunCommand(context.Background(), cmd)
	t.Logf("omni %v → exit=%d\n%s", args, exitCode, out)
	return string(out), exitCode
}

// logContains checks if the journalctl buffer contains the pattern.
func logContains(buf *syncBuffer, pattern string) bool {
	return bytes.Contains(buf.Bytes(), []byte(pattern))
}

// teardownAgent deletes an agent by name via CLI. Safe to call in t.Cleanup.
func teardownAgent(t *testing.T, cfg testConfig, name string) {
	t.Helper()
	t.Logf("teardown: deleting agent %q", name)
	_, _ = runOmniAllowFail(t, cfg, "agent", "delete", name, "--workspace", cfg.workspace)
}

// resumeAgentBackground runs `omni agent resume <name>` in a background goroutine
// via StreamCommand so it does not block the test.
func resumeAgentBackground(t *testing.T, cfg testConfig, name string) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	var buf syncBuffer
	done := make(chan struct{})

	go func() {
		defer close(done)
		cmd := []string{cfg.omniPath, "agent", "resume", name, "--workspace", cfg.workspace}
		_ = cfg.exec.StreamCommand(ctx, &buf, cmd)
	}()

	stop = func() {
		cancel()
		<-done
		t.Logf("resume %s output:\n%s", name, buf.String())
	}
	t.Cleanup(stop)
	return stop
}
