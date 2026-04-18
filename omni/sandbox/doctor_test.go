package sandbox

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoctorHealth(t *testing.T) {
	origLookupPath := lookupPath
	origCurrentOS := currentOS
	t.Cleanup(func() {
		lookupPath = origLookupPath
		currentOS = origCurrentOS
	})

	t.Run("LinuxRunscInstalled", func(t *testing.T) {
		t.Log("Running Linux runsc installed health scenario")
		currentOS = func() string { return "linux" }
		lookupPath = func(file string) (string, error) {
			require.Equal(t, "runsc", file, "Linux health check should resolve runsc")
			return "/usr/local/bin/runsc", nil
		}

		status := NewDoctor().Health()
		assert.Equal(t, ProvisionerGVisor, status.Provider, "Linux health should target gVisor")
		assert.True(t, status.Installed, "Linux health should report installed when runsc is present")
		assert.Equal(t, "/usr/local/bin/runsc", status.Binary, "Linux health should report runsc path")
	})

	t.Run("LinuxRunscMissing", func(t *testing.T) {
		t.Log("Running Linux runsc missing health scenario")
		currentOS = func() string { return "linux" }
		lookupPath = func(file string) (string, error) {
			require.Equal(t, "runsc", file, "Linux health check should resolve runsc")
			return "", errors.New("not found")
		}

		status := NewDoctor().Health()
		assert.Equal(t, ProvisionerGVisor, status.Provider, "Linux health should target gVisor")
		assert.False(t, status.Installed, "Linux health should report not installed when runsc is missing")
		assert.Empty(t, status.Binary, "Linux health should not report a binary path when runsc is missing")
	})
}

func TestDoctorInstall(t *testing.T) {
	origLookupPath := lookupPath
	origCurrentOS := currentOS
	origResolveScriptPath := resolveScriptPath
	origRunInstallScriptFn := runInstallScriptFn
	t.Cleanup(func() {
		lookupPath = origLookupPath
		currentOS = origCurrentOS
		resolveScriptPath = origResolveScriptPath
		runInstallScriptFn = origRunInstallScriptFn
	})

	t.Run("LinuxMissingRunscRunsInstaller", func(t *testing.T) {
		t.Log("Running Linux install scenario when runsc is missing")
		currentOS = func() string { return "linux" }
		lookupPath = func(file string) (string, error) { return "", errors.New("not found") }
		resolveScriptPath = func() (string, error) { return "/tmp/install-sandbox-runtime.sh", nil }

		called := false
		runInstallScriptFn = func(path string) error {
			called = true
			assert.Equal(t, "/tmp/install-sandbox-runtime.sh", path, "Install should execute resolved script path")
			return nil
		}

		err := NewDoctor().Install()
		require.NoError(t, err, "Install should succeed when installer script succeeds")
		assert.True(t, called, "Install should invoke installer when runsc is missing")
	})

	t.Run("LinuxInstalledSkipsInstaller", func(t *testing.T) {
		t.Log("Running Linux install skip scenario when runsc already exists")
		currentOS = func() string { return "linux" }
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

	t.Run("DarwinSkipsGVisorInstaller", func(t *testing.T) {
		t.Log("Running Darwin install skip scenario")
		currentOS = func() string { return "darwin" }
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
