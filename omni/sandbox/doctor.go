package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

type Doctor struct{}

type HealthStatus struct {
	Provider  ProvisionerKind
	Installed bool
	Binary    string
	OS        string
	Missing   []string
	Next      string
}

var (
	lookupPath         = exec.LookPath
	statPath           = os.Stat
	readFile           = os.ReadFile
	resolveScriptPath  = defaultInstallScriptPath
	runInstallScriptFn = runInstallScript
	currentOS          = func() string { return runtime.GOOS }
	currentEUID        = os.Geteuid
	currentUsername    = resolveCurrentUsername
)

func NewDoctor() *Doctor {
	return &Doctor{}
}

func (d *Doctor) Health() HealthStatus {
	osName := currentOS()
	switch osName {
	case "linux":
		return d.linuxHealth(osName)
	case "darwin":
		path, err := lookupPath("sandbox-exec")
		if err != nil {
			return HealthStatus{
				Provider:  ProvisionerSeatbelt,
				Installed: false,
				OS:        osName,
				Missing:   []string{"sandbox-exec"},
				Next:      "install/enable sandbox-exec",
			}
		}
		return HealthStatus{
			Provider:  ProvisionerSeatbelt,
			Installed: true,
			Binary:    path,
			OS:        osName,
			Next:      "-",
		}
	default:
		return HealthStatus{
			Provider:  "",
			Installed: false,
			OS:        osName,
			Next:      "unsupported OS/runtime",
		}
	}
}

func (d *Doctor) linuxHealth(osName string) HealthStatus {
	status := HealthStatus{Provider: ProvisionerGVisor, Installed: false, OS: osName}
	runscPath, err := lookupPath("runsc")
	if err != nil {
		status.Missing = append(status.Missing, "runsc")
		status.Next = "run: omni doctor install"
		return status
	}
	status.Binary = runscPath

	// Rootless gVisor setup requires uidmap helpers for user namespace mapping.
	if currentEUID() != 0 {
		if uidErr := validateUIDMapHelper("newuidmap"); uidErr != nil {
			status.Missing = append(status.Missing, "newuidmap")
		}
		if gidErr := validateUIDMapHelper("newgidmap"); gidErr != nil {
			status.Missing = append(status.Missing, "newgidmap")
		}
		username, userErr := currentUsername()
		if userErr != nil {
			status.Missing = append(status.Missing, "username")
		} else {
			if subUIDErr := ensureSubIDConfigured("/etc/subuid", username); subUIDErr != nil {
				status.Missing = append(status.Missing, "subuid")
			}
			if subGIDErr := ensureSubIDConfigured("/etc/subgid", username); subGIDErr != nil {
				status.Missing = append(status.Missing, "subgid")
			}
		}
	}
	if len(status.Missing) > 0 {
		status.Next = "run: omni doctor install"
		return status
	}

	status.Installed = true
	status.Next = "-"
	return status
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
	if err := runInstallScriptFn(scriptPath); err != nil {
		return err
	}
	post := d.Health()
	if !post.Installed {
		return fmt.Errorf("sandbox doctor install incomplete: missing prerequisites: %s", strings.Join(post.Missing, ","))
	}
	return nil
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

func resolveCurrentUsername() (string, error) {
	current, err := user.Current()
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(current.Username)
	if strings.Contains(name, "\\") {
		parts := strings.Split(name, "\\")
		name = parts[len(parts)-1]
	}
	if strings.Contains(name, "/") {
		parts := strings.Split(name, "/")
		name = parts[len(parts)-1]
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("empty username")
	}
	return name, nil
}

func validateUIDMapHelper(binary string) error {
	path, err := lookupPath(binary)
	if err != nil {
		return err
	}
	info, statErr := statPath(path)
	if statErr != nil {
		return statErr
	}
	if info.Mode()&os.ModeSetuid == 0 {
		return fmt.Errorf("%s missing setuid bit", binary)
	}
	if currentOS() == "linux" {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("%s invalid stat type", binary)
		}
		if stat.Uid != 0 {
			return fmt.Errorf("%s must be root-owned", binary)
		}
	}
	return nil
}

func ensureSubIDConfigured(filePath string, username string) error {
	raw, err := readFile(filePath)
	if err != nil {
		return err
	}
	prefix := strings.TrimSpace(username) + ":"
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, prefix) {
			return nil
		}
	}
	return fmt.Errorf("%s missing entry for %s", filePath, username)
}
