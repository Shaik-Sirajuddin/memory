package gvisor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	sandboxlog "github.com/Shaik-Sirajuddin/memory/sandbox/log"
	"github.com/Shaik-Sirajuddin/memory/sandbox/provider"
)

var logger = sandboxlog.NewLogger("gvisor")

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
	logger.Debug("provisioner init", "hasSandbox", sbx != nil, "executable", opts.Executable, "globalArgs", opts.GlobalArgs, "extraArgs", opts.ExtraArgs)
	return &Provisioner{state: provider.NewProvisionerState(opts.Store), sandbox: provider.CloneSandbox(sbx), options: opts}, nil
}

func (p *Provisioner) Create(params provider.CreateSandboxParams) (provider.SandboxRuntime, error) {
	logger.Debug("create runtime requested", "id", params.ID)
	sbx, err := p.state.Create(provider.ProvisionerGVisor, params, p.sandbox)
	if err != nil {
		logger.Error("create runtime failed", "id", params.ID, "err", err)
		return nil, err
	}
	createdID := ""
	createdPID := ""
	if sbx != nil && sbx.Data != nil {
		createdID = sbx.Data.ID
	}
	if sbx != nil && sbx.State != nil {
		createdPID = sbx.State.PID
	}
	logger.Info("runtime created", "id", createdID, "pid", createdPID)
	return p.wrap(sbx), nil
}

func (p *Provisioner) List(params provider.ListSandboxParams) ([]provider.SandboxRuntime, error) {
	logger.Debug("list runtimes requested", "activeOnly", params.Active)
	items, err := p.state.List(params)
	if err != nil {
		logger.Error("list runtimes failed", "err", err)
		return nil, err
	}
	out := make([]provider.SandboxRuntime, 0, len(items))
	for _, sbx := range items {
		out = append(out, p.wrap(sbx))
	}
	logger.Info("list runtimes completed", "count", len(out))
	return out, nil
}

func (p *Provisioner) GetSandbox(params *provider.GetSandboxParams) (provider.SandboxRuntime, error) {
	logger.Debug("get runtime requested")
	sbx, err := p.state.Get(params)
	if err != nil {
		logger.Error("get runtime failed", "err", err)
		return nil, err
	}
	loadedID := ""
	loadedPID := ""
	if sbx != nil && sbx.Data != nil {
		loadedID = sbx.Data.ID
	}
	if sbx != nil && sbx.State != nil {
		loadedPID = sbx.State.PID
	}
	logger.Info("runtime loaded", "id", loadedID, "pid", loadedPID)
	return p.wrap(sbx), nil
}

func (p *Provisioner) wrap(sbx *provider.Sandbox) *Runtime {
	return &Runtime{sandbox: provider.CloneSandbox(sbx), provisioner: p}
}

func (r *Runtime) Sandbox() *provider.Sandbox { return provider.CloneSandbox(r.sandbox) }

func (r *Runtime) Command(command string, args []string) error {
	logger.Debug("runtime command", "id", runtimeID(r.sandbox), "command", command, "args", args)
	return r.provisioner.run(r.sandbox, command, args, true)
}

func (r *Runtime) Execute(command string, args []string) error {
	logger.Debug("runtime execute", "id", runtimeID(r.sandbox), "command", command, "args", args)
	return r.provisioner.run(r.sandbox, command, args, false)
}

func (r *Runtime) Capture(command string, args []string) (*provider.ExecutionResult, error) {
	logger.Debug("runtime capture", "id", runtimeID(r.sandbox), "command", command, "args", args)
	return r.provisioner.capture(r.sandbox, command, args)
}

func (r *Runtime) Start(command string, args []string) (provider.SandboxProcess, error) {
	logger.Debug("runtime start", "id", runtimeID(r.sandbox), "command", command, "args", args)
	return r.provisioner.start(r.sandbox, command, args)
}

