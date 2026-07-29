package tmux

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSingleQuote(t *testing.T) {
	assert.Equal(t, "'/tmp/plain'", singleQuote("/tmp/plain"))
	assert.Equal(t, `'/tmp/it'\''s here'`, singleQuote("/tmp/it's here"))
	assert.Equal(t, "''", singleQuote(""))
}

func TestEnsureSessionWorkDirRejectsControlsBeforeTmuxAccess(t *testing.T) {
	err := EnsureSessionWorkDir("unused", "/safe\x1b[201~\nunsafe")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "control characters"), err)
}

func TestIsShellCommand(t *testing.T) {
	for _, shell := range []string{"zsh", "bash", "sh", "ash", "fish", "-zsh", "/bin/bash"} {
		assert.True(t, IsShellCommand(shell), "expected %q to be a shell", shell)
	}
	for _, notShell := range []string{"claude", "node", "bun", "env", "codex", "agm", ""} {
		assert.False(t, IsShellCommand(notShell), "expected %q to not be a shell", notShell)
	}
}

func TestSamePath_ResolvesSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "upper")
	require.NoError(t, os.Mkdir(target, 0o755))
	link := filepath.Join(dir, "merged")
	require.NoError(t, os.Symlink(target, link))

	assert.True(t, samePath(link, target), "symlink and target should compare equal")
	assert.True(t, samePath(target, target))
	assert.False(t, samePath(target, filepath.Join(dir, "other")))
}

// TestNewSession_RepairsWorkDirWhenServerCwdDeleted is the regression test for
// ce-5zbg: a tmux server whose own working directory has been deleted silently
// ignores `new-session -c <dir>` (spawn.c guards the pane chdir behind
// getcwd()), so panes inherit the dead directory and harness CLIs die
// instantly at startup. NewSession must detect the mis-placed pane and steer
// it into the requested workdir.
func TestNewSession_RepairsWorkDirWhenServerCwdDeleted(t *testing.T) {
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()
	setupTestState(t)

	// Start the tmux server from a directory that we then delete, wedging the
	// server's cwd exactly like a reaped git worktree does. (t.TempDir cleanup
	// tolerates the mid-test removal.)
	doomedDir := t.TempDir()
	seedCmd := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", "seed-session")
	seedCmd.Dir = doomedDir
	require.NoError(t, seedCmd.Run(), "failed to start seed session")
	defer exec.Command("tmux", "-S", socketPath, "kill-server").Run()
	time.Sleep(200 * time.Millisecond)
	require.NoError(t, os.RemoveAll(doomedDir), "failed to delete server cwd")

	// Sanity-check the wedge is real: a raw `new-session -c` on this server
	// must NOT land the pane in the requested directory. If tmux ever fixes
	// this upstream the repair path becomes dead code and this test should be
	// revisited.
	rawTarget := t.TempDir()
	require.NoError(t, exec.Command("tmux", "-S", socketPath,
		"new-session", "-d", "-s", "raw-session", "-c", rawTarget).Run())
	time.Sleep(500 * time.Millisecond)
	rawPath, err := PaneCurrentPath("raw-session")
	require.NoError(t, err)
	if samePath(rawPath, rawTarget) {
		t.Skipf("tmux %s honored -c despite deleted server cwd; wedge not reproducible", tmuxVersionString(t))
	}

	// The actual assertion: NewSession must leave the pane in workDir even on
	// the wedged server (verify + `cd` repair).
	workDir := t.TempDir()
	sessionName := "test-workdir-repair"
	require.NoError(t, NewSession(sessionName, workDir))
	defer killSession(sessionName)

	panePath, err := PaneCurrentPath(sessionName)
	require.NoError(t, err)
	assert.True(t, samePath(panePath, workDir),
		"pane cwd %q should match requested workdir %q after repair", panePath, workDir)
}

// TestNewSession_WorkDirHonored covers the healthy-server happy path: no
// repair needed, NewSession still verifies and returns quickly.
func TestNewSession_WorkDirHonored(t *testing.T) {
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()
	setupTestState(t)

	workDir := t.TempDir()
	sessionName := "test-workdir-honored"
	start := time.Now()
	require.NoError(t, NewSession(sessionName, workDir))
	defer killSession(sessionName)

	panePath, err := PaneCurrentPath(sessionName)
	require.NoError(t, err)
	assert.True(t, samePath(panePath, workDir),
		"pane cwd %q should match requested workdir %q", panePath, workDir)
	assert.Less(t, time.Since(start), workDirVerifyDeadline,
		"happy path should not exhaust the verify deadline")
}

func tmuxVersionString(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("tmux", "-V").CombinedOutput()
	if err != nil {
		return "unknown"
	}
	return string(out)
}
