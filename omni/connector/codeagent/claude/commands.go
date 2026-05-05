package claude

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
	sandbox "github.com/Shaik-Sirajuddin/memory/sandbox/provider"
	"github.com/creack/pty"
)

const (
	Claude codeagent.Provider = "claude"
)

// PTY bracketed-paste and submit constants for ExecInSession.
const (
	submitKey  = "\x1b[13;2u" // CSI-u Shift+Enter
	ctrlU      = "\x15"
	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"
)

// interactiveStdin/Stdout/Stderr are the I/O streams used by Resume.
// They are package-level vars so tests can substitute non-TTY writers.
var (
	interactiveStdin  io.Reader = nil // nil = open /dev/tty at runtime
	interactiveStdout io.Writer = nil
	interactiveStderr io.Writer = nil
)

// ============================================================
// Session lifecycle
// ============================================================

// Create verifies the claude binary is reachable and the user is authenticated,
// then stores the resolved session parameters for subsequent Exec/Stream calls.
func (a *claudeAgent) Create(p codeagent.CreateSessionParams) (*codeagent.CreateSessionResult, error) {
	a.mu.Lock()
	if p.WorkDir != "" {
		a.workDir = p.WorkDir
	}
	if p.Model != "" {
		a.model = p.Model
	}
	if p.PermissionMode != "" {
		a.permMode = p.PermissionMode
	}
	if p.SystemPrompt != "" {
		a.systemPrompt = p.SystemPrompt
	}
	id := p.ID
	if id == "" {
		id = generateID()
	}
	// If a pre-existing Claude session ID is provided, attach to it directly
	// instead of seeding a new one.
	if p.SessionID != "" {
		a.sessionID = p.SessionID
	} else {
		a.sessionID = id
	}
	if p.RunTime != nil {
		a.sbxRuntime = *p.RunTime
	}
	workDir := a.workDir
	sessionID := a.sessionID
	a.mu.Unlock()

	// Verify binary.
	out, err := captureOutput(workDir, "claude", "--version")
	if err != nil {
		return nil, fmt.Errorf("claude: create: binary unreachable: %w", err)
	}
	logger.Debug("Create: claude binary ok", "version", trimSpace(out))

	// Verify authentication.
	authCmd := exec.Command("claude", "auth", "status")
	authCmd.Dir = workDir
	if err := authCmd.Run(); err != nil {
		logger.Warn("Create: user not authenticated", "err", err)
		return nil, fmt.Errorf("claude: create: not authenticated — run 'claude auth login' first")
	}

	// When attaching to an existing session, skip seeding — the conversation
	// already exists in Claude's store and a seed call would corrupt it.
	if p.SessionID != "" {
		logger.Info("Create: attached to existing session", "id", id, "sessionID", sessionID, "workDir", workDir)
		return &codeagent.CreateSessionResult{ID: id, Name: p.Name}, nil
	}

	// Seed the session into Claude's session store by running a minimal
	// print-mode call with --session-id. Without this, `claude -r <id>`
	// fails with "No conversation found" because Claude only persists a
	// session after at least one print-mode exchange.
	a.mu.RLock()
	model := a.model
	rt := a.sbxRuntime
	a.mu.RUnlock()

	seedArgs := []string{
		"-p", "hello",
		"--session-id", sessionID,
		"--model", model,
		"--output-format", "json",
		"--max-turns", "1",
	}
	seedOut, seedErr := execOutput(workDir, rt, "claude", seedArgs...)
	if seedErr != nil {
		return nil, fmt.Errorf("claude: create: seed session: %w", seedErr)
	}
	logger.Debug("Create: session seeded", "id", id, "output", trimSpace(seedOut))

	logger.Info("Create: session ready", "id", id, "workDir", workDir)
	return &codeagent.CreateSessionResult{ID: id, Name: p.Name}, nil
}

// ptyStoper is satisfied by PTY daemon clients that can notify the daemon
// when a session terminates. It is optional — Pipe-only clients are fine.
type ptyStoper interface {
	Stop(agentID, sessionID string) error
}

