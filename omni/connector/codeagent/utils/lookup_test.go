package codeagentutils

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeNVMBin creates a fake binary at <nvmDir>/versions/node/<nodeVer>/bin/<name>
// and returns its full path.
func makeNVMBin(t *testing.T, nvmDir, nodeVer, name string, modTime time.Time) string {
	t.Helper()
	binDir := filepath.Join(nvmDir, "versions", "node", nodeVer, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	path := filepath.Join(binDir, name)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\necho fake"), 0o755))
	require.NoError(t, os.Chtimes(path, modTime, modTime))
	return path
}

func TestLookPathNVM_FoundViaNVMDirEnv(t *testing.T) {
	nvmDir := t.TempDir()
	t.Setenv("NVM_DIR", nvmDir)

	want := makeNVMBin(t, nvmDir, "v20.0.0", "claude", time.Now())

	got, err := LookPathNVM("claude")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestLookPathNVM_FallsBackToHomeNVM(t *testing.T) {
	// Unset NVM_DIR so the function resolves via ~/.nvm.
	t.Setenv("NVM_DIR", "")

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	nvmDir := filepath.Join(home, ".nvm")

	// Only run if ~/.nvm exists — avoids creating dirs in the user's home.
	if _, err := os.Stat(nvmDir); os.IsNotExist(err) {
		t.Skip("~/.nvm not present on this machine")
	}

	// The real claude binary should be discoverable.
	got, err := LookPathNVM("claude")
	require.NoError(t, err)
	assert.Contains(t, got, ".nvm")
	assert.Contains(t, got, "claude")
}

func TestLookPathNVM_PicksMostRecentVersion(t *testing.T) {
	nvmDir := t.TempDir()
	t.Setenv("NVM_DIR", nvmDir)

	now := time.Now()
	makeNVMBin(t, nvmDir, "v18.0.0", "claude", now.Add(-2*time.Hour))
	want := makeNVMBin(t, nvmDir, "v20.0.0", "claude", now.Add(-1*time.Hour))
	makeNVMBin(t, nvmDir, "v19.0.0", "claude", now.Add(-3*time.Hour))

	got, err := LookPathNVM("claude")
	require.NoError(t, err)
	assert.Equal(t, want, got, "should return the most recently modified binary")
}

func TestLookPathNVM_NotFound(t *testing.T) {
	nvmDir := t.TempDir()
	t.Setenv("NVM_DIR", nvmDir)

	_, err := LookPathNVM("claude")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "claude")
}

func TestLookPathNVM_SkipsDirectories(t *testing.T) {
	nvmDir := t.TempDir()
	t.Setenv("NVM_DIR", nvmDir)

	// Place a directory where the binary should be — must not be returned.
	dirPath := filepath.Join(nvmDir, "versions", "node", "v20.0.0", "bin", "claude")
	require.NoError(t, os.MkdirAll(dirPath, 0o755))

	_, err := LookPathNVM("claude")
	require.Error(t, err, "a directory at the binary path should not be returned")
}
