package seatbelt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/memory/sandbox/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvisionerBuildCommand(t *testing.T) {
	t.Run("ReadOnlyProfile", func(t *testing.T) {
		p, err := New(&provider.Sandbox{
			Config: &provider.Config{
				AgentPolicy: &provider.Policy{
					FSPolicy: provider.FSPolicy(provider.PermissiveRead),
					Config: provider.MountConfig{
						AccessDirs:  []string{"/Users/demo/Documents"},
						BlockedDirs: []string{"/Users/demo/Documents/private"},
					},
				},
			},
		}, provider.ProvisionerOptions{
			WorkDir: "/workspace",
			Store:   newMemoryStore("seatbelt"),
		})
		require.NoError(t, err, "Creating the seatbelt provisioner should not return an error")

		executable, args, err := p.BuildCommand(&provider.Sandbox{
			Config: &provider.Config{
				AgentPolicy: &provider.Policy{
					FSPolicy: provider.FSPolicy(provider.PermissiveRead),
					Config: provider.MountConfig{
						AccessDirs:  []string{"/Users/demo/Documents"},
						BlockedDirs: []string{"/Users/demo/Documents/private"},
					},
				},
			},
		}, "/bin/echo", []string{"hello"})
		require.NoError(t, err, "Building a seatbelt command should not return an error")
		require.Len(t, args, 4, "Seatbelt command should include profile, executable, and args")
		assert.Equal(t, "sandbox-exec", executable, "Seatbelt executable should default to sandbox-exec")
		assert.Contains(t, args[1], `(allow file-read* (subpath "/workspace"))`, "Seatbelt profile should allow workspace reads")
		assert.Contains(t, args[1], `(allow file-read* (subpath "/Users/demo/Documents"))`, "Seatbelt profile should allow configured access directories")
		assert.Contains(t, args[1], `(deny file-read* file-write* (subpath "/Users/demo/Documents/private"))`, "Seatbelt profile should deny blocked directories")
	})

	t.Run("WriteEnabledProfile", func(t *testing.T) {
		p, err := New(&provider.Sandbox{
			Config: &provider.Config{
				AgentPolicy: &provider.Policy{
					FSPolicy: provider.FSPolicy(provider.Inherit),
				},
			},
		}, provider.ProvisionerOptions{
			WorkDir: "/workspace",
			Store:   newMemoryStore("seatbelt"),
		})
		require.NoError(t, err, "Creating the seatbelt provisioner should not return an error")

		_, args, err := p.BuildCommand(&provider.Sandbox{
			Config: &provider.Config{
				AgentPolicy: &provider.Policy{
					FSPolicy: provider.FSPolicy(provider.Inherit),
				},
			},
		}, "/usr/bin/true", nil)
		require.NoError(t, err, "Building a seatbelt command should not return an error")
		assert.Contains(t, args[1], `(allow file-write* (subpath "/workspace"))`, "Seatbelt profile should allow workspace writes when write access is enabled")
	})
}

func TestProvisionerDirectoryOps(t *testing.T) {
	t.Run("CreateAndListRelativeDirs", func(t *testing.T) {
		workDir := t.TempDir()
		p, err := New(nil, provider.ProvisionerOptions{
			WorkDir: workDir,
			Store:   newMemoryStore("seatbelt"),
		})
		require.NoError(t, err, "Creating seatbelt provisioner should not return an error")

		require.NoError(t, p.CreateDir("one"), "CreateDir should create relative directory under WorkDir")
		require.NoError(t, p.CreateDir("two"), "CreateDir should create a second directory under WorkDir")
		require.NoError(t, os.WriteFile(filepath.Join(workDir, "file.txt"), []byte("x"), 0o644), "Test setup should create a non-directory file")

		dirs, err := p.ListDirs(".")
		require.NoError(t, err, "ListDirs should list directories under relative path")
		assert.Subset(t, dirs, []string{"one", "two"}, "ListDirs should include created directories")
		assert.NotContains(t, dirs, "file.txt", "ListDirs should not include files")
	})
}

func TestUpdateSandbox(t *testing.T) {
	t.Run("Validation", func(t *testing.T) {
		p, err := New(nil, provider.ProvisionerOptions{WorkDir: t.TempDir()})
		require.NoError(t, err, "Creating seatbelt provisioner should not return an error")

		_, err = p.UpdateSandbox(nil)
		require.Error(t, err, "UpdateSandbox should return an error when params are missing")

		_, err = p.UpdateSandbox(&provider.UpdateSandboxParams{})
		require.Error(t, err, "UpdateSandbox should return an error when sandbox id is missing")
	})

	t.Run("UpdateExistingSandbox", func(t *testing.T) {
		p, err := New(nil, provider.ProvisionerOptions{WorkDir: t.TempDir()})
		require.NoError(t, err, "Creating seatbelt provisioner should not return an error")

		rt, err := p.Create(provider.CreateSandboxParams{
			ID: "seatbelt-update-1",
			Config: &provider.Config{
				AgentPolicy: &provider.Policy{FSPolicy: provider.FSPolicy(provider.Inherit)},
			},
		})
		require.NoError(t, err, "Create should initialize sandbox runtime for update test")
		require.NotNil(t, rt, "Create should return runtime instance")

		updated, err := p.UpdateSandbox(&provider.UpdateSandboxParams{
			ID: "seatbelt-update-1",
			Config: &provider.Config{
				AgentPolicy: &provider.Policy{FSPolicy: provider.FSPolicy(provider.PermissiveRead)},
			},
		})
		require.NoError(t, err, "UpdateSandbox should update config for existing sandbox id")
		require.NotNil(t, updated, "UpdateSandbox should return runtime for existing sandbox")
		require.NotNil(t, updated.Sandbox().Config, "Updated runtime should still include config")
		assert.Equal(t, provider.FSPolicy(provider.PermissiveRead), updated.Sandbox().Config.AgentPolicy.FSPolicy, "Updated runtime config should reflect requested policy")
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
