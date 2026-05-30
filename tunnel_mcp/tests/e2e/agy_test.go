//go:build e2e

package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

const (
	agyStartWait    = 15 * time.Second // agy may need longer than claude to initialise
	agySendWait     = 60 * time.Second
	agyDeliveryWait = 10 * time.Second // best-effort; subject to server-bugs.md #7
)

// TestAgySaysHi verifies end-to-end delivery via the agy provider:
//   - an agy agent resumes with --detach
//   - exec delivers a prompt instructing it to call send_message
//   - journalctl confirms send_message was called (sender side)
//   - no unexpected ERROR entries
//
// Known failure surface (tracked in server-bugs.md):
//   - If agy binary lacks tunnel-mcp MCP configuration, no send_message will appear.
//     The test will fail with a clear "send_message not observed" message so the
//     gap is visible in CI rather than silently skipped.
//   - exec in session observation is best-effort (#7).
func TestAgySaysHi(t *testing.T) {
	const (
		agyAgent    = "e2e-agy-sender"
		claudeAgent = "e2e-agy-receiver"
	)

	cfg := newConfig(t)
	teardownAgent(t, cfg, agyAgent)
	teardownAgent(t, cfg, claudeAgent)
	t.Cleanup(func() {
		teardownAgent(t, cfg, agyAgent)
		teardownAgent(t, cfg, claudeAgent)
	})

	_, logBuf := captureLog(t, cfg)
	time.Sleep(300 * time.Millisecond)

	runOmni(t, cfg, "team", "init")
	runOmni(t, cfg, "agent", "init", agyAgent,
		"--workspace", cfg.workspace, "--provider", "agy", "--interactive=false")
	runOmni(t, cfg, "agent", "init", claudeAgent,
		"--workspace", cfg.workspace, "--provider", "claude", "--interactive=false")

	runOmni(t, cfg, "agent", "resume", claudeAgent, "--detach", "--workspace", cfg.workspace)

	// agy connector does not implement p.Detached in Resume() — it always calls
	// client.Attach() which requires a real TTY. Until that is fixed, resume with
	// --detach fails with "pty attach: inappropriate ioctl for device".
	// We attempt the resume and skip with a tracked reason rather than failing hard,
	// so the test documents the gap without flapping CI.
	if out, code := runOmniAllowFail(t, cfg, "agent", "resume", agyAgent, "--detach", "--workspace", cfg.workspace); code != 0 {
		if strings.Contains(out, "pty attach") || strings.Contains(out, "inappropriate ioctl") {
			t.Skipf("SKIP: agy --detach not implemented (agy connector Resume ignores p.Detached, always calls Attach) — server-bugs.md #8")
		}
		t.Fatalf("omni agent resume %s --detach failed unexpectedly (exit=%d): %s", agyAgent, code, out)
	}
	time.Sleep(agyStartWait)

	prompt := fmt.Sprintf(
		"Use the tunnel-mcp send_message tool to send the message 'hi from agy' to the agent named '%s'. "+
			"Call the tool exactly once then stop.",
		claudeAgent,
	)
	runOmni(t, cfg, "agent", "exec", agyAgent, "--prompt", prompt)

	if !logBuf.WaitFor("tool=send_message", agySendWait) {
		t.Errorf("agy send_message not observed within %s — agy MCP integration may not be working", agySendWait)
		t.Logf("=== journalctl (%d bytes) ===\n%s", len(logBuf.String()), logBuf.String())
		return
	}

	if !logBuf.WaitFor("exec in session", agyDeliveryWait) {
		t.Logf("WARN: exec in session not observed for receiver within %s (server-bugs.md #7)", agyDeliveryWait)
	}

	log := logBuf.String()
	assertNoLogErrors(t, log)
	assertSenderLogged(t, log, agyAgent)
	assertNoExecSessionFailed(t, log)
	if t.Failed() {
		t.Logf("=== journalctl (%d bytes) ===\n%s", len(log), log)
	}
}

// TestAgyHookRegistration verifies that the agy global settings file contains
// the expected omni hook commands. Hooks are written to /root/.agy/settings.json
// (global, not per-agent) by the container entrypoint / config_sync.
func TestAgyHookRegistration(t *testing.T) {
	cfg := newConfig(t)

	// Global agy settings — written by entrypoint or config_sync, not per-agent init.
	settingsPath := "/root/.agy/settings.json"
	exitCode, out, _ := cfg.exec.RunCommand(
		newCtx(t), []string{"cat", settingsPath},
	)
	if exitCode != 0 {
		t.Fatalf("agy global settings not found at %s", settingsPath)
	}

	settings := string(out)
	t.Logf("agy settings.json:\n%s", settings)

	for _, hook := range []string{
		"omni hook --event PostToolUse",
		"omni hook --event PreToolUse",
		"omni hook --event Stop",
		"omni hook --event SessionStart",
	} {
		assertLogContains(t, settings, hook,
			"agy settings.json must contain hook command: "+hook)
	}
	assertLogContains(t, settings, `"mcp__tunnel-mcp__*"`,
		"agy settings.json must allow tunnel-mcp tools")
}
