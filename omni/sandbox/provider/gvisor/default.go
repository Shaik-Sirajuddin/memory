package gvisor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	sandboxlog "github.com/Shaik-Sirajuddin/memory/sandbox/log"
	"github.com/Shaik-Sirajuddin/memory/sandbox/provider"
	"github.com/adrg/xdg"
)

var logger = sandboxlog.NewLogger("gvisor")

type Provisioner struct {
	state   provider.ProvisionerState
	sandbox *provider.Sandbox
	options provider.ProvisionerOptions
	parser  provider.SandboxConfigParser
}

type Runtime struct {
	sandbox     *provider.Sandbox
	provisioner *Provisioner
}

func New(sbx *provider.Sandbox, opts provider.ProvisionerOptions) (*Provisioner, error) {
	logger.Debug("provisioner init", "hasSandbox", sbx != nil, "executable", opts.Executable, "globalArgs", opts.GlobalArgs, "extraArgs", opts.ExtraArgs)
	return &Provisioner{
		state:   provider.NewProvisionerState(opts.Store),
		sandbox: provider.CloneSandbox(sbx),
		options: opts,
		parser:  gvisorConfigParser{},
	}, nil
}

func (p *Provisioner) Create(params provider.CreateSandboxParams) (provider.SandboxRuntime, error) {
	logger.Debug("create runtime requested", "id", params.ID)
	cfg := params.Config
	if cfg == nil && p.sandbox != nil {
		cfg = p.sandbox.Config
	}
	parsed, err := p.parseConfig(cfg)
	if err != nil {
		logger.Error("runtime config parse failed", "id", params.ID, "err", err)
		return nil, err
	}
	logger.Debug("runtime config parsed", "id", params.ID, "allowWrite", parsed.AllowWrite, "accessDirs", parsed.AccessDirs, "blockedDirs", parsed.BlockedDirs)
	if err := p.ensureRuntimeCreated(params.ID); err != nil {
		logger.Error("runtime create failed", "id", params.ID, "err", err)
		return nil, err
	}
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

func (p *Provisioner) UpdateSandbox(params *provider.UpdateSandboxParams) (provider.SandboxRuntime, error) {
	if params == nil {
		return nil, fmt.Errorf("sandbox: update sandbox params are required")
	}
	id := strings.TrimSpace(params.ID)
	if id == "" {
		return nil, fmt.Errorf("sandbox: update sandbox id is required")
	}
	parsed, err := p.parseConfig(params.Config)
	if err != nil {
		return nil, err
	}
	logger.Debug("update sandbox config parsed", "id", id, "allowWrite", parsed.AllowWrite, "accessDirs", parsed.AccessDirs, "blockedDirs", parsed.BlockedDirs)
	name := id
	rt, err := p.GetSandbox(&provider.GetSandboxParams{Name: &name, Active: false})
	if err != nil {
		return nil, err
	}
	if err := rt.Sync(params.Config); err != nil {
		return nil, err
	}
	return rt, nil
}

func (p *Provisioner) List(params provider.ListSandboxParams) ([]provider.SandboxRuntime, error) {
	logger.Debug("list runtimes requested", "activeOnly", params.Active)
	items, err := p.state.List(params)
	if err != nil {
		logger.Error("list runtimes failed", "err", err)
		return nil, err
	}
	items = p.reconcileRuntimeState(items)
	out := make([]provider.SandboxRuntime, 0, len(items))
	for _, sbx := range items {
		out = append(out, p.wrap(sbx))
	}
	logger.Info("list runtimes completed", "count", len(out))
	return out, nil
}

func (p *Provisioner) GetSandbox(params *provider.GetSandboxParams) (provider.SandboxRuntime, error) {
	logger.Debug("get runtime requested")
	items, err := p.List(provider.ListSandboxParams{Active: false})
	if err != nil {
		return nil, err
	}
	for _, rt := range items {
		sbx := rt.Sandbox()
		if sbx == nil {
			continue
		}
		if params != nil {
			if params.Active && (sbx.State == nil || !sbx.State.Active) {
				continue
			}
			if params.PID != nil && (sbx.State == nil || sbx.State.PID != *params.PID) {
				continue
			}
			if params.Name != nil && (sbx.Data == nil || sbx.Data.ID != *params.Name) {
				continue
			}
		}
		loadedID := ""
		loadedPID := ""
		if sbx.Data != nil {
			loadedID = sbx.Data.ID
		}
		if sbx.State != nil {
			loadedPID = sbx.State.PID
		}
		logger.Info("runtime loaded", "id", loadedID, "pid", loadedPID)
		return p.wrap(sbx), nil
	}
	logger.Error("get runtime failed", "err", provider.NoProcessFound)
	return nil, provider.NoProcessFound
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

func (p *Provisioner) CreateDir(path string) error {
	target, err := p.resolveDir(path)
	if err != nil {
		return err
	}
	return os.MkdirAll(target, 0o755)
}

func (p *Provisioner) ListDirs(path string) ([]string, error) {
	target, err := p.resolveDir(path)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			out = append(out, entry.Name())
		}
	}
	return out, nil
}

