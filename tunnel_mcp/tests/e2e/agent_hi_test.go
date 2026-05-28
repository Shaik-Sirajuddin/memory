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

	// how long to wait for detached sessions to be active in ptydaemon
	agentStartWait = 8 * time.Second
	// how long to wait for agent-1 to call send_message and agent-2 to receive
	deliveryWait = 40 * time.Second
)

// TestAgentsSayHi launches two claude agents in detached mode, instructs
// agent-1 to send a greeting to agent-2 via tunnel-mcp send_message, then asserts:
//   - journalctl contains no error-level lines
//   - send_message tool was called (delivery path exercised)
//
// Teardown: both agents deleted via CLI.
func TestAgentsSayHi(t *testing.T) {
	cfg := newConfig(t)

	t.Cleanup(func() {
		teardownAgent(t, cfg, hiAgent1)
		teardownAgent(t, cfg, hiAgent2)
	})

	_, logBuf := captureLog(t, cfg)
	time.Sleep(500 * time.Millisecond)

	runOmni(t, cfg, "team", "init")

	t.Logf("creating agents %s and %s", hiAgent1, hiAgent2)
	runOmni(t, cfg, "agent", "init", hiAgent1,
		"--workspace", cfg.workspace, "--provider", "claude", "--interactive=false")
	runOmni(t, cfg, "agent", "init", hiAgent2,
		"--workspace", cfg.workspace, "--provider", "claude", "--interactive=false")

	// --detach starts the PTY daemon session and returns immediately (no TTY needed)
	t.Logf("resuming agents in detached mode")
	runOmni(t, cfg, "agent", "resume", hiAgent2, "--detach", "--workspace", cfg.workspace)
	runOmni(t, cfg, "agent", "resume", hiAgent1, "--detach", "--workspace", cfg.workspace)

	t.Logf("waiting %s for sessions to be active...", agentStartWait)
	time.Sleep(agentStartWait)

	prompt := fmt.Sprintf(
		"Use the tunnel-mcp MCP server's send_message tool to send the message "+
			"'hi from %s' to the agent named '%s'. "+
			"Call the tool now and confirm it was sent.",
		hiAgent1, hiAgent2,
	)
	t.Logf("sending prompt to %s", hiAgent1)
	runOmni(t, cfg, "agent", "exec", hiAgent1, "--prompt", prompt)

	t.Logf("waiting %s for delivery...", deliveryWait)
	time.Sleep(deliveryWait)

	log := logBuf.String()
	t.Logf("=== journalctl snapshot (%d bytes) ===\n%s", len(log), log)

	assertNoLogErrors(t, log)
	assertLogContains(t, log, "send_message", "send_message tool call must appear in journalctl")
}

// TestMessageRefsIntegrity verifies that when agent-1 sends a message to
// agent-2, the refs forwarded with the prompt contain:
//   - author_agent_name = agent name string (not a UUID)
//   - prompt body = actual message content (not boilerplate)
//
// This guards against the bugs in tunnel_mcp/server/reply.go:
//   - replyRefs() setting author_agent_name to agent ID instead of name
//   - replyPrompt() dropping actual content and emitting boilerplate
func TestMessageRefsIntegrity(t *testing.T) {
	cfg := newConfig(t)

	sender := "e2e-refs-sender"
	receiver := "e2e-refs-receiver"
	t.Cleanup(func() {
		teardownAgent(t, cfg, sender)
		teardownAgent(t, cfg, receiver)
	})

	_, logBuf := captureLog(t, cfg)
	time.Sleep(500 * time.Millisecond)

	runOmni(t, cfg, "team", "init")
	runOmni(t, cfg, "agent", "init", sender,
		"--workspace", cfg.workspace, "--provider", "claude", "--interactive=false")
	runOmni(t, cfg, "agent", "init", receiver,
		"--workspace", cfg.workspace, "--provider", "claude", "--interactive=false")

	runOmni(t, cfg, "agent", "resume", receiver, "--detach", "--workspace", cfg.workspace)
	runOmni(t, cfg, "agent", "resume", sender, "--detach", "--workspace", cfg.workspace)
	time.Sleep(agentStartWait)

	const sentText = "integrity-check-payload-xyz"
	prompt := fmt.Sprintf(
		"Use the tunnel-mcp send_message tool to send the message '%s' to the agent named '%s'. Call the tool now.",
		sentText, receiver,
	)
	runOmni(t, cfg, "agent", "exec", sender, "--prompt", prompt)
	time.Sleep(deliveryWait)

	log := logBuf.String()
	t.Logf("=== journalctl snapshot (%d bytes) ===\n%s", len(log), log)

	assertNoLogErrors(t, log)

	// payload must appear in log — confirms actual content was forwarded, not boilerplate
	assert.True(t, strings.Contains(log, sentText),
		"forwarded prompt must contain sent text %q — got boilerplate instead\nlog:\n%s", sentText, log)

	// author_agent_name in refs must be a name string, not a UUID
	// UUIDs match the pattern xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	reUUID := regexp.MustCompile(`author_agent_name":"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"`)
	assert.False(t, reUUID.MatchString(log),
		"author_agent_name must be agent name string, not UUID\nlog:\n%s", log)
}

// ─── assertion helpers ──────────────────────────────────────────────────────

var (
	reLogError = regexp.MustCompile(`(?m)level=ERROR`)
	rePanic    = regexp.MustCompile(`(?im)panic:|fatal error:`)
)

func assertNoLogErrors(t *testing.T, log string) {
	t.Helper()
	errorLines := extractMatches(log, reLogError)
	if len(errorLines) > 0 {
		t.Errorf("journalctl contains %d ERROR line(s):\n%s\n\n--- full log ---\n%s",
			len(errorLines), strings.Join(errorLines, "\n"), log)
	}
	assert.Empty(t, extractMatches(log, rePanic),
		"journalctl must not contain panic/fatal lines\nfull log:\n%s", log)
}

func assertLogContains(t *testing.T, log, substr, msg string) {
	t.Helper()
	require.True(t, bytes.Contains([]byte(log), []byte(substr)),
		"%s\nsubstring %q not found\n--- full log ---\n%s", msg, substr, log)
}

func extractMatches(s string, re *regexp.Regexp) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if re.MatchString(line) {
			out = append(out, line)
		}
	}
	return out
}
