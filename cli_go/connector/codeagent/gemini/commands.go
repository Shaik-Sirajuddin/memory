package gemini

import (
	"bufio"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
	"github.com/Shaik-Sirajuddin/memory/connector/sandbox"
)

const (
	Gemini codeagent.Provider = "gemini"
)

// Create prepares a Gemini CLI session and syncs model/approval/sandbox settings.
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

func (a *geminiAgent) Resume(_ codeagent.ResumeSessionParams) (*codeagent.ResumeSessionResult, error) {
	logger.Warn("Resume: gemini CLI resume is not implemented")
	return nil, errors.New("gemini: resume: not implemented")
}

func (a *geminiAgent) List(_ codeagent.ListSessionsParams) (*codeagent.ListSessionsResult, error) {
	logger.Warn("List: gemini CLI has no session list API")
	return &codeagent.ListSessionsResult{Sessions: nil}, nil
}

func (a *geminiAgent) Delete(_ codeagent.DeleteSessionParams) (*codeagent.DeleteSessionResult, error) {
	logger.Warn("Delete: gemini CLI has no session delete API")
	return &codeagent.DeleteSessionResult{Deleted: false}, nil
}

func (a *geminiAgent) Stop() {
	logger.Info("Stop: no-op for gemini non-interactive sessions")
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

func buildExecArgs(prompt, model string, permMode codeagent.PermissionMode, systemPrompt string, format codeagent.OutputFormat, maxTurns int, sbx *sandbox.Sandbox) []string {
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

func buildStreamArgs(prompt, model string, permMode codeagent.PermissionMode, systemPrompt string, maxTurns int, sbx *sandbox.Sandbox) []string {
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
