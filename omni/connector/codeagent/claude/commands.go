package claude

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
	sandbox "github.com/Shaik-Sirajuddin/memory/sandbox/provider"
	"github.com/Shaik-Sirajuddin/memory/sandbox/provider/bubblewrap"
	"github.com/Shaik-Sirajuddin/memory/sandbox/provider/gvisor"
	"github.com/Shaik-Sirajuddin/memory/sandbox/provider/seatbelt"
)

const (
	Claude codeagent.Provider = "claude"
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
	a.sessionID = id
	if p.RunTime != nil {
		a.sbxRuntime = *p.RunTime
	}
	workDir := a.workDir
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

	// Seed the session into Claude's session store by running a minimal
	// print-mode call with --session-id. Without this, `claude -r <id>`
	// fails with "No conversation found" because Claude only persists a
	// session after at least one print-mode exchange.
	a.mu.RLock()
	model := a.model
	a.mu.RUnlock()

	seedArgs := []string{
		"-p", "hello",
		"--session-id", id,
		"--model", model,
		"--output-format", "json",
		"--max-turns", "1",
	}
	seedCmd, cmdErr := a.commandFor(workDir, nil, "claude", seedArgs...)
	if cmdErr != nil {
		return nil, fmt.Errorf("claude: create: seed command: %w", cmdErr)
	}
	seedOut, seedErr := seedCmd.Output()
	if seedErr != nil {
		return nil, fmt.Errorf("claude: create: seed session: %w", wrapExitError("claude seed", seedErr))
	}
	logger.Debug("Create: session seeded", "id", id, "output", trimSpace(string(seedOut)))

	logger.Info("Create: session ready", "id", id, "workDir", workDir)
	return &codeagent.CreateSessionResult{ID: id, Name: p.Name}, nil
}

// Resume launches an interactive claude session via `claude -r <id>`.
func (a *claudeAgent) Resume(p codeagent.ResumeSessionParams) (*codeagent.ResumeSessionResult, error) {
	a.mu.RLock()
	workDir := a.workDir
	rt := a.sbxRuntime
	a.mu.RUnlock()

	args := []string{"-r", p.ID}
	if p.ForkSession {
		args = append(args, "--fork-session")
	}

	cmd, err := a.commandFor(workDir, rt, "claude", args...)
	if err != nil {
		return nil, fmt.Errorf("claude: resume: sandbox command: %w", err)
	}

	// Prefer injectable streams (set by tests). In production, open /dev/tty
	// directly so the child gets a proper controlling terminal for raw mode.
	// Pipe-like fds from os.Stdin/Stdout cause EIO on setRawMode.
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
	logger.Info("Resume: interactive session started", "pid", pid, "sessionID", p.ID)

	// Block until the interactive session ends. This keeps the tty fd open
	// for the full duration and prevents the caller from racing with the child.
	_ = cmd.Wait()

	return &codeagent.ResumeSessionResult{ProcessID: pid, SessionID: p.ID}, nil
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

	cmd, err := a.commandFor(workDir, rt, "claude", args...)
	if err != nil {
		return nil, fmt.Errorf("claude exec: sandbox command: %w", err)
	}

	out, err := cmd.Output()
	if err != nil {
		return nil, wrapExitError("claude exec", err)
	}

	response := strings.TrimSpace(string(out))
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

	cmd, err := a.commandFor(workDir, rt, "claude", args...)
	if err != nil {
		return nil, fmt.Errorf("claude stream: sandbox command: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("claude stream: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("claude stream: start process: %w", err)
	}

	ch := make(chan codeagent.StreamEvent, 32)

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
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

func (a *claudeAgent) commandFor(workDir string, rt sandbox.SandboxRuntime, name string, args ...string) (*exec.Cmd, error) {
	a.mu.RLock()
	provisioner := a.sbxProvisioner
	runtime := a.sbxRuntime
	a.mu.RUnlock()

	if runtime == nil || provisioner == nil {
		cmd := exec.Command(name, args...)
		cmd.Dir = workDir
		return cmd, nil
	}

	sbx := runtime.Sandbox()
	var executable string
	var wrappedArgs []string
	var err error

	switch p := provisioner.(type) {
	case *bubblewrap.Provisioner:
		executable, wrappedArgs, err = p.BuildCommand(sbx, name, args)
	case *seatbelt.Provisioner:
		executable, wrappedArgs, err = p.BuildCommand(sbx, name, args)
	case *gvisor.Provisioner:
		executable, wrappedArgs, err = p.ExecCommand(sbx, name, args)
	default:
		return nil, fmt.Errorf("unsupported sandbox provisioner %T", provisioner)
	}
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(executable, wrappedArgs...)
	cmd.Dir = workDir
	return cmd, nil
}

func trimSpace(s string) string {
	return strings.TrimSpace(s)
}
