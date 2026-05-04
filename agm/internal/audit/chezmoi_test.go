package audit

import (
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

// TestCheckChezmoiDrift_NoBinary verifies the check is a no-op when chezmoi
// is not on PATH. We invoke the function with a sanitized PATH so LookPath
// fails regardless of host setup.
func TestCheckChezmoiDrift_NoBinary(t *testing.T) {
	t.Setenv("PATH", "")
	issues, err := checkChezmoiDrift()
	require.NoError(t, err)
	assert.Nil(t, issues)
}

// TestCheckChezmoiDrift_SmokeWhenInstalled runs against the host's chezmoi if
// it's installed. We don't assert on the issue list (it depends on host
// state); we just verify the check runs without error.
func TestCheckChezmoiDrift_SmokeWhenInstalled(t *testing.T) {
	if !chezmoiInstalled() {
		t.Skip("chezmoi not installed")
	}
	_, err := checkChezmoiDrift()
	assert.NoError(t, err)
}
