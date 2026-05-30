//go:build e2e

package e2e

import (
	"fmt"
	"testing"
	"time"
)

const (
	crossProviderStartWait = 20 * time.Second // codex may need longer than claude to initialise
)

// TestCodexSaysHiToClaude verifies cross-provider message delivery:
// a codex agent uses tunnel-mcp send_message to greet a claude agent.
//
// The prompt explicitly instructs codex to stop after sending so the
// two agents do not start an open-ended conversation.
func TestCodexSaysHiToClaude(t *testing.T) {
	codexAgent := "e2e-codex-hi"
	claudeAgent := "e2e-claude-hi"

	cfg := newConfig(t)
	teardownAgent(t, cfg, codexAgent)
	teardownAgent(t, cfg, claudeAgent)
	// Codex session fails with "not authenticated" in omni-e2e container.
	// Pending auth fix from codex-connector team (OPENAI_API_KEY or shared auth volume).
	t.Skip("blocked: codex auth expired in e2e container — awaiting OPENAI_API_KEY or shared auth volume fix")
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

	if !logBuf.WaitFor("tool=send_message", sendMessageWait) {
		t.Errorf("codex send_message not observed within %s", sendMessageWait)
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
