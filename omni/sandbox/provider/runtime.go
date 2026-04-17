package provider

import (
	"bytes"
	"io"
	"os/exec"
)

type startedProcess struct {
	cmd    *exec.Cmd
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func RunCaptured(cmd *exec.Cmd) (*ExecutionResult, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return &ExecutionResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode(err),
	}, err
}

func StartCaptured(cmd *exec.Cmd) (SandboxProcess, error) {
	p := &startedProcess{cmd: cmd}
	cmd.Stdout = &p.stdout
	cmd.Stderr = &p.stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *startedProcess) PID() int {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *startedProcess) Stdout() io.Reader { return bytes.NewReader(p.stdout.Bytes()) }
func (p *startedProcess) Stderr() io.Reader { return bytes.NewReader(p.stderr.Bytes()) }

func (p *startedProcess) Wait() (*ExecutionResult, error) {
	err := p.cmd.Wait()
	return &ExecutionResult{
		Stdout:   p.stdout.String(),
		Stderr:   p.stderr.String(),
		ExitCode: exitCode(err),
	}, err
}

func (p *startedProcess) Kill() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return -1
}
