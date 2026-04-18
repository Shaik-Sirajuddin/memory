package gvisor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/memory/sandbox/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvisionerExecCommand(t *testing.T) {
	t.Run("RunscExecShape", func(t *testing.T) {
		p, err := New(nil, provider.ProvisionerOptions{
			Executable: "custom-runsc",
			GlobalArgs: []string{"--root=/run/user/1000/runsc"},
			ExtraArgs:  []string{"--stdio=none"},
			Store:      newMemoryStore("gvisor"),
		})
		require.NoError(t, err, "Creating the gVisor provisioner should not return an error")

		executable, args, err := p.ExecCommand(&provider.Sandbox{
			State: &provider.State{PID: "sandbox-1"},
		}, "/bin/sh", []string{"-lc", "pwd"})
		require.NoError(t, err, "Building a gVisor exec command should not return an error")
		assert.Equal(t, "custom-runsc", executable, "gVisor executable should respect the configured override")
		assert.Equal(t, []string{
			"--root=/run/user/1000/runsc",
			"exec",
			"sandbox-1",
			"/bin/sh",
			"-lc",
			"pwd",
			"--stdio=none",
		}, args, "gVisor exec args should match the expected runsc invocation")
	})

	t.Run("Validation", func(t *testing.T) {
		p, err := New(nil, provider.ProvisionerOptions{Store: newMemoryStore("gvisor")})
		require.NoError(t, err, "Creating the gVisor provisioner should not return an error")

		_, _, err = p.ExecCommand(&provider.Sandbox{}, "/bin/sh", nil)
		require.Error(t, err, "Building a gVisor command without a pid should return an error")

		_, _, err = p.ExecCommand(&provider.Sandbox{State: &provider.State{PID: "sandbox-1"}}, "", nil)
		require.Error(t, err, "Building a gVisor command without a command should return an error")
	})
}

func TestParseRuntimeList(t *testing.T) {
	t.Run("HeaderAndRows", func(t *testing.T) {
		out := `ID               PID         STATUS      BUNDLE      CREATED                          OWNER
sandbox-1        1234        running     /tmp/b1     2026-04-18T12:00:00.000000+00:00  user
sandbox-2        0           stopped     /tmp/b2     2026-04-18T12:01:00.000000+00:00  user`
		items := parseRuntimeList(out)
		require.Len(t, items, 2, "Parser should return each non-header runtime row")
		assert.Equal(t, "sandbox-1", items[0].ID, "Parser should capture runtime id from the first column")
		assert.Equal(t, "1234", items[0].PID, "Parser should capture runtime pid from the second column")
		assert.Equal(t, "running", items[0].Status, "Parser should capture normalized runtime status from the third column")
		assert.Equal(t, "sandbox-2", items[1].ID, "Parser should capture second runtime id")
		assert.Equal(t, "stopped", items[1].Status, "Parser should capture stopped runtime status")
	})

	t.Run("EmptyInput", func(t *testing.T) {
		items := parseRuntimeList("")
		assert.Empty(t, items, "Parser should return no runtimes for empty output")
	})
}

func TestRuntimeActive(t *testing.T) {
	assert.True(t, runtimeActive("running"), "Running status should be considered active")
	assert.True(t, runtimeActive("created"), "Created status should be considered active")
	assert.True(t, runtimeActive("paused"), "Paused status should be considered active")
	assert.False(t, runtimeActive("stopped"), "Stopped status should not be considered active")
	assert.False(t, runtimeActive(""), "Empty status should not be considered active")
}

func TestProvisionerDirectoryOps(t *testing.T) {
	t.Run("CreateAndListRelativeDirs", func(t *testing.T) {
		workDir := t.TempDir()
		p, err := New(nil, provider.ProvisionerOptions{
			WorkDir: workDir,
			Store:   newMemoryStore("gvisor"),
		})
		require.NoError(t, err, "Creating gVisor provisioner should not return an error")

		require.NoError(t, p.CreateDir("alpha"), "CreateDir should create relative directory under WorkDir")
		require.NoError(t, p.CreateDir("beta"), "CreateDir should create a second directory under WorkDir")
		require.NoError(t, os.WriteFile(filepath.Join(workDir, "note.txt"), []byte("x"), 0o644), "Test setup should create a non-directory file")

		dirs, err := p.ListDirs(".")
		require.NoError(t, err, "ListDirs should list directories under relative path")
		assert.Subset(t, dirs, []string{"alpha", "beta"}, "ListDirs should include created directories")
		assert.NotContains(t, dirs, "note.txt", "ListDirs should not include files")
	})
}

