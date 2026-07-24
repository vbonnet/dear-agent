package tmux

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubProbes replaces the liveness probes for the duration of a test so the
// CleanStaleSocket decision ladder can be exercised without a real server.
func stubProbes(t *testing.T, dialable, tmuxOK bool, pids []int, pidErr error) {
	t.Helper()
	origDial, origCmd, origPIDs := probeDialable, probeTmuxCommand, probeServerPIDs
	t.Cleanup(func() {
		probeDialable, probeTmuxCommand, probeServerPIDs = origDial, origCmd, origPIDs
	})
	probeDialable = func(string) bool { return dialable }
	probeTmuxCommand = func(string) bool { return tmuxOK }
	probeServerPIDs = func(string) ([]int, error) { return pids, pidErr }
}

// newSocketFile creates a real Unix socket file at a short path. macOS caps
// sockaddr_un paths at ~104 bytes, which t.TempDir() can exceed.
func newSocketFile(t *testing.T) (path string, closeFn func()) {
	t.Helper()
	// Not t.TempDir(): macOS caps sockaddr_un paths near 104 bytes and the
	// per-test temp path overflows it ("invalid argument" on bind).
	dir, err := os.MkdirTemp("/tmp", "agmsock") //nolint:usetesting // see above
	require.NoError(t, err)
	path = filepath.Join(dir, "s.sock")

	ln, err := net.Listen("unix", path)
	require.NoError(t, err)

	return path, func() {
		ln.Close()
		os.RemoveAll(dir)
	}
}

// TestCleanStaleSocket_KeepsSocketWhenServerBound is the ce-7ep9 regression
// test. A dial that fails while a live server is bound must NOT delete the
// socket: unlinking it strands that server's sessions permanently.
func TestCleanStaleSocket_KeepsSocketWhenServerBound(t *testing.T) {
	path, cleanup := newSocketFile(t)
	defer cleanup()
	t.Setenv("AGM_TMUX_SOCKET", path)

	// Both reachability probes fail (busy/backlogged server), but the process
	// table proves a server is bound.
	stubProbes(t, false, false, []int{4242}, nil)

	err := CleanStaleSocket()

	var lsb *LiveServerBoundError
	require.ErrorAs(t, err, &lsb, "should report the orphaned-server state")
	assert.Equal(t, []int{4242}, lsb.PIDs)
	assert.NotContains(t, err.Error(), "kill 4242", "a busy server must not be treated as disposable")
	assert.Contains(t, err.Error(), "socket was retained")

	assert.FileExists(t, path, "socket must survive: deleting it orphans the live server")
}

// A dial can lose a race with a loaded server. tmux answering is proof of life,
// so the socket stays and no process scan is needed.
func TestCleanStaleSocket_KeepsSocketWhenTmuxAnswers(t *testing.T) {
	path, cleanup := newSocketFile(t)
	defer cleanup()
	t.Setenv("AGM_TMUX_SOCKET", path)

	scanned := false
	stubProbes(t, false, true, nil, nil)
	origPIDs := probeServerPIDs
	t.Cleanup(func() { probeServerPIDs = origPIDs })
	probeServerPIDs = func(string) ([]int, error) { scanned = true; return nil, nil }

	require.NoError(t, CleanStaleSocket())
	assert.FileExists(t, path, "socket must survive when tmux answers")
	assert.False(t, scanned, "should short-circuit before the process scan")
}

// The happy path: unreachable AND nothing bound is the only case that deletes.
func TestCleanStaleSocket_RemovesProvenStaleSocket(t *testing.T) {
	path, cleanup := newSocketFile(t)
	defer cleanup()
	t.Setenv("AGM_TMUX_SOCKET", path)

	stubProbes(t, false, false, nil, nil)

	require.NoError(t, CleanStaleSocket())
	assert.NoFileExists(t, path, "a provably stale socket should be removed")
}

// If the process scan fails, staleness is unproven. Refusing to delete costs a
// retry; deleting wrongly costs the sessions.
func TestCleanStaleSocket_RefusesWhenScanFails(t *testing.T) {
	path, cleanup := newSocketFile(t)
	defer cleanup()
	t.Setenv("AGM_TMUX_SOCKET", path)

	stubProbes(t, false, false, nil, errors.New("ps unavailable"))

	err := CleanStaleSocket()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to remove socket")
	assert.FileExists(t, path, "socket must survive an inconclusive scan")
}

