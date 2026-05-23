package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Shaik-Sirajuddin/memory/mcp/mcp/runner"
	hookoperator "github.com/Shaik-Sirajuddin/memory/svc/hook-operator"
	"github.com/Shaik-Sirajuddin/memory/svc/ptydaemon"
)

// ServiceConfig holds the enable flag shared by all service entries.
type ServiceConfig struct {
	Enabled bool
}

// PTYDaemonConfig configures the ptydaemon service.
type PTYDaemonConfig struct {
	ServiceConfig
	SocketPath string
	DBPath     string
}

// HookOperatorConfig configures the hook-operator service.
type HookOperatorConfig struct {
	ServiceConfig
	SocketPath string
	BinaryPath string // omni binary used in hook entries; resolved by service if empty
}

// AxolinkMCPConfig configures the tunnel MCP service.
type AxolinkMCPConfig struct {
	ServiceConfig
}

// ServiceMux runs ptydaemon, hook-operator, and axolink-mcp as in-process
// goroutines under a shared context. Stopping the context is the only
// shutdown signal needed.
type ServiceMux struct {
	PTYDaemon    PTYDaemonConfig
	HookOperator HookOperatorConfig
	AxolinkMCP   AxolinkMCPConfig
}

// Run starts all enabled services and blocks until all have exited. The first
// service error is returned; context cancellation is not treated as an error.
func (m *ServiceMux) Run(ctx context.Context, log *slog.Logger) error {
	type result struct{ err error }
	ch := make(chan result, 3)
	var wg sync.WaitGroup

	if m.PTYDaemon.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Info("ptydaemon starting", "socket", m.PTYDaemon.SocketPath)
			err := ptydaemon.Run(ctx, m.PTYDaemon.SocketPath, m.PTYDaemon.DBPath)
			if err != nil {
				ch <- result{fmt.Errorf("ptydaemon: %w", err)}
			}
		}()
	}

	if m.HookOperator.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := hookoperator.Run(ctx, hookoperator.RunOptions{
				UnixPath:   m.HookOperator.SocketPath,
				BinaryPath: m.HookOperator.BinaryPath,
			})
			if err != nil {
				ch <- result{fmt.Errorf("hook-operator: %w", err)}
			}
		}()
	}

	if m.AxolinkMCP.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := runner.Run(ctx, runner.DefaultConfig()); err != nil {
				ch <- result{fmt.Errorf("axolink-mcp: %w", err)}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	for r := range ch {
		if r.err != nil {
			return r.err
		}
	}
	return nil
}
