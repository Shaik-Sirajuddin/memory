package sandbox_test

import (
	"testing"

	"github.com/Shaik-Sirajuddin/memory/sandbox/provider"
	"github.com/stretchr/testify/assert"
)

func TestProviderTypes(t *testing.T) {
	t.Run("DefaultWorkspaceDir", func(t *testing.T) {
		assert.Equal(t, provider.WorkspaceDir("_default"), provider.Default, "Default workspace dir should match the package default")
	})

	t.Run("ProvisionerKinds", func(t *testing.T) {
		assert.Equal(t, provider.ProvisionerKind("gvisor"), provider.ProvisionerGVisor, "gVisor provisioner kind should match the expected value")
		assert.Equal(t, provider.ProvisionerKind("bubblewrap"), provider.ProvisionerBubblewrap, "bubblewrap provisioner kind should match the expected value")
		assert.Equal(t, provider.ProvisionerKind("seatbelt"), provider.ProvisionerSeatbelt, "seatbelt provisioner kind should match the expected value")
	})

	t.Run("SandboxComposition", func(t *testing.T) {
		cfg := &provider.Config{
			AgentPolicy: &provider.Policy{
				Dir:      provider.Default,
				FSPolicy: provider.FSPolicy(provider.Inherit),
				Config: provider.MountConfig{
					AccessDirs:  []string{"/tmp/cache"},
					BlockedDirs: []string{"/secret"},
				},
			},
		}
		sbx := &provider.Sandbox{
			Config: cfg,
			State: &provider.State{
				PID:    "sandbox-1",
				Active: true,
			},
			Data: &provider.Data{
				ID:          "sandbox-1",
				Application: "gvisor",
				CreatedAt:   "2026-04-09T00:00:00Z",
			},
		}

		assert.Equal(t, provider.FSPolicy(provider.Inherit), sbx.Config.AgentPolicy.FSPolicy, "Sandbox config should retain the configured fs policy")
		assert.Subset(t, sbx.Config.AgentPolicy.Config.AccessDirs, []string{"/tmp/cache"}, "Sandbox config should retain allowed directories")
		assert.Subset(t, sbx.Config.AgentPolicy.Config.BlockedDirs, []string{"/secret"}, "Sandbox config should retain blocked directories")
		assert.Equal(t, "sandbox-1", sbx.State.PID, "Sandbox state should retain the configured pid")
		assert.True(t, sbx.State.Active, "Sandbox state should retain the active flag")
		assert.Equal(t, "gvisor", sbx.Data.Application, "Sandbox data should retain the application name")
	})
}