func (r *Runtime) Sync(config *provider.Config) error {
	if r.sandbox == nil || r.sandbox.Data == nil {
		logger.Error("runtime sync failed: missing sandbox")
		return fmt.Errorf("sandbox: runtime sandbox is required")
	}
	r.sandbox.Config = provider.CloneConfig(config)
	r.provisioner.state.SyncOne(r.sandbox.Data.ID, config)
	logger.Info("runtime synced", "id", runtimeID(r.sandbox))
	return nil
}

func (p *Provisioner) ExecCommand(sbx *provider.Sandbox, command string, args []string) (string, []string, error) {
	if sbx == nil || sbx.State == nil || strings.TrimSpace(sbx.State.PID) == "" {
		logger.Error("build command failed: pid missing")
		return "", nil, fmt.Errorf("sandbox: pid is required")
	}
	if strings.TrimSpace(command) == "" {
		logger.Error("build command failed: command missing", "pid", sbx.State.PID)
		return "", nil, fmt.Errorf("sandbox: command is required")
	}
	executable := p.options.Executable
	if executable == "" {
		executable = "runsc"
	}
	cmdArgs := append([]string{}, p.options.GlobalArgs...)
	cmdArgs = append(cmdArgs, "exec", sbx.State.PID, command)
	cmdArgs = append(cmdArgs, args...)
	cmdArgs = append(cmdArgs, p.options.ExtraArgs...)
	logger.Debug("command built", "executable", executable, "args", cmdArgs)
	return executable, cmdArgs, nil
}

func (p *Provisioner) run(sbx *provider.Sandbox, command string, args []string, interactive bool) error {
	cmd, err := p.command(sbx, command, args)
	if err != nil {
		logger.Error("run command build failed", "command", command, "args", args, "err", err)
		return err
	}
	if interactive {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		logger.Error("run failed", "command", command, "args", args, "interactive", interactive, "err", err)
		return err
	}
	logger.Info("run completed", "command", command, "args", args, "interactive", interactive)
	return nil
}

func (p *Provisioner) capture(sbx *provider.Sandbox, command string, args []string) (*provider.ExecutionResult, error) {
	cmd, err := p.command(sbx, command, args)
	if err != nil {
		logger.Error("capture command build failed", "command", command, "args", args, "err", err)
		return nil, err
	}
	out, err := provider.RunCaptured(cmd)
	if err != nil {
		logger.Error("capture failed", "command", command, "args", args, "err", err)
		return nil, err
	}
	logger.Info("capture completed", "command", command, "args", args, "exitCode", out.ExitCode)
	return out, nil
}

func (p *Provisioner) start(sbx *provider.Sandbox, command string, args []string) (provider.SandboxProcess, error) {
	cmd, err := p.command(sbx, command, args)
	if err != nil {
		logger.Error("start command build failed", "command", command, "args", args, "err", err)
		return nil, err
	}
	process, err := provider.StartCaptured(cmd)
	if err != nil {
		logger.Error("start failed", "command", command, "args", args, "err", err)
		return nil, err
	}
	logger.Info("start completed", "command", command, "args", args, "pid", process.PID())
	return process, nil
}

func (p *Provisioner) command(sbx *provider.Sandbox, command string, args []string) (*exec.Cmd, error) {
	executable, cmdArgs, err := p.ExecCommand(sbx, command, args)
	if err != nil {
		return nil, err
	}
	resolved, err := exec.LookPath(executable)
	if err != nil {
		logger.Error("resolve executable failed", "executable", executable, "err", err)
		return nil, fmt.Errorf("sandbox: resolve %s: %w", executable, err)
	}
	logger.Debug("command resolved", "requested", executable, "resolved", resolved)
	return exec.Command(resolved, cmdArgs...), nil
}

func runtimeID(sbx *provider.Sandbox) string {
	if sbx == nil || sbx.Data == nil {
		return ""
	}
	return sbx.Data.ID
}

/***

***/