// Resume launches an interactive claude session via `claude -r <id>`.
func (a *claudeAgent) Resume(p codeagent.ResumeSessionParams) (*codeagent.ResumeSessionResult, error) {
	// Block if a PTY session is already active.
	a.mu.RLock()
	live := a.masterPTY != nil
	a.mu.RUnlock()
	if live {
		return nil, errors.New("claude: PTY session already active; stop it before resuming")
	}

	a.mu.Lock()
	workDir := a.workDir
	if p.RunTime != nil {
		a.sbxRuntime = *p.RunTime
	}
	rt := a.sbxRuntime
	client := a.ptyClient
	agentID := a.sessionID
	a.mu.Unlock()

	// Prefer the explicit Claude session ID when provided; fall back to p.ID.
	resumeID := p.ID
	if p.SessionID != "" {
		resumeID = p.SessionID
	}

	args := []string{"-r", resumeID}
	if p.ForkSession {
		args = append(args, "--fork-session")
	}

	if rt != nil {
		if err := rt.Command("claude", args); err != nil {
			return nil, fmt.Errorf("claude: resume: sandbox command: %w", err)
		}
		pid := runtimePID(rt)
		logger.Info("Resume: interactive sandbox session completed", "pid", pid, "sessionID", resumeID)
		return &codeagent.ResumeSessionResult{ProcessID: pid, SessionID: resumeID}, nil
	}

	cmd := localCommand(workDir, "claude", args...)

	// PTY mode: start under a pseudo-terminal so ExecInSession can write directly
	// to the master fd without going through the daemon's HTTP layer.
	if client != nil {
		master, err := pty.Start(cmd)
		if err != nil {
			return nil, fmt.Errorf("claude: resume: pty start: %w", err)
		}
		a.mu.Lock()
		a.masterPTY = master
		a.mu.Unlock()

		pid := strconv.Itoa(cmd.Process.Pid)
		logger.Info("Resume: PTY session started", "pid", pid, "sessionID", resumeID)

		go func() {
			_ = cmd.Wait()
			a.mu.Lock()
			a.masterPTY = nil
			a.mu.Unlock()
			if s, ok := client.(ptyStoper); ok {
				_ = s.Stop(agentID, resumeID)
			}
			logger.Debug("Resume: PTY session terminated", "sessionID", resumeID)
		}()

		return &codeagent.ResumeSessionResult{ProcessID: pid, SessionID: resumeID}, nil
	}

	// Blocking mode: open /dev/tty directly so the child gets a proper
	// controlling terminal for raw mode. Pipe-like fds cause EIO on setRawMode.
	if interactiveStdin != nil || interactiveStdout != nil || interactiveStderr != nil {
		cmd.Stdin = interactiveStdin
		cmd.Stdout = interactiveStdout
		cmd.Stderr = interactiveStderr
	} else {
		tty, ttyErr := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if ttyErr == nil {
			defer tty.Close()
			cmd.Stdin = tty
			cmd.Stdout = tty
			cmd.Stderr = tty
		} else {
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("claude: resume: start process: %w", err)
	}

	pid := fmt.Sprintf("%d", cmd.Process.Pid)
	logger.Info("Resume: interactive session started", "pid", pid, "sessionID", resumeID)

	// Block until the interactive session ends. This keeps the tty fd open
	// for the full duration and prevents the caller from racing with the child.
	_ = cmd.Wait()

	return &codeagent.ResumeSessionResult{ProcessID: pid, SessionID: resumeID}, nil
}

// List is not supported by the Claude CLI.
func (a *claudeAgent) List(_ codeagent.ListSessionsParams) (*codeagent.ListSessionsResult, error) {
	logger.Warn("List: claude CLI has no session list command")
	return &codeagent.ListSessionsResult{Sessions: nil}, nil
}

// Delete is not supported by the Claude CLI.
func (a *claudeAgent) Delete(_ codeagent.DeleteSessionParams) (*codeagent.DeleteSessionResult, error) {
	logger.Warn("Delete: claude CLI has no session delete command")
	return &codeagent.DeleteSessionResult{Deleted: false}, nil
}

func (a *claudeAgent) Stop() {
	logger.Info("Stop: no-op for non-interactive claude sessions")
}

// ============================================================
// ExecInSession
// ============================================================

// ExecInSession sends a prompt into an active interactive PTY session.
// It is fire-and-forget: the prompt is piped into the PTY stdin and the call
// returns immediately without waiting for a response.
func (a *claudeAgent) ExecInSession(p codeagent.ExecInSessionParams) (*codeagent.ExecInSessionResult, error) {
	a.mu.RLock()
	master := a.masterPTY
	client := a.ptyClient
	agentID := a.sessionID
	a.mu.RUnlock()

	// Bracketed-paste ensures multi-line prompts are sent atomically before
	// the submit key triggers execution.
	payload := []byte(ctrlU + pasteStart + p.Prompt + pasteEnd + submitKey)

	if master != nil {
		// Direct write to the PTY master — no HTTP round-trip needed.
		if _, err := master.Write(payload); err != nil {
			return nil, fmt.Errorf("claude: ExecInSession: write to PTY: %w", err)
		}
	} else if client != nil {
		if err := client.Pipe(agentID, p.SessionID, payload); err != nil {
			return nil, fmt.Errorf("claude: ExecInSession: session not live: %w", err)
		}
	} else {
		return nil, fmt.Errorf("claude: ExecInSession: no active PTY session")
	}

	logger.Debug("ExecInSession: prompt piped", "sessionID", p.SessionID, "promptLen", len(p.Prompt))
	return &codeagent.ExecInSessionResult{SessionID: p.SessionID}, nil
}

// ============================================================
// GetSessionConfig
// ============================================================

func (a *claudeAgent) GetSessionConfig(_ codeagent.GetSessionConfigParams) (*codeagent.GetSessionConfigResult, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return &codeagent.GetSessionConfigResult{
		Model:          codeagent.Model{Provider: Claude, Model: a.model},
		PermissionMode: a.permMode,
		WorkDir:        a.workDir,
		SystemPrompt:   a.systemPrompt,
	}, nil
}

// ============================================================
// Exec
// ============================================================

func (a *claudeAgent) Exec(p codeagent.ExecuteParams) (*codeagent.ExecuteResult, error) {
	a.mu.RLock()
	workDir := a.workDir
	model := a.model
	permMode := a.permMode
	systemPrompt := a.systemPrompt
	sessionID := a.sessionID
	rt := a.sbxRuntime
	a.mu.RUnlock()

	args := buildExecArgs(p.Prompt, model, permMode, systemPrompt, sessionID, p.OutputFormat, p.MaxTurns)
	logger.Debug("Exec", "workDir", workDir, "args", args)

	response, err := execOutput(workDir, rt, "claude", args...)
	if err != nil {
		return nil, err
	}
	logger.Debug("Exec completed", "responseLen", len(response))

	return &codeagent.ExecuteResult{
		PromptID:   p.PromptId,
		SessionID:  sessionID,
		Response:   response,
		StopReason: "stop",
	}, nil
}

// ============================================================
// Stream
// ============================================================

func (a *claudeAgent) Stream(p codeagent.StreamParams) (*codeagent.StreamResult, error) {
	a.mu.RLock()
	workDir := a.workDir
	model := a.model
	permMode := a.permMode
	systemPrompt := a.systemPrompt
	sessionID := a.sessionID
	rt := a.sbxRuntime
	a.mu.RUnlock()

	args := buildStreamArgs(p.Prompt, model, permMode, systemPrompt, sessionID, p.MaxTurns)
	logger.Debug("Stream", "workDir", workDir, "args", args)

	ch := make(chan codeagent.StreamEvent, 32)
	if rt != nil {
		proc, err := rt.Start("claude", args)
		if err != nil {
			return nil, fmt.Errorf("claude stream: sandbox start: %w", err)
		}
		go func() {
			defer close(ch)
			res, waitErr := proc.Wait()
			if waitErr != nil {
				msg := runtimeErrorf("claude stream", res, waitErr).Error()
				logger.Error("Stream: sandbox process exited with error", "err", msg)
				ch <- codeagent.StreamEvent{Type: "stop", Done: true, Content: msg}
				return
			}
			scanner := bufio.NewScanner(strings.NewReader(res.Stdout))
			for scanner.Scan() {
				line := scanner.Text()
				if line == "" {
					continue
				}
				ev := parseClaudeLine(line)
				ch <- ev
				if ev.Done {
					return
				}
			}
			ch <- codeagent.StreamEvent{Type: "stop", Done: true}
			logger.Debug("Stream completed via sandbox runtime")
		}()
		return &codeagent.StreamResult{Events: ch, SessionID: sessionID}, nil
	}

	cmd := localCommand(workDir, "claude", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("claude stream: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("claude stream: start process: %w", err)
	}
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			ev := parseClaudeLine(line)
			ch <- ev
			if ev.Done {
				return
			}
		}
		if err := cmd.Wait(); err != nil {
			msg := wrapExitError("claude stream", err).Error()
			logger.Error("Stream: process exited with error", "err", msg)
			ch <- codeagent.StreamEvent{Type: "stop", Done: true, Content: msg}
			return
		}
		ch <- codeagent.StreamEvent{Type: "stop", Done: true}
		logger.Debug("Stream completed")
	}()

	return &codeagent.StreamResult{Events: ch, SessionID: sessionID}, nil
}