func TestUpdateSandbox(t *testing.T) {
	t.Run("Validation", func(t *testing.T) {
		p, err := New(nil, provider.ProvisionerOptions{WorkDir: t.TempDir()})
		require.NoError(t, err, "Creating gVisor provisioner should not return an error")

		_, err = p.UpdateSandbox(nil)
		require.Error(t, err, "UpdateSandbox should return an error when params are missing")

		_, err = p.UpdateSandbox(&provider.UpdateSandboxParams{})
		require.Error(t, err, "UpdateSandbox should return an error when sandbox id is missing")
	})

	t.Run("UpdateExistingSandbox", func(t *testing.T) {
		p, err := New(nil, provider.ProvisionerOptions{WorkDir: t.TempDir()})
		require.NoError(t, err, "Creating gVisor provisioner should not return an error")

		_, err = p.state.Create(provider.ProvisionerGVisor, provider.CreateSandboxParams{
			ID: "sandbox-update-1",
			Config: &provider.Config{
				AgentPolicy: &provider.Policy{FSPolicy: provider.FSPolicy(provider.Inherit)},
			},
		}, nil)
		require.NoError(t, err, "Test setup should create sandbox metadata in provisioner state")

		rt, err := p.UpdateSandbox(&provider.UpdateSandboxParams{
			ID: "sandbox-update-1",
			Config: &provider.Config{
				AgentPolicy: &provider.Policy{FSPolicy: provider.FSPolicy(provider.PermissiveRead)},
			},
		})
		require.NoError(t, err, "UpdateSandbox should update config for existing sandbox id")
		require.NotNil(t, rt, "UpdateSandbox should return runtime for existing sandbox")
		require.NotNil(t, rt.Sandbox().Config, "Updated runtime should still include config")
		assert.Equal(t, provider.FSPolicy(provider.PermissiveRead), rt.Sandbox().Config.AgentPolicy.FSPolicy, "Updated runtime config should reflect requested policy")
	})
}

func TestResolveBundlePath(t *testing.T) {
	t.Run("UsesConfiguredBundle", func(t *testing.T) {
		workDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(workDir, "config.json"), []byte("{}"), 0o644), "Test setup should write config.json in WorkDir")
		p, err := New(nil, provider.ProvisionerOptions{WorkDir: workDir})
		require.NoError(t, err, "Creating gVisor provisioner should not return an error")

		bundle, err := p.resolveBundlePath("sandbox-bundle-1")
		require.NoError(t, err, "resolveBundlePath should return configured WorkDir bundle when config exists")
		assert.Equal(t, filepath.Clean(workDir), bundle, "resolveBundlePath should use configured WorkDir bundle path")
	})

	t.Run("CreatesDefaultBundleWhenWorkDirIsNotBundle", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", t.TempDir())
		p, err := New(nil, provider.ProvisionerOptions{WorkDir: t.TempDir()})
		require.NoError(t, err, "Creating gVisor provisioner should not return an error")

		bundle, err := p.resolveBundlePath("sandbox-bundle-2")
		require.NoError(t, err, "resolveBundlePath should create default bundle when WorkDir is not an OCI bundle")
		require.FileExists(t, filepath.Join(bundle, "config.json"), "resolveBundlePath should create config.json in default bundle")

		raw, err := os.ReadFile(filepath.Join(bundle, "config.json"))
		require.NoError(t, err, "Test should read generated default config.json")
		var spec ociSpec
		require.NoError(t, json.Unmarshal(raw, &spec), "Generated config.json should be valid OCI JSON")
		assert.Equal(t, "1.0.2", spec.OCIVersion, "Generated default OCI spec should set ociVersion")
		assert.Equal(t, []string{"/bin/sh", "-c", "sleep infinity"}, spec.Process.Args, "Generated default OCI spec should keep sandbox process alive")
	})
}

type memoryStore struct {
	info provider.Info
}

func newMemoryStore(application string) *memoryStore {
	return &memoryStore{info: provider.Info{Application: application}}
}

func (s *memoryStore) Info() provider.Info            { return s.info }
func (s *memoryStore) Create(*provider.Sandbox) error { return nil }
func (s *memoryStore) Update(*provider.Sandbox) error { return nil }
func (s *memoryStore) Get(*provider.GetSandboxParams) (*provider.Sandbox, error) {
	return nil, provider.NoProcessFound
}
func (s *memoryStore) List() ([]*provider.Sandbox, error) { return nil, nil }
