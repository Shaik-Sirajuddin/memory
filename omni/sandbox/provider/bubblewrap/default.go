package bubblewrap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Shaik-Sirajuddin/memory/sandbox/provider"
)

type Provisioner struct {
	state   provider.ProvisionerState
	sandbox *provider.Sandbox
	options provider.ProvisionerOptions
}

type Runtime struct {
	sandbox     *provider.Sandbox
	provisioner *Provisioner
}

func New(sbx *provider.Sandbox, opts provider.ProvisionerOptions) (*Provisioner, error) {
	return &Provisioner{state: provider.NewProvisionerState(opts.Store), sandbox: provider.CloneSandbox(sbx), options: opts}, nil
}

func (p *Provisioner) Create(params provider.CreateSandboxParams) (provider.SandboxRuntime, error) {
	sbx, err := p.state.Create(provider.ProvisionerBubblewrap, params, p.sandbox)
	if err != nil {
		return nil, err
	}
	return p.wrap(sbx), nil
}

func (p *Provisioner) List(params provider.ListSandboxParams) ([]provider.SandboxRuntime, error) {
	items, err := p.state.List(params)
	if err != nil {
		return nil, err
	}
	out := make([]provider.SandboxRuntime, 0, len(items))
	for _, sbx := range items {
		out = append(out, p.wrap(sbx))
	}
	return out, nil
}
func (p *Provisioner) GetSandbox(params *provider.GetSandboxParams) (provider.SandboxRuntime, error) {
	sbx, err := p.state.Get(params)
	if err != nil {
		return nil, err
	}
	return p.wrap(sbx), nil
}

func (p *Provisioner) wrap(sbx *provider.Sandbox) *Runtime {
	return &Runtime{sandbox: provider.CloneSandbox(sbx), provisioner: p}
}

func (r *Runtime) Sandbox() *provider.Sandbox { return provider.CloneSandbox(r.sandbox) }
func (r *Runtime) Command(command string, args []string) error {
	return r.provisioner.run(r.sandbox, command, args, true)
}
func (r *Runtime) Execute(command string, args []string) error {
	return r.provisioner.run(r.sandbox, command, args, false)
}
func (r *Runtime) Capture(command string, args []string) (*provider.ExecutionResult, error) {
	return r.provisioner.capture(r.sandbox, command, args)
}
func (r *Runtime) Start(command string, args []string) (provider.SandboxProcess, error) {
	return r.provisioner.start(r.sandbox, command, args)
}
func (r *Runtime) Sync(config *provider.Config) error {
	if r.sandbox == nil || r.sandbox.Data == nil {
		return fmt.Errorf("sandbox: runtime sandbox is required")
	}
	r.sandbox.Config = provider.CloneConfig(config)
	r.provisioner.state.SyncOne(r.sandbox.Data.ID, config)
	return nil
}

func (p *Provisioner) BuildCommand(sbx *provider.Sandbox, command string, args []string) (string, []string, error) {
	if strings.TrimSpace(command) == "" {
		return "", nil, fmt.Errorf("sandbox: command is required")
	}
	executable := p.options.Executable
	if executable == "" {
		executable = "bwrap"
	}
	workDir := p.options.WorkDir
	if workDir == "" {
		workDir = "."
	}
	resolvedWorkDir := filepath.Clean(workDir)
	cmdArgs := []string{"--die-with-parent", "--new-session", "--unshare-pid", "--proc", "/proc", "--dev", "/dev", "--ro-bind", "/", "/"}
	if provider.SandboxAllowsWrite(sbx) {
		cmdArgs = append(cmdArgs, "--bind", resolvedWorkDir, resolvedWorkDir)
	} else {
		cmdArgs = append(cmdArgs, "--ro-bind", resolvedWorkDir, resolvedWorkDir)
	}
	for _, dir := range provider.SandboxAccessDirs(sbx) {
		cmdArgs = append(cmdArgs, "--ro-bind", dir, dir)
	}
	for _, dir := range provider.SandboxBlockedDirs(sbx) {
		cmdArgs = append(cmdArgs, "--tmpfs", dir)
	}
	cmdArgs = append(cmdArgs, "--chdir", resolvedWorkDir)
	cmdArgs = append(cmdArgs, p.options.ExtraArgs...)
	cmdArgs = append(cmdArgs, "--", command)
	cmdArgs = append(cmdArgs, args...)
	return executable, cmdArgs, nil
}

func (p *Provisioner) run(sbx *provider.Sandbox, command string, args []string, interactive bool) error {
	cmd, err := p.command(sbx, command, args)
	if err != nil {
		return err
	}
	if interactive {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

func (p *Provisioner) capture(sbx *provider.Sandbox, command string, args []string) (*provider.ExecutionResult, error) {
	cmd, err := p.command(sbx, command, args)
	if err != nil {
		return nil, err
	}
	return provider.RunCaptured(cmd)
}

func (p *Provisioner) start(sbx *provider.Sandbox, command string, args []string) (provider.SandboxProcess, error) {
	cmd, err := p.command(sbx, command, args)
	if err != nil {
		return nil, err
	}
	return provider.StartCaptured(cmd)
}

func (p *Provisioner) command(sbx *provider.Sandbox, command string, args []string) (*exec.Cmd, error) {
	executable, cmdArgs, err := p.BuildCommand(sbx, command, args)
	if err != nil {
		return nil, err
	}
	resolved, err := exec.LookPath(executable)
	if err != nil {
		return nil, fmt.Errorf("sandbox: resolve %s: %w", executable, err)
	}
	return exec.Command(resolved, cmdArgs...), nil
}
