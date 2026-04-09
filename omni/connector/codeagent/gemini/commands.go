package gemini

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
	sandbox "github.com/Shaik-Sirajuddin/memory/sandbox/provider"
)

const (
	Gemini codeagent.Provider = "gemini"
)

var (
	interactiveStdin  io.Reader = os.Stdin
	interactiveStdout io.Writer = os.Stdout
	interactiveStderr io.Writer = os.Stderr
)

// Create prepares a Gemini CLI session and syncs model/approval/sandbox settings.
// TODO : Missing validation checks on model
func (a *geminiAgent) Create(p codeagent.CreateSessionParams) (*codeagent.CreateSessionResult, error) {
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
	model := a.model
	permMode := a.permMode
	a.mu.Unlock()

	out, err := captureOutput(workDir, "gemini", "--version")
	if err != nil {
		return nil, fmt.Errorf("gemini: create: binary unreachable: %w", err)
	}
	logger.Debug("Create: gemini binary ok", "version", trimSpace(out))

	if err := syncModelAndModeConfig(workDir, model, permMode); err != nil {
		logger.Warn("Create: could not sync model/mode config", "err", err)
	}
	if err := a.syncSandboxConfig(); err != nil {
		logger.Warn("Create: could not sync sandbox config", "err", err)
	}

	return &codeagent.CreateSessionResult{ID: id, Name: p.Name}, nil
}

func (a *geminiAgent) Resume(p codeagent.ResumeSessionParams) (*codeagent.ResumeSessionResult, error) {
	a.mu.RLock()
	workDir := a.workDir
	a.mu.RUnlock()

	geminiArgs := []string{"--resume"}
	if p.ID != "" {
		geminiArgs = append(geminiArgs, p.ID)
	}
	if p.ForkSession {
		// Gemini CLI does not expose a fork-session flag in non-interactive args.
		logger.Warn("Resume: ForkSession requested but unsupported by gemini CLI; continuing without fork")
	}

	cmd := exec.Command(resumeShell(), "-lc", buildShellExecCommand("gemini", geminiArgs...))
	cmd.Dir = workDir
	cmd.Stdin = interactiveStdin
	cmd.Stdout = interactiveStdout
	cmd.Stderr = interactiveStderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("gemini: resume: start process: %w", err)
	}

	pid := fmt.Sprintf("%d", cmd.Process.Pid)
	a.mu.Lock()
	a.activeCmd = cmd
	resolvedSessionID := p.ID
	if resolvedSessionID == "" {
		resolvedSessionID = "latest"
	}
	a.sessionID = resolvedSessionID
	a.mu.Unlock()

	logger.Info("Resume: interactive session started", "pid", pid, "sessionID", resolvedSessionID)
	return &codeagent.ResumeSessionResult{ProcessID: pid, SessionID: resolvedSessionID}, nil
}

func (a *geminiAgent) List(_ codeagent.ListSessionsParams) (*codeagent.ListSessionsResult, error) {
	a.mu.RLock()
	workDir := a.workDir
	model := a.model
	a.mu.RUnlock()

	out, err := captureOutput(workDir, "gemini", "--list-sessions")
	if err != nil {
		return nil, fmt.Errorf("gemini: list sessions: %w", err)
	}

	sessions := parseSessionList(out, workDir, model)
	return &codeagent.ListSessionsResult{Sessions: sessions}, nil
}

func (a *geminiAgent) Delete(p codeagent.DeleteSessionParams) (*codeagent.DeleteSessionResult, error) {
	a.mu.RLock()
	workDir := a.workDir
	a.mu.RUnlock()

	if strings.TrimSpace(p.ID) == "" {
		return nil, errors.New("gemini: delete session: empty session id")
	}
	if _, err := captureOutput(workDir, "gemini", "--delete-session", p.ID); err != nil {
		return nil, fmt.Errorf("gemini: delete session %q: %w", p.ID, err)
	}
	logger.Info("Delete: session deleted", "sessionID", p.ID)
	return &codeagent.DeleteSessionResult{Deleted: true}, nil
}

func (a *geminiAgent) Stop() {
	a.mu.Lock()
	cmd := a.activeCmd
	a.activeCmd = nil
	a.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		logger.Info("Stop: no active gemini process")
		return
	}
	if err := cmd.Process.Kill(); err != nil {
		logger.Warn("Stop: failed to kill active process", "err", err)
		return
	}
	if err := cmd.Wait(); err != nil {
		logger.Debug("Stop: active gemini process reaped", "err", err)
	}
	logger.Info("Stop: active gemini process terminated")
}

func (a *geminiAgent) GetSessionConfig(_ codeagent.GetSessionConfigParams) (*codeagent.GetSessionConfigResult, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return &codeagent.GetSessionConfigResult{
		Model:          codeagent.Model{Provider: Gemini, Model: a.model},
		PermissionMode: a.permMode,
		WorkDir:        a.workDir,
		SystemPrompt:   a.systemPrompt,
	}, nil
}

