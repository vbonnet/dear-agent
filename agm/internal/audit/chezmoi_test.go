package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseChezmoiDiffPaths_Empty(t *testing.T) {
	assert.Empty(t, parseChezmoiDiffPaths(""))
}

func TestParseChezmoiDiffPaths_SingleFile(t *testing.T) {
	out := `diff --git a/.bashrc b/.bashrc
index 1234..5678 100644
--- a/.bashrc
+++ b/.bashrc
@@ -1,3 +1,4 @@
 # before
+# added line
 # after
`
	paths := parseChezmoiDiffPaths(out)
	assert.Equal(t, []string{"/.bashrc"}, paths)
}

func TestParseChezmoiDiffPaths_MultipleFiles(t *testing.T) {
	out := `diff --git a/.bashrc b/.bashrc
@@ -1 +1 @@
-old
+new
diff --git a/.config/foo/bar b/.config/foo/bar
@@ -1 +1 @@
-x
+y
`
	paths := parseChezmoiDiffPaths(out)
	assert.Equal(t, []string{"/.bashrc", "/.config/foo/bar"}, paths)
}

func TestParseChezmoiDiffPaths_DeduplicatesRepeats(t *testing.T) {
	out := `diff --git a/.bashrc b/.bashrc
@@ -1 +1 @@
diff --git a/.bashrc b/.bashrc
@@ -2 +2 @@
`
	paths := parseChezmoiDiffPaths(out)
	assert.Equal(t, []string{"/.bashrc"}, paths)
}

func TestParseChezmoiDiffPaths_IgnoresNoise(t *testing.T) {
	out := `chezmoi: warning: 'encryption' not set
some preamble
diff --git a/.zshrc b/.zshrc
some other line
`
	paths := parseChezmoiDiffPaths(out)
	assert.Equal(t, []string{"/.zshrc"}, paths)
}

func TestParseChezmoiDiffPaths_SkipsChezmoiscripts(t *testing.T) {
	out := `diff --git a/.chezmoiscripts/run_once_install.sh b/.chezmoiscripts/run_once_install.sh
@@ -0,0 +1 @@
+#!/bin/bash
diff --git a/.bashrc b/.bashrc
@@ -1 +1 @@
-old
+new
`
	paths := parseChezmoiDiffPaths(out)
	assert.Equal(t, []string{"/.bashrc"}, paths)
}

// TestCheckChezmoiDrift_NoBinary verifies the check is a no-op when chezmoi
// is not on PATH. We invoke the function with a sanitized PATH so LookPath
// fails regardless of host setup.
func TestCheckChezmoiDrift_NoBinary(t *testing.T) {
	t.Setenv("PATH", "")
	issues, err := checkChezmoiDrift()
	require.NoError(t, err)
	assert.Nil(t, issues)
}

// TestCheckChezmoiDrift_SmokeWhenInstalled exercises the installed chezmoi
// against a synthetic source and home, independent of host dotfiles.
func TestCheckChezmoiDrift_SmokeWhenInstalled(t *testing.T) {
	if !chezmoiInstalled() {
		t.Skip("chezmoi not installed")
	}

	root := t.TempDir()
	home := filepath.Join(root, "home")
	data := filepath.Join(root, "data")
	source := filepath.Join(data, "chezmoi")
	require.NoError(t, os.MkdirAll(home, 0o700))
	require.NoError(t, os.MkdirAll(source, 0o700))
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "CHEZMOI_") {
			t.Setenv(key, "")
			require.NoError(t, os.Unsetenv(key))
		}
	}

	const target = ".agm-audit-smoke"
	require.NoError(t, os.WriteFile(filepath.Join(source, "dot_agm-audit-smoke"), []byte("managed\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(home, target), []byte("managed\n"), 0o644))

	issues, err := checkChezmoiDrift()
	require.NoError(t, err)
	assert.Empty(t, issues)

	require.NoError(t, os.WriteFile(filepath.Join(home, target), []byte("local edit\n"), 0o644))
	issues, err = checkChezmoiDrift()
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, IssueChezmoiDrift, issues[0].Type)
	assert.Equal(t, SeverityWarning, issues[0].Severity)
	assert.Equal(t, "/"+target, issues[0].Path)
}