// ============================================================
// Arg builders
// ============================================================

func buildExecArgs(prompt, model string, permMode codeagent.PermissionMode, systemPrompt, sessionID string, format codeagent.OutputFormat, maxTurns int) []string {
	args := []string{"-p", prompt, "--model", model}

	switch format {
	case codeagent.OutputFormatJSON:
		args = append(args, "--output-format", "json")
	case codeagent.OutputFormatStreamJSON:
		args = append(args, "--output-format", "stream-json")
	default:
		args = append(args, "--output-format", "text")
	}

	if maxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", maxTurns))
	}
	if permMode != "" && permMode != codeagent.PermissionDefault {
		args = append(args, "--permission-mode", string(permMode))
	}
	if systemPrompt != "" {
		args = append(args, "--system-prompt", systemPrompt)
	}
	if sessionID != "" {
		args = append(args, "--session-id", sessionID)
	}
	return args
}

func buildStreamArgs(prompt, model string, permMode codeagent.PermissionMode, systemPrompt, sessionID string, maxTurns int) []string {
	args := []string{"-p", prompt, "--model", model, "--output-format", "stream-json"}
	if maxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", maxTurns))
	}
	if permMode != "" && permMode != codeagent.PermissionDefault {
		args = append(args, "--permission-mode", string(permMode))
	}
	if systemPrompt != "" {
		args = append(args, "--system-prompt", systemPrompt)
	}
	if sessionID != "" {
		args = append(args, "--session-id", sessionID)
	}
	return args
}

