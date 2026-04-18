package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type Doctor struct{}

type HealthStatus struct {
	Provider  ProvisionerKind
	Installed bool
	Binary    string
	OS        string
}

var (
	lookupPath         = exec.LookPath
	resolveScriptPath  = defaultInstallScriptPath
	runInstallScriptFn = runInstallScript
	currentOS          = func() string { return runtime.GOOS }
)

func NewDoctor() *Doctor {
	return &Doctor{}
}

func (d *Doctor) Health() HealthStatus {
	osName := currentOS()
	switch osName {
	case "linux":
		path, err := lookupPath("runsc")
		if err != nil {
			return HealthStatus{Provider: ProvisionerGVisor, Installed: false, OS: osName}
		}
		return HealthStatus{Provider: ProvisionerGVisor, Installed: true, Binary: path, OS: osName}
	case "darwin":
		path, err := lookupPath("sandbox-exec")
		if err != nil {
			return HealthStatus{Provider: ProvisionerSeatbelt, Installed: false, OS: osName}
		}
		return HealthStatus{Provider: ProvisionerSeatbelt, Installed: true, Binary: path, OS: osName}
	default:
		return HealthStatus{Provider: "", Installed: false, OS: osName}
	}
}

func (d *Doctor) Install() error {
	status := d.Health()
	if status.Provider != ProvisionerGVisor {
		return nil
	}
	if status.Installed {
		return nil
	}
	scriptPath, err := resolveScriptPath()
	if err != nil {
		return err
	}
	return runInstallScriptFn(scriptPath)
}

func runInstallScript(path string) error {
	cmd := exec.Command("sh", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sandbox doctor install failed: %w", err)
	}
	return nil
}

func defaultInstallScriptPath() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("sandbox doctor: resolve caller path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	scriptPath := filepath.Join(root, "scripts", "install-sandbox-runtime.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		return "", fmt.Errorf("sandbox doctor: install script not found at %s: %w", scriptPath, err)
	}
	return scriptPath, nil
}