func (p *Provisioner) resolveDir(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("sandbox: directory path is required")
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	workDir := strings.TrimSpace(p.options.WorkDir)
	if workDir == "" {
		return "", fmt.Errorf("sandbox: relative directory path requires provisioner WorkDir")
	}
	return filepath.Join(filepath.Clean(workDir), path), nil
}

type runtimeEntry struct {
	ID     string
	PID    string
	Status string
}

type ociSpec struct {
	OCIVersion string `json:"ociVersion"`
	Process    struct {
		Terminal bool     `json:"terminal"`
		Args     []string `json:"args"`
		Cwd      string   `json:"cwd"`
	} `json:"process"`
	Root struct {
		Path     string `json:"path"`
		Readonly bool   `json:"readonly"`
	} `json:"root"`
}

type gvisorConfigParser struct{}

func (gvisorConfigParser) Parse(config *provider.Config) (*provider.ParsedSandboxConfig, error) {
	sbx := &provider.Sandbox{Config: provider.CloneConfig(config)}
	return &provider.ParsedSandboxConfig{
		AllowWrite:  provider.SandboxAllowsWrite(sbx),
		AccessDirs:  provider.SandboxAccessDirs(sbx),
		BlockedDirs: provider.SandboxBlockedDirs(sbx),
	}, nil
}

func (p *Provisioner) parseConfig(config *provider.Config) (*provider.ParsedSandboxConfig, error) {
	if p.parser == nil {
		return nil, fmt.Errorf("sandbox: config parser is required")
	}
	return p.parser.Parse(config)
}

func (p *Provisioner) ensureRuntimeCreated(id string) error {
	bundle, err := p.resolveBundlePath(id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("sandbox: id is required")
	}
	entries, err := p.runtimeList()
	if err == nil {
		for _, entry := range entries {
			if entry.ID == id {
				logger.Debug("runtime already present", "id", id, "status", entry.Status)
				return nil
			}
		}
	}
	if err := p.runscManage("create", "--bundle", bundle, id); err != nil {
		return fmt.Errorf("sandbox: runsc create %s: %w", id, err)
	}
	if err := p.runscManage("start", id); err != nil {
		_ = p.runscManage("delete", id)
		return fmt.Errorf("sandbox: runsc start %s: %w", id, err)
	}
	logger.Info("runtime created in runsc", "id", id, "bundle", bundle)
	return nil
}

func (p *Provisioner) resolveBundlePath(id string) (string, error) {
	if bundle, ok := p.bundlePath(); ok {
		return bundle, nil
	}
	return p.ensureDefaultBundle(id)
}

func (p *Provisioner) ensureDefaultBundle(id string) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", fmt.Errorf("sandbox: id is required")
	}
	path, err := xdg.DataFile(filepath.Join("memory", "sandboxes", "gvisor", "bundles", id, ".keep"))
	if err != nil {
		return "", fmt.Errorf("sandbox: resolve default gvisor bundle dir: %w", err)
	}
	bundleDir := filepath.Dir(path)
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		return "", fmt.Errorf("sandbox: create default gvisor bundle dir: %w", err)
	}
	configPath := filepath.Join(bundleDir, "config.json")
	if _, err := os.Stat(configPath); err == nil {
		return bundleDir, nil
	}
	spec := defaultSpec()
	raw, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return "", fmt.Errorf("sandbox: marshal default gvisor spec: %w", err)
	}
	if err := os.WriteFile(configPath, raw, 0o644); err != nil {
		return "", fmt.Errorf("sandbox: write default gvisor spec: %w", err)
	}
	logger.Info("default gvisor bundle created", "id", id, "bundle", bundleDir)
	return bundleDir, nil
}

