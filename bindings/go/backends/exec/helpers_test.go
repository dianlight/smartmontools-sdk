// SPDX-License-Identifier: GPL-2.0-or-later

package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSmartctlSearchPaths_NotEmpty(t *testing.T) {
	assert.NotEmpty(t, smartctlSearchPaths, "smartctlSearchPaths must contain at least one entry")
}

func TestSmartctlSearchPaths_ContainsPlatformPaths(t *testing.T) {
	expected := []string{
		"/usr/sbin/smartctl",
		"/usr/local/sbin/smartctl",
		"/opt/homebrew/bin/smartctl",
		"/usr/local/bin/smartctl",
		"/usr/syno/bin/smartctl",
		"/run/current-system/sw/sbin/smartctl",
	}
	for _, want := range expected {
		assert.Contains(t, smartctlSearchPaths, want,
			"smartctlSearchPaths should include %q", want)
	}
}

func TestResolveSmartctlPath_SearchPathFallback(t *testing.T) {
	tmpDir := t.TempDir()
	fakeSmartctl := filepath.Join(tmpDir, "smartctl")
	err := os.WriteFile(fakeSmartctl, []byte("#!/bin/sh\necho fake"), 0o755)
	require.NoError(t, err)

	orig := smartctlSearchPaths
	t.Cleanup(func() { smartctlSearchPaths = orig })
	smartctlSearchPaths = append([]string{fakeSmartctl}, orig...)

	t.Setenv("PATH", "")

	got, err := resolveSmartctlPath()
	require.NoError(t, err)
	assert.Equal(t, fakeSmartctl, got)
}

func TestResolveSmartctlPath_NotFound(t *testing.T) {
	orig := smartctlSearchPaths
	t.Cleanup(func() { smartctlSearchPaths = orig })
	smartctlSearchPaths = []string{}

	t.Setenv("PATH", "")

	_, err := resolveSmartctlPath()
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "smartctl not found"),
		"error should mention 'smartctl not found', got: %v", err)
}

func TestResolveSmartctlPath_SkipsNonExecutable(t *testing.T) {
	tmpDir := t.TempDir()

	nonExec := filepath.Join(tmpDir, "smartctl-noexec")
	require.NoError(t, os.WriteFile(nonExec, []byte("#!/bin/sh"), 0o644))

	execFile := filepath.Join(tmpDir, "smartctl-exec")
	require.NoError(t, os.WriteFile(execFile, []byte("#!/bin/sh"), 0o755))

	orig := smartctlSearchPaths
	t.Cleanup(func() { smartctlSearchPaths = orig })
	smartctlSearchPaths = []string{nonExec, execFile}

	t.Setenv("PATH", "")

	got, err := resolveSmartctlPath()
	require.NoError(t, err)
	assert.Equal(t, execFile, got, "non-executable candidate should be skipped")
}

func TestResolveSmartctlPath_SkipsDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	dirPath := filepath.Join(tmpDir, "smartctl-dir")
	require.NoError(t, os.Mkdir(dirPath, 0o755))

	execFile := filepath.Join(tmpDir, "smartctl-real")
	require.NoError(t, os.WriteFile(execFile, []byte("#!/bin/sh"), 0o755))

	orig := smartctlSearchPaths
	t.Cleanup(func() { smartctlSearchPaths = orig })
	smartctlSearchPaths = []string{dirPath, execFile}

	t.Setenv("PATH", "")

	got, err := resolveSmartctlPath()
	require.NoError(t, err)
	assert.Equal(t, execFile, got, "directory entry should be skipped")
}
