package seatbelt

import (
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
