//go:build e2e

package e2e

import (
	"fmt"
	"testing"
	"time"
)

const (
	crossProviderStartWait    = 20 * time.Second // codex may need longer than claude to initialise
	crossProviderDeliveryWait = 120 * time.Second // gpt-5.4-mini may need longer to call tools
)

// TestCodexSaysHiToClaude verifies cross-provider message delivery:
// a codex agent uses tunnel-mcp send_message to greet a claude agent.
//
// MCP identity (env var injection) is confirmed working — codex connects with
// correct sender_id/sender_type. But codex does not call send_message when prompted.
// Suspected: tool discovery mismatch (config key "tunnel_mcp" vs tool namespace
// gpt-5.4-mini sees). Tracking with codex-connector team.
func TestCodexSaysHiToClaude(t *testing.T) {
	codexAgent := "e2e-codex-hi"
	claudeAgent := "e2e-claude-hi"

	cfg := newConfig(t)
	teardownAgent(t, cfg, codexAgent)
	teardownAgent(t, cfg, claudeAgent)
	// MCP identity works (sender_id/type correctly sent). But codex only sees `health`
	// in its MCP tool list — `send_message` is not advertised by the tunnel-mcp
	// streamable HTTP server to codex clients. Claude sees it fine. Server-side
	// tools/list response investigation pending with codex-connector team.
	t.Skip("blocked: send_message not in codex MCP tool list — tunnel-mcp server tools/list investigation pending")
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
	// Prompt avoids mentioning the server name to prevent tool lookup mismatch.
	prompt := fmt.Sprintf(
		"You have access to MCP tools. Call the send_message tool with target agent name '%s' "+
			"and message 'hi from codex'. Call the tool once then stop.",
		claudeAgent,
	)
	runOmni(t, cfg, "agent", "exec", codexAgent, "--prompt", prompt)

	if !logBuf.WaitFor("tool=send_message", crossProviderDeliveryWait) {
		t.Errorf("codex send_message not observed within %s", crossProviderDeliveryWait)
		t.Logf("=== journalctl (%d bytes) ===\n%s", len(logBuf.String()), logBuf.String())
		return
	}
	if !logBuf.WaitFor("exec in session", execInSessionWait) {
		t.Logf("WARN: exec in session not observed for claude receiver within %s (server-bugs.md #7)", execInSessionWait)
	}

	log := logBuf.String()
	assertNoLogErrors(t, log)
	assertSenderLogged(t, log, codexAgent)
	assertNoExecSessionFailed(t, log)
	if t.Failed() {
		t.Logf("=== journalctl (%d bytes) ===\n%s", len(log), log)
	}
}
