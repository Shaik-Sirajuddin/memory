package sandbox

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeFileInfo struct {
	mode os.FileMode
	sys  any
}

func (f fakeFileInfo) Name() string       { return "fake" }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() any           { return f.sys }

func TestDoctorHealth(t *testing.T) {
	origLookupPath := lookupPath
	origStatPath := statPath
	origReadFile := readFile
	origCurrentOS := currentOS
	origCurrentEUID := currentEUID
	origCurrentUsername := currentUsername
	t.Cleanup(func() {
		lookupPath = origLookupPath
		statPath = origStatPath
		readFile = origReadFile
		currentOS = origCurrentOS
		currentEUID = origCurrentEUID
		currentUsername = origCurrentUsername
	})

	t.Run("LinuxRunscInstalled", func(t *testing.T) {
		t.Log("Running Linux runsc installed health scenario")
		currentOS = func() string { return "linux" }
		currentEUID = func() int { return 0 }
		lookupPath = func(file string) (string, error) {
			require.Equal(t, "runsc", file, "Linux health check should resolve runsc")
			return "/usr/local/bin/runsc", nil
		}

		status := NewDoctor().Health()
		assert.Equal(t, ProvisionerGVisor, status.Provider, "Linux health should target gVisor")
		assert.True(t, status.Installed, "Linux health should report installed when runsc is present")
		assert.Equal(t, "/usr/local/bin/runsc", status.Binary, "Linux health should report runsc path")
		assert.Equal(t, "-", status.Next, "Linux health should report no next action when runtime is ready")
	})

	t.Run("LinuxRunscMissing", func(t *testing.T) {
		t.Log("Running Linux runsc missing health scenario")
		currentOS = func() string { return "linux" }
		currentEUID = func() int { return 1000 }
		lookupPath = func(file string) (string, error) {
			require.Equal(t, "runsc", file, "Linux health check should resolve runsc")
			return "", errors.New("not found")
		}

		status := NewDoctor().Health()
		assert.Equal(t, ProvisionerGVisor, status.Provider, "Linux health should target gVisor")
		assert.False(t, status.Installed, "Linux health should report not installed when runsc is missing")
		assert.Empty(t, status.Binary, "Linux health should not report a binary path when runsc is missing")
		assert.Subset(t, status.Missing, []string{"runsc"}, "Linux health should report runsc in missing prerequisites")
		assert.Equal(t, "run: omni doctor install", status.Next, "Linux health should suggest doctor install when prerequisites are missing")
	})

	t.Run("LinuxRootlessMissingUIDMapHelpers", func(t *testing.T) {
		t.Log("Running Linux rootless health scenario when uidmap helpers are missing")
		currentOS = func() string { return "linux" }
		currentEUID = func() int { return 1000 }
		currentUsername = func() (string, error) { return "siraj", nil }
		lookupPath = func(file string) (string, error) {
			switch file {
			case "runsc":
				return "/usr/local/bin/runsc", nil
			case "newuidmap", "newgidmap":
				return "", errors.New("not found")
			default:
				t.Fatalf("unexpected binary lookup %q", file)
				return "", nil
			}
		}
		readFile = func(path string) ([]byte, error) {
			return []byte("siraj:100000:65536\n"), nil
		}

		status := NewDoctor().Health()
		assert.Equal(t, ProvisionerGVisor, status.Provider, "Linux health should target gVisor")
		assert.False(t, status.Installed, "Linux health should report not installed when rootless uidmap helpers are missing")
		assert.Equal(t, "/usr/local/bin/runsc", status.Binary, "Linux health should still report runsc path when present")
		assert.Subset(t, status.Missing, []string{"newuidmap", "newgidmap"}, "Linux rootless health should report missing uidmap helpers")
		assert.Equal(t, "run: omni doctor install", status.Next, "Linux rootless health should suggest doctor install when uidmap helpers are missing")
	})

	t.Run("LinuxRootlessUIDMapWrongOwnership", func(t *testing.T) {
		t.Log("Running Linux rootless health scenario when uidmap helpers are not root-owned")
		currentOS = func() string { return "linux" }
		currentEUID = func() int { return 1000 }
		currentUsername = func() (string, error) { return "siraj", nil }
		lookupPath = func(file string) (string, error) {
			switch file {
			case "runsc":
				return "/usr/local/bin/runsc", nil
			case "newuidmap":
				return "/usr/bin/newuidmap", nil
			case "newgidmap":
				return "/usr/bin/newgidmap", nil
			default:
				return "", errors.New("not found")
			}
		}
		statPath = func(path string) (os.FileInfo, error) {
			return fakeFileInfo{
				mode: os.ModeSetuid | 0o755,
				sys:  &syscall.Stat_t{Uid: 1000},
			}, nil
		}
		readFile = func(path string) ([]byte, error) {
			return []byte("siraj:100000:65536\n"), nil
		}

		status := NewDoctor().Health()
		assert.False(t, status.Installed, "Linux rootless health should report not installed when uidmap ownership is invalid")
		assert.Subset(t, status.Missing, []string{"newuidmap", "newgidmap"}, "Linux rootless health should report uidmap helpers when ownership is invalid")
	})

	t.Run("LinuxRootlessMissingSubID", func(t *testing.T) {
		t.Log("Running Linux rootless health scenario when subid mappings are missing")
		currentOS = func() string { return "linux" }
		currentEUID = func() int { return 1000 }
		currentUsername = func() (string, error) { return "siraj", nil }
		lookupPath = func(file string) (string, error) {
			switch file {
			case "runsc":
				return "/usr/local/bin/runsc", nil
			case "newuidmap":
				return "/usr/bin/newuidmap", nil
			case "newgidmap":
				return "/usr/bin/newgidmap", nil
			default:
				return "", errors.New("not found")
			}
		}
		statPath = func(path string) (os.FileInfo, error) {
			return fakeFileInfo{
				mode: os.ModeSetuid | 0o755,
				sys:  &syscall.Stat_t{Uid: 0},
			}, nil
		}
		readFile = func(path string) ([]byte, error) {
			return []byte("other:100000:65536\n"), nil
		}

		status := NewDoctor().Health()
		assert.False(t, status.Installed, "Linux rootless health should report not installed when subid mappings are missing")
		assert.Subset(t, status.Missing, []string{"subuid", "subgid"}, "Linux rootless health should report missing subuid and subgid mappings")
	})
}

