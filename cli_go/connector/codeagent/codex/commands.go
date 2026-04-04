package codex

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
	Codex codeagent.Provider = "codex"
)

// ============================================================
// Session lifecycle
// ============================================================

// Create prepares a codex CLI session.
// Codex has no persistent server-side sessions; "creating" a session means
// (1) applying the caller's params, (2) verifying the codex binary is reachable
// and the user is authenticated, and (3) writing the resolved model into the
// workspace .codex/config.yaml so that any interactive `codex` invocation in
// the same directory inherits the same defaults.
func (a *codexAgent) Create(p codeagent.CreateSessionParams) (*codeagent.CreateSessionResult, error) {
	a.mu.Lock()
	if p.WorkDir != "" {
		a.workDir = p.WorkDir
	}
	if p.Model != "" {
		a.model = p.Model
	}
	id := p.ID
	if id == "" {
		id = generateID()
	}
	a.sessionID = id
	workDir := a.workDir
	model := a.model
	a.mu.Unlock()

	// Verify the codex binary is reachable by running `codex --version`.
	out, err := captureOutput(workDir, "codex", "--version")
	if err != nil {
		return nil, fmt.Errorf("codex: create: binary unreachable: %w", err)
	}
	logger.Debug("Create: codex binary ok", "version", trimSpace(out))

	// Verify authentication via `codex auth status` (exit 0 = authenticated).
	authCmd := exec.Command("codex", "auth", "status")
	authCmd.Dir = workDir
	if err := authCmd.Run(); err != nil {
		logger.Warn("Create: user not authenticated", "err", err)
		return nil, fmt.Errorf("codex: create: not authenticated — run 'codex auth login' first")
	}

	// Persist model into .codex/config.yaml so interactive sessions inherit it.
	if syncErr := syncModelConfig(workDir, model); syncErr != nil {
		// Non-fatal: log and continue — Exec/Stream always pass -m explicitly.
		logger.Warn("Create: could not sync model to config", "err", syncErr)
	}

	logger.Info("Create: session ready", "id", id, "model", model, "workDir", workDir)
	return &codeagent.CreateSessionResult{ID: id, Name: p.Name}, nil
}
func (a *codexAgent) Resume(_ codeagent.ResumeSessionParams) (*codeagent.ResumeSessionResult, error) {
	logger.Warn("Resume: not supported by codex CLI")
	return nil, errors.New("codex: resume: codex CLI has no session resume command")
}

func (a *codexAgent) List(_ codeagent.ListSessionsParams) (*codeagent.ListSessionsResult, error) {
	logger.Warn("List: codex has no session list API")
	return &codeagent.ListSessionsResult{Sessions: nil}, nil
}

func (a *codexAgent) Delete(_ codeagent.DeleteSessionParams) (*codeagent.DeleteSessionResult, error) {
	logger.Warn("Delete: codex has no session delete API")
	return &codeagent.DeleteSessionResult{Deleted: false}, nil
}

func (a *codexAgent) Stop() {
	logger.Info("Stop: no-op for codex non-interactive sessions")
}

// ============================================================
// GetSessionConfig
// ============================================================

func (a *codexAgent) GetSessionConfig(_ codeagent.GetSessionConfigParams) (*codeagent.GetSessionConfigResult, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return &codeagent.GetSessionConfigResult{
		Model:          codeagent.Model{Provider: Codex, Model: a.model},
		PermissionMode: codeagent.PermissionDefault,
		WorkDir:        a.workDir,
	}, nil
}

// ============================================================
// Exec
// ============================================================

func (a *codexAgent) Exec(p codeagent.ExecuteParams) (*codeagent.ExecuteResult, error) {
	a.mu.RLock()
	workDir := a.workDir
	model := a.model
	sbx := a.sbx
	a.mu.RUnlock()

	args := buildExecArgs(p.Prompt, model, p.OutputFormat, p.MaxTurns, sbx)
	logger.Debug("Exec", "workDir", workDir, "args", args)

	cmd := exec.Command("codex", args...)
	cmd.Dir = workDir

	out, err := cmd.Output()
	if err != nil {
		return nil, wrapExitError("codex exec", err)
	}

	response := strings.TrimSpace(string(out))
	logger.Debug("Exec completed", "responseLen", len(response))

	return &codeagent.ExecuteResult{
		PromptID:   p.PromptId,
		Response:   response,
		StopReason: "stop",
	}, nil
}

// ============================================================
// Stream
// ============================================================

func (a *codexAgent) Stream(p codeagent.StreamParams) (*codeagent.StreamResult, error) {
	a.mu.RLock()
	workDir := a.workDir
	model := a.model
	sbx := a.sbx
	a.mu.RUnlock()

	args := buildStreamArgs(p.Prompt, model, p.MaxTurns, sbx)
	logger.Debug("Stream", "workDir", workDir, "args", args)

	cmd := exec.Command("codex", args...)
	cmd.Dir = workDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("codex stream: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("codex stream: start process: %w", err)
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
			ev := parseCodexLine(line)
			ch <- ev
			if ev.Done {
				return
			}
		}
		if err := cmd.Wait(); err != nil {
			msg := wrapExitError("codex stream", err).Error()
			logger.Error("Stream: process exited with error", "err", msg)
			ch <- codeagent.StreamEvent{Type: "stop", Done: true, Content: msg}
			return
		}
		ch <- codeagent.StreamEvent{Type: "stop", Done: true}
		logger.Debug("Stream completed")
	}()

	return &codeagent.StreamResult{Events: ch}, nil
}

// ============================================================
// Arg builders
// ============================================================

func buildExecArgs(prompt, model string, format codeagent.OutputFormat, maxTurns int, sbx *sandbox.Sandbox) []string {
	args := []string{"exec", prompt, "-m", model}
	if format == codeagent.OutputFormatJSON || format == codeagent.OutputFormatStreamJSON {
		args = append(args, "--json")
	}
	if maxTurns > 0 {
		args = append(args, fmt.Sprintf("--max-turns=%d", maxTurns))
	}
	if sf := sandboxFlagValue(sbx); sf != "" {
		args = append(args, "--sandbox", sf)
	}
	return args
}

func buildStreamArgs(prompt, model string, maxTurns int, sbx *sandbox.Sandbox) []string {
	args := []string{"exec", prompt, "-m", model, "--json"}
	if maxTurns > 0 {
		args = append(args, fmt.Sprintf("--max-turns=%d", maxTurns))
	}
	if sf := sandboxFlagValue(sbx); sf != "" {
		args = append(args, "--sandbox", sf)
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