// ============================================================
// Helpers
// ============================================================

func wrapExitError(op string, err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("%s: exit %d: %s", op, exitErr.ExitCode(), strings.TrimSpace(string(exitErr.Stderr)))
	}
	return fmt.Errorf("%s: %w", op, err)
}

func captureOutput(dir, name string, args ...string) (string, error) {
	cmd := localCommand(dir, name, args...)
	out, err := cmd.Output()
	return string(out), err
}

func localCommand(workDir, name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Dir = workDir
	return cmd
}

func execOutput(workDir string, rt sandbox.SandboxRuntime, name string, args ...string) (string, error) {
	if rt == nil {
		out, err := localCommand(workDir, name, args...).Output()
		if err != nil {
			return "", wrapExitError("claude exec", err)
		}
		return strings.TrimSpace(string(out)), nil
	}
	res, err := rt.Capture(name, args)
	if err != nil {
		return "", runtimeErrorf("claude exec", res, err)
	}
	return strings.TrimSpace(res.Stdout), nil
}

func runtimeErrorf(op string, res *sandbox.ExecutionResult, err error) error {
	if res == nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if strings.TrimSpace(res.Stderr) != "" {
		return fmt.Errorf("%s: exit %d: %s", op, res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	return fmt.Errorf("%s: exit %d: %w", op, res.ExitCode, err)
}

func runtimePID(rt sandbox.SandboxRuntime) string {
	if rt == nil {
		return ""
	}
	sbx := rt.Sandbox()
	if sbx == nil || sbx.State == nil {
		return ""
	}
	return sbx.State.PID
}

func trimSpace(s string) string {
	return strings.TrimSpace(s)
}
