//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

const (
	execTestAgent  = "e2e-exec-test"
	codexTestAgent = "e2e-codex-test"
	execStartWait  = 10 * time.Second
	execPromptWait = 30 * time.Second
)

// TestAgentExecNoSession verifies that resuming an agent with --detach then
// calling exec delivers the prompt without a TTY.
func TestAgentExecNoSession(t *testing.T) {
	cfg := newConfig(t)
	t.Cleanup(func() { teardownAgent(t, cfg, execTestAgent) })

	_, logBuf := captureLog(t, cfg)
	time.Sleep(300 * time.Millisecond)

	runOmni(t, cfg, "team", "init")
	runOmni(t, cfg, "agent", "init", execTestAgent,
		"--workspace", cfg.workspace, "--provider", "claude", "--interactive=false")

	// --detach: starts session in ptydaemon, returns immediately, no TTY needed
	t.Logf("resuming %s in detached mode", execTestAgent)
	runOmni(t, cfg, "agent", "resume", execTestAgent, "--detach", "--workspace", cfg.workspace)
	time.Sleep(execStartWait)

	out := runOmni(t, cfg, "agent", "exec", execTestAgent,
		"--prompt", "reply with the single word: pong")
	t.Logf("exec output: %s", out)

	time.Sleep(execPromptWait)

	log := logBuf.String()
	t.Logf("=== journalctl (%d bytes) ===\n%s", len(log), log)

	assertNoLogErrors(t, log)
	if !strings.Contains(out, "prompt sent") && !strings.Contains(log, "ExecInSession") {
		t.Errorf("expected prompt delivery confirmation, got output=%q", out)
	}
}

// TestAgentResumeDetached verifies that `omni agent resume --detach` exits 0
// without attaching to a terminal, and a subsequent exec delivers a prompt.
func TestAgentResumeDetached(t *testing.T) {
	cfg := newConfig(t)
	name := execTestAgent + "-detach"
	t.Cleanup(func() { teardownAgent(t, cfg, name) })

	_, logBuf := captureLog(t, cfg)
	time.Sleep(300 * time.Millisecond)

	runOmni(t, cfg, "team", "init")
	runOmni(t, cfg, "agent", "init", name,
		"--workspace", cfg.workspace, "--provider", "claude", "--interactive=false")

	t.Logf("resuming %s with --detach", name)
	runOmni(t, cfg, "agent", "resume", name, "--detach", "--workspace", cfg.workspace)
	time.Sleep(execStartWait)

	out := runOmni(t, cfg, "agent", "exec", name,
		"--prompt", "reply with the single word: pong")
	t.Logf("exec output: %s", out)

	time.Sleep(execPromptWait)

	log := logBuf.String()
	assertNoLogErrors(t, log)
	if !strings.Contains(out, "prompt sent") && !strings.Contains(log, "ExecInSession") {
		t.Errorf("expected prompt delivery confirmation, got output=%q", out)
	}
}

// TestCodexAgentDelivery verifies a codex-provider agent receives a tunnel-mcp
// send_message from a claude agent.
//
// SKIP until codex auth is confirmed available in the e2e container.
func TestCodexAgentDelivery(t *testing.T) {
	t.Skip("pending: codex auth availability in container not confirmed")

	cfg := newConfig(t)
	sender := "e2e-codex-sender"
	receiver := codexTestAgent

	t.Cleanup(func() {
		teardownAgent(t, cfg, sender)
		teardownAgent(t, cfg, receiver)
	})

	_, logBuf := captureLog(t, cfg)
	time.Sleep(300 * time.Millisecond)

	runOmni(t, cfg, "team", "init")
	runOmni(t, cfg, "agent", "init", sender,
		"--workspace", cfg.workspace, "--provider", "claude", "--interactive=false")
	runOmni(t, cfg, "agent", "init", receiver,
		"--workspace", cfg.workspace, "--provider", "codex", "--interactive=false")

	runOmni(t, cfg, "agent", "resume", receiver, "--detach", "--workspace", cfg.workspace)
	runOmni(t, cfg, "agent", "resume", sender, "--detach", "--workspace", cfg.workspace)
	time.Sleep(execStartWait)

	prompt := "Use the tunnel-mcp send_message tool to send 'hi from claude' to the agent named '" + receiver + "'. Call the tool now."
	runOmni(t, cfg, "agent", "exec", sender, "--prompt", prompt)
	time.Sleep(execPromptWait)

	log := logBuf.String()
	t.Logf("=== journalctl (%d bytes) ===\n%s", len(log), log)

	assertNoLogErrors(t, log)
	assertLogContains(t, log, "send_message", "send_message must appear in journalctl")
}
