package defaults

import (
	"os/exec"
	"path/filepath"

	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
	sandbox "github.com/Shaik-Sirajuddin/memory/sandbox"
)

// Binary candidate lists mirror the respective connector configs so that
// this package does not need to import the connector packages (some of which
// have pre-existing build issues in their generated files).
var claudeBinaries = []string{
	"claude",
	"/usr/local/bin/claude",
	"/opt/homebrew/bin/claude",
}

var geminiBinaries = []string{
	"gemini",
	"/usr/local/bin/gemini",
	"/opt/homebrew/bin/gemini",
}

// BinaryDirs returns the unique parent directories that contain the resolved
// binary for the given provider.  Candidates are resolved via exec.LookPath;
// absolute hard-coded paths are included directly when look-up fails.
func BinaryDirs(provider codeagent.Provider) []string {
	var candidates []string
	switch provider {
	case "claude":
		candidates = claudeBinaries
	case "gemini":
		candidates = geminiBinaries
	default:
		// codex and unknown providers: resolve the provider name from PATH.
		if p, err := exec.LookPath(string(provider)); err == nil {
			return []string{filepath.Dir(p)}
		}
		return nil
	}

	seen := map[string]struct{}{}
	var dirs []string
	for _, bin := range candidates {
		resolved, err := exec.LookPath(bin)
		if err != nil {
			// Absolute fallback paths are included even when not present.
			if filepath.IsAbs(bin) {
				d := filepath.Dir(bin)
				if _, ok := seen[d]; !ok {
					seen[d] = struct{}{}
					dirs = append(dirs, d)
				}
			}
			continue
		}
		d := filepath.Dir(resolved)
		if _, ok := seen[d]; !ok {
			seen[d] = struct{}{}
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// SandboxConfig returns the default sandbox.Config for the given provider.
// The workspace policy grants permissive-read access to the workspace.
// The agent policy adds read access to the directories containing the
// provider's binary so the agent process can locate its own executable.
func SandboxConfig(provider codeagent.Provider) *sandbox.Config {
	binDirs := BinaryDirs(provider)
	return &sandbox.Config{
		WorkSpacePolicy: &sandbox.Policy{
			FSPolicy: sandbox.FSPolicy(sandbox.PermissiveRead),
		},
		AgentPolicy: &sandbox.Policy{
			FSPolicy: sandbox.FSPolicy(sandbox.PermissiveRead),
			Config: sandbox.MountConfig{
				AccessDirs: binDirs,
			},
		},
	}
}
