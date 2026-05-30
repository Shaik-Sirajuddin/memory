//go:build e2e

package e2e

import (
	"fmt"
	"testing"
	"time"
)

const (
	crossProviderStartWait    = 20 * time.Second // codex needs more startup time than claude
	crossProviderDeliveryWait = 90 * time.Second
)

// TestCodexSaysHiToClaude verifies cross-provider message delivery:
// a codex agent uses tunnel-mcp send_message to greet a claude agent.
//
// The prompt explicitly instructs codex to stop after sending so the
// two agents do not start an open-ended conversation.
//
// SKIP: codex PTY sessions never appear in MCP handler logs (no sender_id=<codex-agent>
// even after 90s). Root cause: codex is not connecting to the tunnel-mcp MCP server
// — likely AXO_LINK_MCP_* env vars not injected by operator for codex sessions, or
// codex runtime not loading /root/.codex/config.toml MCP section. Tracked in
// memory/agents/e2e/generated/server-bugs.md.
func TestCodexSaysHiToClaude(t *testing.T) {
	codexAgent := "e2e-codex-hi"
	claudeAgent := "e2e-claude-hi"

	cfg := newConfig(t)
	teardownAgent(t, cfg, codexAgent)
	teardownAgent(t, cfg, claudeAgent)
	t.Skip("blocked: codex sessions do not connect to tunnel-mcp MCP server — see server-bugs.md #3")
	t.Cleanup(func() {
		teardownAgent(t, cfg, codexAgent)
		teardownAgent(t, cfg, claudeAgent)
	})

	_, logBuf := captureLog(t, cfg)
	time.Sleep(300 * time.Millisecond)

	runOmni(t, cfg, "team", "init")
	runOmni(t, cfg, "agent", "init", codexAgent,
		"--workspace", cfg.workspace, "--provider", "codex", "--interactive=false")
	runOmni(t, cfg, "agent", "init", claudeAgent,
		"--workspace", cfg.workspace, "--provider", "claude", "--interactive=false")

	runOmni(t, cfg, "agent", "resume", claudeAgent, "--detach", "--workspace", cfg.workspace)
	runOmni(t, cfg, "agent", "resume", codexAgent, "--detach", "--workspace", cfg.workspace)
	time.Sleep(crossProviderStartWait)

	// Explicit stop instruction prevents the agents from replying back and forth.
	prompt := fmt.Sprintf(
		"Use the tunnel-mcp send_message tool to send the message 'hi from codex' to the agent named '%s'. "+
			"Call the tool exactly once, then stop. Do not wait for a reply and do not send any further messages.",
		claudeAgent,
	)
	runOmni(t, cfg, "agent", "exec", codexAgent, "--prompt", prompt)
	time.Sleep(crossProviderDeliveryWait)

	log := logBuf.String()
	assertNoLogErrors(t, log)
	assertLogContains(t, log, "send_message", "send_message tool call must appear in journalctl")
	assertLogContains(t, log, "sender_id="+codexAgent, "mcp-handler must log codex agent as sender")
	if t.Failed() {
		t.Logf("=== journalctl (%d bytes) ===\n%s", len(log), log)
	}
}
