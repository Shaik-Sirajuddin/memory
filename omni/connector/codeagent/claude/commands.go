package claude

import (
	"bufio"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
)

const (
	Claude codeagent.Provider = "claude"
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

	logger.Info("Create: session ready", "id", id, "workDir", workDir)
	return &codeagent.CreateSessionResult{ID: id, Name: p.Name}, nil
}

// Resume launches an interactive claude session via `claude -r <id>`.
func (a *claudeAgent) Resume(p codeagent.ResumeSessionParams) (*codeagent.ResumeSessionResult, error) {
	a.mu.RLock()
	workDir := a.workDir
	a.mu.RUnlock()

	args := []string{"-r", p.ID}
	if p.ForkSession {
		args = append(args, "--fork-session")
	}

	cmd := exec.Command("claude", args...)
	cmd.Dir = workDir
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("claude: resume: start process: %w", err)
	}

	pid := fmt.Sprintf("%d", cmd.Process.Pid)
	logger.Info("Resume: interactive session started", "pid", pid, "sessionID", p.ID)
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
	a.mu.RUnlock()

	args := buildExecArgs(p.Prompt, model, permMode, systemPrompt, sessionID, p.OutputFormat, p.MaxTurns)
	logger.Debug("Exec", "workDir", workDir, "args", args)

	cmd := exec.Command("claude", args...)
	cmd.Dir = workDir

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
	a.mu.RUnlock()

	args := buildStreamArgs(p.Prompt, model, permMode, systemPrompt, sessionID, p.MaxTurns)
	logger.Debug("Stream", "workDir", workDir, "args", args)

	cmd := exec.Command("claude", args...)
	cmd.Dir = workDir

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

func trimSpace(s string) string {
	return strings.TrimSpace(s)
}