func (a *geminiAgent) Exec(p codeagent.ExecuteParams) (*codeagent.ExecuteResult, error) {
	a.mu.RLock()
	workDir := a.workDir
	model := a.model
	permMode := a.permMode
	systemPrompt := a.systemPrompt
	sessionID := a.sessionID
	sbx := a.sbx
	a.mu.RUnlock()

	args := buildExecArgs(p.Prompt, model, permMode, systemPrompt, p.OutputFormat, p.MaxTurns, sbx)
	logger.Debug("Exec", "workDir", workDir, "args", args)

	cmd := exec.Command("gemini", args...)
	cmd.Dir = workDir

	out, err := cmd.Output()
	if err != nil {
		return nil, wrapExitError("gemini exec", err)
	}

	response := strings.TrimSpace(string(out))
	return &codeagent.ExecuteResult{
		PromptID:   p.PromptId,
		SessionID:  sessionID,
		Response:   response,
		StopReason: "stop",
	}, nil
}

func (a *geminiAgent) Stream(p codeagent.StreamParams) (*codeagent.StreamResult, error) {
	a.mu.RLock()
	workDir := a.workDir
	model := a.model
	permMode := a.permMode
	systemPrompt := a.systemPrompt
	sessionID := a.sessionID
	sbx := a.sbx
	a.mu.RUnlock()

	args := buildStreamArgs(p.Prompt, model, permMode, systemPrompt, p.MaxTurns, sbx)
	logger.Debug("Stream", "workDir", workDir, "args", args)

	cmd := exec.Command("gemini", args...)
	cmd.Dir = workDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("gemini stream: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("gemini stream: start process: %w", err)
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
			ev := parseGeminiLine(line)
			ch <- ev
			if ev.Done {
				return
			}
		}
		if err := cmd.Wait(); err != nil {
			msg := wrapExitError("gemini stream", err).Error()
			logger.Error("Stream: process exited with error", "err", msg)
			ch <- codeagent.StreamEvent{Type: "stop", Done: true, Content: msg}
			return
		}
		ch <- codeagent.StreamEvent{Type: "stop", Done: true}
	}()

	return &codeagent.StreamResult{Events: ch, SessionID: sessionID}, nil
}

func buildExecArgs(prompt, model string, permMode codeagent.PermissionMode, systemPrompt string, format codeagent.OutputFormat, maxTurns int, sbx *sandbox.Config) []string {
	args := []string{}
	if prompt != "" {
		args = append(args, prompt)
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	if am := approvalModeFlag(permMode); am != "" {
		args = append(args, "--approval-mode", am)
	}
	if systemPrompt != "" {
		args = append(args, "--system-prompt", systemPrompt)
	}
	if maxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", maxTurns))
	}
	if sf := sandboxFlagValue(sbx); sf == "danger-full-access" {
		args = append(args, "--yolo")
	}
	if format == codeagent.OutputFormatJSON || format == codeagent.OutputFormatStreamJSON {
		args = append(args, "--acp")
	}
	return args
}

func buildStreamArgs(prompt, model string, permMode codeagent.PermissionMode, systemPrompt string, maxTurns int, sbx *sandbox.Config) []string {
	return buildExecArgs(prompt, model, permMode, systemPrompt, codeagent.OutputFormatStreamJSON, maxTurns, sbx)
}

func approvalModeFlag(mode codeagent.PermissionMode) string {
	switch mode {
	case "", codeagent.PermissionDefault:
		return "default"
	case codeagent.PermissionPlan:
		return "plan"
	case codeagent.PermissionAcceptEdits, codeagent.PermissionAuto:
		return "auto_edit"
	case codeagent.PermissionDontAsk, codeagent.PermissionBypassPermissions:
		return "yolo"
	default:
		return "default"
	}
}

func permissionFromApprovalMode(mode string) codeagent.PermissionMode {
	switch strings.TrimSpace(mode) {
	case "default":
		return codeagent.PermissionDefault
	case "plan":
		return codeagent.PermissionPlan
	case "auto_edit":
		return codeagent.PermissionAcceptEdits
	case "yolo":
		return codeagent.PermissionBypassPermissions
	default:
		return ""
	}
}

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

func resumeShell() string {
	if shell := strings.TrimSpace(os.Getenv("SHELL")); shell != "" {
		return shell
	}
	return "bash"
}

func buildShellExecCommand(name string, args ...string) string {
	parts := make([]string, 0, len(args)+2)
	parts = append(parts, "exec", shellQuote(name))
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuote(v string) string {
	if v == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(v, "'", `'"'"'`) + "'"
}

var sessionLinePattern = regexp.MustCompile(`^\s*\d+\.\s*(.*?)\s*\[([^\]]+)\]\s*$`)

func parseSessionList(raw, workDir, model string) []*codeagent.Session {
	var sessions []*codeagent.Session
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		m := sessionLinePattern.FindStringSubmatch(line)
		if len(m) != 3 {
			continue
		}
		name := strings.TrimSpace(m[1])
		id := strings.TrimSpace(m[2])
		sessions = append(sessions, &codeagent.Session{
			ID:       id,
			Name:     name,
			Provider: Gemini,
			Model:    model,
			WorkDir:  workDir,
		})
	}
	return sessions
}