func TestDoctorInstall(t *testing.T) {
	origLookupPath := lookupPath
	origStatPath := statPath
	origReadFile := readFile
	origCurrentOS := currentOS
	origCurrentEUID := currentEUID
	origCurrentUsername := currentUsername
	origResolveScriptPath := resolveScriptPath
	origRunInstallScriptFn := runInstallScriptFn
	t.Cleanup(func() {
		lookupPath = origLookupPath
		statPath = origStatPath
		readFile = origReadFile
		currentOS = origCurrentOS
		currentEUID = origCurrentEUID
		currentUsername = origCurrentUsername
		resolveScriptPath = origResolveScriptPath
		runInstallScriptFn = origRunInstallScriptFn
	})

	t.Run("LinuxMissingRunscRunsInstaller", func(t *testing.T) {
		t.Log("Running Linux install scenario when runsc is missing")
		currentOS = func() string { return "linux" }
		currentEUID = func() int { return 0 }
		runscInstalled := false
		lookupPath = func(file string) (string, error) {
			if file != "runsc" {
				return "", errors.New("not found")
			}
			if !runscInstalled {
				return "", errors.New("not found")
			}
			return "/usr/local/bin/runsc", nil
		}
		resolveScriptPath = func() (string, error) { return "/tmp/install-sandbox-runtime.sh", nil }

		called := false
		runInstallScriptFn = func(path string) error {
			called = true
			assert.Equal(t, "/tmp/install-sandbox-runtime.sh", path, "Install should execute resolved script path")
			runscInstalled = true
			return nil
		}

		err := NewDoctor().Install()
		require.NoError(t, err, "Install should succeed when installer script succeeds")
		assert.True(t, called, "Install should invoke installer when runsc is missing")
	})

	t.Run("LinuxInstalledSkipsInstaller", func(t *testing.T) {
		t.Log("Running Linux install skip scenario when runsc already exists")
		currentOS = func() string { return "linux" }
		currentEUID = func() int { return 0 }
		lookupPath = func(file string) (string, error) { return "/usr/local/bin/runsc", nil }
		resolveScriptPath = func() (string, error) {
			t.Fatal("Install should not resolve script when runsc already exists")
			return "", nil
		}
		runInstallScriptFn = func(path string) error {
			t.Fatal("Install should not run script when runsc already exists")
			return nil
		}

		err := NewDoctor().Install()
		require.NoError(t, err, "Install should be a no-op when runsc is already installed")
	})

	t.Run("LinuxRootlessMissingUIDMapRunsInstaller", func(t *testing.T) {
		t.Log("Running Linux install scenario when runsc exists but uidmap helpers are missing")
		currentOS = func() string { return "linux" }
		currentEUID = func() int { return 1000 }
		currentUsername = func() (string, error) { return "siraj", nil }
		uidmapReady := false
		lookupPath = func(file string) (string, error) {
			switch file {
			case "runsc":
				return "/usr/local/bin/runsc", nil
			case "newuidmap":
				if !uidmapReady {
					return "", errors.New("not found")
				}
				return "/usr/bin/newuidmap", nil
			case "newgidmap":
				if !uidmapReady {
					return "", errors.New("not found")
				}
				return "/usr/bin/newgidmap", nil
			default:
				return "", errors.New("not found")
			}
		}
		statPath = func(path string) (os.FileInfo, error) {
			return fakeFileInfo{mode: os.ModeSetuid | 0o755, sys: &syscall.Stat_t{Uid: 0}}, nil
		}
		readFile = func(path string) ([]byte, error) {
			return []byte("siraj:100000:65536\n"), nil
		}
		resolveScriptPath = func() (string, error) { return "/tmp/install-sandbox-runtime.sh", nil }

		called := false
		runInstallScriptFn = func(path string) error {
			called = true
			assert.Equal(t, "/tmp/install-sandbox-runtime.sh", path, "Install should execute resolved script path for missing uidmap helpers")
			uidmapReady = true
			return nil
		}

		err := NewDoctor().Install()
		require.NoError(t, err, "Install should run when rootless uidmap helpers are missing")
		assert.True(t, called, "Install should invoke installer when rootless uidmap helpers are missing")
	})

	t.Run("LinuxRootlessMissingSubIDRunsInstaller", func(t *testing.T) {
		t.Log("Running Linux install scenario when subid mappings are missing")
		currentOS = func() string { return "linux" }
		currentEUID = func() int { return 1000 }
		currentUsername = func() (string, error) { return "siraj", nil }
		subIDReady := false
		lookupPath = func(file string) (string, error) {
			switch file {
			case "runsc":
				return "/usr/local/bin/runsc", nil
			case "newuidmap":
				return "/usr/bin/newuidmap", nil
			case "newgidmap":
				return "/usr/bin/newgidmap", nil
			default:
				return "", errors.New("not found")
			}
		}
		statPath = func(path string) (os.FileInfo, error) {
			return fakeFileInfo{mode: os.ModeSetuid | 0o755, sys: &syscall.Stat_t{Uid: 0}}, nil
		}
		readFile = func(path string) ([]byte, error) {
			if subIDReady {
				return []byte("siraj:100000:65536\n"), nil
			}
			return []byte("other:100000:65536\n"), nil
		}
		resolveScriptPath = func() (string, error) { return "/tmp/install-sandbox-runtime.sh", nil }

		called := false
		runInstallScriptFn = func(path string) error {
			called = true
			assert.Equal(t, "/tmp/install-sandbox-runtime.sh", path, "Install should execute resolved script path for missing subid mappings")
			subIDReady = true
			return nil
		}

		err := NewDoctor().Install()
		require.NoError(t, err, "Install should run when rootless subid mappings are missing")
		assert.True(t, called, "Install should invoke installer when rootless subid mappings are missing")
	})

	t.Run("DarwinSkipsGVisorInstaller", func(t *testing.T) {
		t.Log("Running Darwin install skip scenario")
		currentOS = func() string { return "darwin" }
		currentEUID = func() int { return 1000 }
		lookupPath = func(file string) (string, error) { return "/usr/bin/sandbox-exec", nil }
		resolveScriptPath = func() (string, error) {
			t.Fatal("Install should not resolve gVisor script on darwin")
			return "", nil
		}
		runInstallScriptFn = func(path string) error {
			t.Fatal("Install should not run gVisor installer on darwin")
			return nil
		}

		err := NewDoctor().Install()
		require.NoError(t, err, "Install should be a no-op on darwin")
	})
}