func defaultSpec() ociSpec {
	var spec ociSpec
	spec.OCIVersion = "1.0.2"
	spec.Process.Terminal = false
	spec.Process.Args = []string{"/bin/sh", "-c", "sleep infinity"}
	spec.Process.Cwd = "/"
	spec.Root.Path = "/"
	spec.Root.Readonly = true
	return spec
}

func (p *Provisioner) bundlePath() (string, bool) {
	workDir := strings.TrimSpace(p.options.WorkDir)
	if workDir == "" {
		return "", false
	}
	bundle := filepath.Clean(workDir)
	configPath := filepath.Join(bundle, "config.json")
	if _, err := os.Stat(configPath); err != nil {
		return "", false
	}
	return bundle, true
}

func (p *Provisioner) reconcileRuntimeState(items []*provider.Sandbox) []*provider.Sandbox {
	entries, err := p.runtimeList()
	if err != nil {
		logger.Debug("runtime list unavailable for reconciliation", "err", err)
		return items
	}
	if len(entries) == 0 {
		return items
	}
	byID := make(map[string]runtimeEntry, len(entries))
	for _, entry := range entries {
		byID[entry.ID] = entry
	}
	out := make([]*provider.Sandbox, 0, len(items)+len(entries))
	seen := map[string]struct{}{}
	for _, sbx := range items {
		if sbx == nil || sbx.Data == nil {
			continue
		}
		entry, ok := byID[sbx.Data.ID]
		if sbx.State == nil {
			sbx.State = &provider.State{PID: sbx.Data.ID}
		}
		if ok {
			sbx.State.Active = runtimeActive(entry.Status)
			seen[sbx.Data.ID] = struct{}{}
		}
		out = append(out, provider.CloneSandbox(sbx))
	}
	for _, entry := range entries {
		if _, ok := seen[entry.ID]; ok {
			continue
		}
		out = append(out, &provider.Sandbox{
			Config: provider.CloneConfig(p.sandbox.Config),
			State: &provider.State{
				PID:    entry.ID,
				Active: runtimeActive(entry.Status),
			},
			Data: &provider.Data{
				ID:          entry.ID,
				Application: string(provider.ProvisionerGVisor),
				CreatedAt:   time.Now().UTC().Format(time.RFC3339),
			},
		})
	}
	return out
}

func (p *Provisioner) runtimeList() ([]runtimeEntry, error) {
	out, err := p.runscManageOutput("list")
	if err != nil {
		return nil, err
	}
	return parseRuntimeList(out), nil
}

func parseRuntimeList(output string) []runtimeEntry {
	entries := []runtimeEntry{}
	sc := bufio.NewScanner(strings.NewReader(output))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if strings.EqualFold(fields[0], "id") {
			continue
		}
		entries = append(entries, runtimeEntry{
			ID:     fields[0],
			PID:    fields[1],
			Status: strings.ToLower(fields[2]),
		})
	}
	return entries
}

func runtimeActive(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "created", "paused":
		return true
	default:
		return false
	}
}

func (p *Provisioner) runscManageOutput(args ...string) (string, error) {
	cmd, err := p.runscManageCommand(args...)
	if err != nil {
		return "", err
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (p *Provisioner) runscManage(args ...string) error {
	cmd, err := p.runscManageCommand(args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (p *Provisioner) runscManageCommand(args ...string) (*exec.Cmd, error) {
	executable := p.options.Executable
	if strings.TrimSpace(executable) == "" {
		executable = "runsc"
	}
	resolved, err := exec.LookPath(executable)
	if err != nil {
		return nil, fmt.Errorf("sandbox: resolve %s: %w", executable, err)
	}
	cmdArgs := append([]string{}, p.options.GlobalArgs...)
	if strings.TrimSpace(p.options.RuntimeRoot) != "" && !containsRunscRootFlag(cmdArgs) {
		cmdArgs = append(cmdArgs, "--root", p.options.RuntimeRoot)
	}
	cmdArgs = append(cmdArgs, args...)
	return exec.Command(resolved, cmdArgs...), nil
}

func containsRunscRootFlag(args []string) bool {
	for i := range args {
		if args[i] == "--root" || strings.HasPrefix(args[i], "--root=") {
			return true
		}
	}
	return false
}

/***
***/
