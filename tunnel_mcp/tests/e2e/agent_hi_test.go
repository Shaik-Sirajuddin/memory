//go:build e2e

package e2e

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	hiAgent1 = "e2e-hi-agent-1"
	hiAgent2 = "e2e-hi-agent-2"

	// how long to wait for agents to attach to the pty daemon after resume
	agentStartWait = 8 * time.Second
	// how long to wait for agent-1 to call send_message and agent-2 to receive
	deliveryWait = 40 * time.Second
)

// TestAgentsSayHi launches two claude agents, instructs agent-1 to send a
// greeting to agent-2 via tunnel-mcp send_message, then asserts:
//   - journalctl contains no error-level lines
//   - send_message tool was called (delivery path exercised)
//
// Teardown: both agents are deleted via CLI — no docker restart needed.
func TestAgentsSayHi(t *testing.T) {
	t.Skip("pending: omni agent resume --detach CLI flag not yet wired; see memory/agents/e2e/generated/test-run-report-2026-05-27.md")
	cfg := newConfig(t)

	// ── teardown registered before anything is created ──────────────────────
	t.Cleanup(func() {
		teardownAgent(t, cfg, hiAgent1)
		teardownAgent(t, cfg, hiAgent2)
	})

	// ── start journalctl first — primary observation pipe ───────────────────
	_, logBuf := captureLog(t, cfg)
	time.Sleep(500 * time.Millisecond) // let journalctl attach before agents start

	// ── ensure workspace ────────────────────────────────────────────────────
	runOmni(t, cfg, "team", "init")

	// ── create both agents (non-interactive — no PTY attach on create) ───────
	t.Logf("creating agents %s and %s", hiAgent1, hiAgent2)
	runOmni(t, cfg, "agent", "init", hiAgent1,
		"--workspace", cfg.workspace, "--provider", "claude", "--interactive=false")
	runOmni(t, cfg, "agent", "init", hiAgent2,
		"--workspace", cfg.workspace, "--provider", "claude", "--interactive=false")

	// ── resume both in background ────────────────────────────────────────────
	// Resume runs in background via StreamCommand because it blocks on PTY lifecycle.
	// The session IS started in the PTY daemon even when terminal attachment fails
	// (no TTY in docker exec without -t). We wait for the daemon to register it.
	t.Logf("resuming %s and %s in background", hiAgent2, hiAgent1)
	resumeAgentBackground(t, cfg, hiAgent2)
	resumeAgentBackground(t, cfg, hiAgent1)

	t.Logf("waiting %s for agent sessions to register...", agentStartWait)
	time.Sleep(agentStartWait)

	// ── send prompt to agent-1 ───────────────────────────────────────────────
	prompt := fmt.Sprintf(
		"Use the tunnel-mcp MCP server's send_message tool to send the message "+
			"'hi from %s' to the agent named '%s'. "+
			"Call the tool now and confirm it was sent.",
		hiAgent1, hiAgent2,
	)
	t.Logf("sending prompt to %s", hiAgent1)
	// exec without --resume: session was already started by resume background
	runOmni(t, cfg, "agent", "exec", hiAgent1, "--prompt", prompt)

	// ── wait for delivery propagation ────────────────────────────────────────
	t.Logf("waiting %s for delivery...", deliveryWait)
	time.Sleep(deliveryWait)

	// ── capture final log snapshot ───────────────────────────────────────────
	log := logBuf.String()
	t.Logf("=== journalctl snapshot (%d bytes) ===\n%s", len(log), log)

	// ── assertions ───────────────────────────────────────────────────────────
	assertNoLogErrors(t, log)
	assertLogContains(t, log, "send_message", "send_message tool call must appear in journalctl")
}

// ─── assertion helpers ──────────────────────────────────────────────────────

var (
	// matches structured log lines with level=ERROR emitted by omni-server
	reLogError = regexp.MustCompile(`(?m)level=ERROR`)
	// matches unstructured panic / fatal lines
	rePanic = regexp.MustCompile(`(?im)panic:|fatal error:`)
)

// assertNoLogErrors fails the test if journalctl contains ERROR-level log lines,
// printing the full log so the failure is self-contained.
func assertNoLogErrors(t *testing.T, log string) {
	t.Helper()
	errorLines := extractMatches(log, reLogError)
	if len(errorLines) > 0 {
		t.Errorf("journalctl contains %d ERROR line(s):\n%s\n\n--- full log ---\n%s",
			len(errorLines), strings.Join(errorLines, "\n"), log)
	}

	panicLines := extractMatches(log, rePanic)
	assert.Empty(t, panicLines,
		"journalctl must not contain panic/fatal lines\nfull log:\n%s", log)
}

// assertLogContains fails with the full log if the substring is absent.
func assertLogContains(t *testing.T, log, substr, msg string) {
	t.Helper()
	require.True(t, bytes.Contains([]byte(log), []byte(substr)),
		"%s\nsubstring %q not found\n--- full log ---\n%s", msg, substr, log)
}

// extractMatches returns all lines from s that match re.
func extractMatches(s string, re *regexp.Regexp) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if re.MatchString(line) {
			out = append(out, line)
		}
	}
	return out
}
