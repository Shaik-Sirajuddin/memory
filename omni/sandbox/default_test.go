package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfigParserLoadValidateSave(t *testing.T) {
	t.Run("RoundTripJSON", func(t *testing.T) {
		dir := t.TempDir()
		inPath := filepath.Join(dir, "sandbox.json")
		raw := []byte(`{
  "workspace_policy": {
    "dir": "/workspace",
    "fs_policy": "permissive-read",
    "config": {
      "access_dirs": ["/workspace", "/usr/bin"],
      "blocked_dirs": ["/workspace/.git"]
    }
  },
  "agent_policy": {
    "dir": "_default",
    "fs_policy": "inherit",
    "config": {
      "access_dirs": ["/tmp"],
      "blocked_dirs": ["/root"]
    }
  }
}`)
		require.NoError(t, os.WriteFile(inPath, raw, 0o644), "Test setup should write input config file")

		parser := NewConfigParser()
		cfg, err := parser.Load(inPath)
		require.NoError(t, err, "Load should parse JSON config via koanf")
		require.NotNil(t, cfg.WorkSpacePolicy, "Load should hydrate workspace policy")
		require.NotNil(t, cfg.AgentPolicy, "Load should hydrate agent policy")
		assert.Equal(t, WorkspaceDir("/workspace"), cfg.WorkSpacePolicy.Dir, "Load should map workspace policy dir")
		assert.Equal(t, FSPolicy(PermissiveRead), cfg.WorkSpacePolicy.FSPolicy, "Load should map workspace fs policy")
		assert.Subset(t, cfg.WorkSpacePolicy.Config.AccessDirs, []string{"/workspace", "/usr/bin"}, "Load should map access dirs")
		assert.Subset(t, cfg.AgentPolicy.Config.BlockedDirs, []string{"/root"}, "Load should map blocked dirs")

		require.NoError(t, Validate(cfg), "Validate wrapper should accept valid config")

		outPath := filepath.Join(dir, "saved", "sandbox.json")
		require.NoError(t, Save(cfg, outPath), "Save wrapper should persist validated config")

		saved, err := Load(outPath)
		require.NoError(t, err, "Load wrapper should read config saved by Save")
		assert.Equal(t, cfg.AgentPolicy.FSPolicy, saved.AgentPolicy.FSPolicy, "Round-trip should preserve policy fields")
	})
}

func TestDefaultConfigParserValidate(t *testing.T) {
	t.Run("RejectsInvalidPolicy", func(t *testing.T) {
		parser := NewConfigParser()
		err := parser.Validate(&Config{
			AgentPolicy: &Policy{FSPolicy: FSPolicy("unsupported")},
		})
		require.Error(t, err, "Validate should reject unsupported fs policies")
		assert.Contains(t, err.Error(), "invalid agent_policy fs_policy", "Validate error should identify invalid field")
	})

	t.Run("RejectsEmptyDirEntries", func(t *testing.T) {
		err := Validate(&Config{
			WorkSpacePolicy: &Policy{
				FSPolicy: FSPolicy(PermissiveRead),
				Config: MountConfig{
					AccessDirs: []string{"/workspace", ""},
				},
			},
		})
		require.Error(t, err, "Validate should reject empty access_dirs entries")
		assert.Contains(t, err.Error(), "access_dirs", "Validate error should mention access_dirs")
	})
}
