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
	execPromptWait = 45 * time.Second // max wait for exec in session to appear
)

// TestAgentExecNoSession verifies that resuming an agent with --detach then
// calling exec delivers the prompt without a TTY.
func TestAgentExecNoSession(t *testing.T) {
	cfg := newConfig(t)
	teardownAgent(t, cfg, execTestAgent)
	t.Cleanup(func() { teardownAgent(t, cfg, execTestAgent) })

	_, logBuf := captureLog(t, cfg)
	time.Sleep(300 * time.Millisecond)

	runOmni(t, cfg, "team", "init")
	runOmni(t, cfg, "agent", "init", execTestAgent,
		"--workspace", cfg.workspace, "--provider", "claude", "--interactive=false")

	// --detach: starts session in ptydaemon, returns immediately, no TTY needed
	runOmni(t, cfg, "agent", "resume", execTestAgent, "--detach", "--workspace", cfg.workspace)
	time.Sleep(execStartWait)

	out := runOmni(t, cfg, "agent", "exec", execTestAgent,
		"--prompt", "reply with the single word: pong")

	// Stream-wait: accept either CLI confirmation or server-side ExecInSession log.
	delivered := strings.Contains(out, "prompt sent") ||
		logBuf.WaitFor("exec in session", execPromptWait)
	if !delivered {
		t.Errorf("prompt not delivered within %s; exec output=%q", execPromptWait, out)
	}

	log := logBuf.String()
	assertNoLogErrors(t, log)
	assertNoExecSessionFailed(t, log)
	if t.Failed() {
		t.Logf("=== journalctl (%d bytes) ===\n%s", len(log), log)
	}
}

// TestAgentResumeDetached verifies that `omni agent resume --detach` exits 0
// without attaching to a terminal, and a subsequent exec delivers a prompt.
func TestAgentResumeDetached(t *testing.T) {
	cfg := newConfig(t)
	name := execTestAgent + "-detach"
	teardownAgent(t, cfg, name)
	t.Cleanup(func() { teardownAgent(t, cfg, name) })

	_, logBuf := captureLog(t, cfg)
	time.Sleep(300 * time.Millisecond)

	runOmni(t, cfg, "team", "init")
	runOmni(t, cfg, "agent", "init", name,
		"--workspace", cfg.workspace, "--provider", "claude", "--interactive=false")

	runOmni(t, cfg, "agent", "resume", name, "--detach", "--workspace", cfg.workspace)
	time.Sleep(execStartWait)

	out := runOmni(t, cfg, "agent", "exec", name,
		"--prompt", "reply with the single word: pong")

	delivered := strings.Contains(out, "prompt sent") ||
		logBuf.WaitFor("exec in session", execPromptWait)
	if !delivered {
		t.Errorf("prompt not delivered within %s; exec output=%q", execPromptWait, out)
	}

	log := logBuf.String()
	assertNoLogErrors(t, log)
	assertNoExecSessionFailed(t, log)
	if t.Failed() {
		t.Logf("=== journalctl (%d bytes) ===\n%s", len(log), log)
	}
}

// TestCodexAgentDelivery verifies a codex-provider agent receives a tunnel-mcp
// send_message from a claude agent.
//
// SKIP: same root cause as TestCodexSaysHiToClaude — codex sessions do not
// connect to tunnel-mcp MCP server so delivery cannot be observed via journalctl.
// Claude (sender) calls send_message fine but the 30s observation window is also
// too short for the auto-resume path. Tracked in server-bugs.md #3.
func TestCodexAgentDelivery(t *testing.T) {
	cfg := newConfig(t)
	sender := "e2e-codex-sender"
	receiver := codexTestAgent

	teardownAgent(t, cfg, sender)
	teardownAgent(t, cfg, receiver)
	t.Skip("blocked: codex sessions do not connect to tunnel-mcp MCP server — see server-bugs.md #3")
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
	assertNoLogErrors(t, log)
	assertLogContains(t, log, "send_message", "send_message must appear in journalctl")
	if t.Failed() {
		t.Logf("=== journalctl (%d bytes) ===\n%s", len(log), log)
	}
}
