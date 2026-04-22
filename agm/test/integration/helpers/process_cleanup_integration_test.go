//go:build integration

package helpers

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestKillSessionProcesses_KillsChildProcesses verifies that KillSessionProcesses
// kills all processes running inside a tmux session's panes, including children.
// This prevents orphaned Claude processes when tests finish.
func TestKillSessionProcesses_KillsChildProcesses(t *testing.T) {
	if !IsTmuxAvailable() {
		t.Skip("Tmux not available")
	}

	sessionName := "test-kill-procs-" + RandomString(6)

	// Create tmux session running bash
	cmd := BuildTmuxCmd("new-session", "-d", "-s", sessionName, "bash")
	require.NoError(t, cmd.Run(), "Failed to create tmux session")

	// Send command to spawn child processes (simulates claude spawning children)
	sendCmd := BuildTmuxCmd("send-keys", "-t", sessionName, "sleep 300 & sleep 300 & sleep 300 &", "C-m")
	require.NoError(t, sendCmd.Run(), "Failed to send keys")

	// Wait for child processes to start
	time.Sleep(500 * time.Millisecond)

	// Get pane PID
	pidCmd := BuildTmuxCmd("list-panes", "-t", sessionName, "-F", "#{pane_pid}")
	pidOutput, err := pidCmd.Output()
	require.NoError(t, err, "Failed to get pane PID")

	panePid, err := strconv.Atoi(strings.TrimSpace(string(pidOutput)))
	require.NoError(t, err, "Failed to parse pane PID")

	// Verify pane process is running
	err = syscall.Kill(panePid, 0)
	require.NoError(t, err, "Pane process should be running")

	// Get process group to check children later
	pgid, err := syscall.Getpgid(panePid)
	require.NoError(t, err, "Failed to get PGID")

	// Kill session processes
	err = KillSessionProcesses(sessionName)
	require.NoError(t, err, "KillSessionProcesses should not error")

	// Wait for processes to die
	time.Sleep(300 * time.Millisecond)

	// Verify pane process is dead
	err = syscall.Kill(panePid, 0)
	require.Error(t, err, "Pane process should be terminated")

	// Verify no processes remain in the process group
	// Use ps to check for any remaining processes in the group
	psCmd := exec.Command("ps", "-o", "pid=", "--pgroup", strconv.Itoa(pgid))
	psOutput, _ := psCmd.Output()
	remaining := strings.TrimSpace(string(psOutput))
	if remaining != "" {
		t.Errorf("Orphaned processes remain in process group %d: %s", pgid, remaining)
	}

	// Verify tmux session is gone
	hasCmd := BuildTmuxCmd("has-session", "-t", sessionName)
	err = hasCmd.Run()
	require.Error(t, err, "Tmux session should be terminated")
}

// TestKillTmuxServer_KillsAllSessions verifies that KillTmuxServer kills the
// entire tmux server, terminating all sessions and their processes.
func TestKillTmuxServer_KillsAllSessions(t *testing.T) {
	// Use an isolated socket for this test to avoid killing user sessions
	isolatedSocket := "/tmp/test-kill-server-" + RandomString(6) + ".sock"
	defer os.Remove(isolatedSocket)

	// Override socket for this test
	originalSocket := os.Getenv("AGM_TMUX_SOCKET")
	os.Setenv("AGM_TMUX_SOCKET", isolatedSocket)
	defer os.Setenv("AGM_TMUX_SOCKET", originalSocket)

	// Create multiple sessions on the isolated server
	for i := 0; i < 3; i++ {
		name := "test-server-kill-" + strconv.Itoa(i)
		cmd := exec.Command("tmux", "-S", isolatedSocket, "new-session", "-d", "-s", name, "sleep", "300")
		require.NoError(t, cmd.Run(), "Failed to create session %s", name)
	}

	// Verify sessions exist
	listCmd := exec.Command("tmux", "-S", isolatedSocket, "list-sessions", "-F", "#{session_name}")
	output, err := listCmd.Output()
	require.NoError(t, err, "Failed to list sessions")
	require.Contains(t, string(output), "test-server-kill-0")

	// Kill the entire server
	err = KillTmuxServer()
	require.NoError(t, err, "KillTmuxServer should not error")

	// Wait for server to die
	time.Sleep(200 * time.Millisecond)

	// Verify server is gone (list-sessions should fail)
	listCmd = exec.Command("tmux", "-S", isolatedSocket, "list-sessions")
	err = listCmd.Run()
	require.Error(t, err, "Tmux server should be terminated")
}

// TestKillSessionProcesses_NilSafety verifies that KillSessionProcesses handles
// non-existent sessions gracefully without errors or panics.
func TestKillSessionProcesses_NilSafety(t *testing.T) {
	// Should not panic or error with non-existent session
	err := KillSessionProcesses("nonexistent-session-" + RandomString(8))
	require.NoError(t, err, "Should handle non-existent session gracefully")
}

// TestCleanupProcessGroup_WithAgmStyleSpawn verifies that process groups created
// with Setpgid are properly cleaned up, including all child processes.
// This simulates the pattern used by agm when spawning Claude processes.
func TestCleanupProcessGroup_WithAgmStyleSpawn(t *testing.T) {
	// Simulate agm-style spawn: parent process with children in its own group
	cmd := exec.Command("bash", "-c", "sleep 300 & sleep 300 & wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	err := cmd.Start()
	require.NoError(t, err, "Failed to start process group")

	parentPid := cmd.Process.Pid

	// Wait for children to spawn
	time.Sleep(300 * time.Millisecond)

	// Verify parent is running
	err = syscall.Kill(parentPid, 0)
	require.NoError(t, err, "Parent process should be running")

	// Get process group
	pgid, err := syscall.Getpgid(parentPid)
	require.NoError(t, err, "Failed to get PGID")
	require.Equal(t, parentPid, pgid, "Process should be group leader")

	// Kill entire process group (SIGTERM then SIGKILL)
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	time.Sleep(200 * time.Millisecond)
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
	_ = cmd.Wait()

	// Wait for cleanup
	time.Sleep(200 * time.Millisecond)

	// Verify parent is dead
	err = syscall.Kill(parentPid, 0)
	require.Error(t, err, "Parent process should be terminated")

	// Verify no children remain in process group
	psCmd := exec.Command("ps", "-o", "pid=", "--pgroup", strconv.Itoa(pgid))
	psOutput, _ := psCmd.Output()
	remaining := strings.TrimSpace(string(psOutput))
	if remaining != "" {
		t.Errorf("Orphaned processes remain in process group %d: %s", pgid, remaining)
	}
}
