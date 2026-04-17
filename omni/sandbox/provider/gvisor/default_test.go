package gvisor

import (
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