// ServerAliveOrRecover must preserve inconclusive cleanup errors instead of
// replacing them with generic removal advice.
func TestServerAliveOrRecover_PreservesUnresponsiveServerError(t *testing.T) {
	path, cleanup := newSocketFile(t)
	defer cleanup()
	t.Setenv("AGM_TMUX_SOCKET", path)

	stubProbes(t, false, false, []int{4242}, nil)

	err := ServerAliveOrRecover()

	var lsb *LiveServerBoundError
	require.ErrorAs(t, err, &lsb, "unresponsive-server error must survive ServerAliveOrRecover")
	assert.NotContains(t, err.Error(), "rm -f", "must never advise removing an existing socket")
	assert.FileExists(t, path)
}

func TestServerAliveOrRecover_PreservesScanFailure(t *testing.T) {
	path, cleanup := newSocketFile(t)
	defer cleanup()
	t.Setenv("AGM_TMUX_SOCKET", path)

	stubProbes(t, false, false, nil, errors.New("ps unavailable"))

	err := ServerAliveOrRecover()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to remove socket")
	assert.NotContains(t, err.Error(), "rm -f")
	assert.FileExists(t, path)
}

func TestDetectOrphanedServer(t *testing.T) {
	t.Setenv("AGM_TMUX_SOCKET", "/tmp/agm-detect-test.sock")

	t.Run("reachable server is not orphaned", func(t *testing.T) {
		stubProbes(t, true, false, []int{99}, nil)
		got, err := DetectOrphanedServer()
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("no socket and no server is not orphaned", func(t *testing.T) {
		stubProbes(t, false, false, nil, nil)
		got, err := DetectOrphanedServer()
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("unreachable with live server is orphaned", func(t *testing.T) {
		stubProbes(t, false, false, []int{7333, 79020}, nil)
		got, err := DetectOrphanedServer()
		require.NoError(t, err)
		require.NotNil(t, got, "socket gone + server alive is the ce-7ep9 state")
		assert.Equal(t, []int{7333, 79020}, got.PIDs)
	})
}

func TestIsTmuxCommand(t *testing.T) {
	for _, tc := range []struct {
		arg0 string
		want bool
	}{
		{"tmux", true},
		{"/opt/homebrew/bin/tmux", true},
		{"/usr/bin/tmux", true},
		{"tmuxinator", false},
		{"/usr/bin/tmuxp", false},
		{"vim", false},
	} {
		assert.Equal(t, tc.want, isTmuxCommand(tc.arg0), "arg0=%s", tc.arg0)
	}
}

func TestProcessExited(t *testing.T) {
	assert.True(t, processExited(os.ErrNotExist))
	assert.True(t, processExited(syscall.ESRCH))
	assert.False(t, processExited(errors.New("permission denied")))
}

// TestFindServerPIDs_RealServer proves the process scan against a real tmux
// server, and — critically — that it still finds the server after the socket
// file has been unlinked. That is the state no socket-based probe can see.
func TestFindServerPIDs_RealServer(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}

	// The whitespace exercises the lossless argv path. ps's display-oriented
	// command column cannot preserve this as one -S argument.
	dir, err := os.MkdirTemp("/tmp", "agm real") //nolint:usetesting // sockaddr_un path limit, see newSocketFile
	require.NoError(t, err)
	defer os.RemoveAll(dir)
	sock := filepath.Join(dir, "r.sock")

	session := fmt.Sprintf("ce7ep9-%d", os.Getpid())
	require.NoError(t, exec.Command("tmux", "-S", sock, "new-session", "-d",
		"-s", session, "sleep", "120").Run())
	var serverPIDs []int
	t.Cleanup(func() {
		// After the test unlinks sock, tmux -S can no longer reach this server.
		// Kill the discovered server PID directly so successful runs do not leak
		// an orphan and its sleep process until the command's timeout expires.
		for _, pid := range serverPIDs {
			if process, err := os.FindProcess(pid); err == nil {
				_ = process.Kill()
			}
		}
		// Before a PID is discovered (for example, if setup fails), the socket
		// still exists and tmux can clean up normally.
		_ = exec.Command("tmux", "-S", sock, "kill-server").Run()
	})

	require.Eventually(t, func() bool {
		pids, _ := FindServerPIDs(sock)
		serverPIDs = pids
		return len(pids) > 0
	}, 5*time.Second, 100*time.Millisecond, "should find the live server")

	// An unrelated socket path must not match.
	other, err := FindServerPIDs(sock + ".bak")
	require.NoError(t, err)
	assert.Empty(t, other, "must match whole argv tokens, not prefixes")

	// Unlink the socket: the server keeps running but becomes unreachable.
	require.NoError(t, os.Remove(sock))
	assert.False(t, socketDialable(sock), "socket is gone, so unreachable")

	pids, err := FindServerPIDs(sock)
	require.NoError(t, err)
	assert.NotEmpty(t, pids, "orphaned server must remain detectable after unlink")
}
