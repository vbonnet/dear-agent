package helpers

import (
	"os/exec"
	"syscall"
	"testing"
)

// ProcessGroup tracks a process group for cleanup.
type ProcessGroup struct {
	pgid int
	cmd  *exec.Cmd
}

// TrackProcessGroup starts a command in a new process group for easier cleanup.
//
// Sets up the command to run in its own process group (PGID) so that all
// child processes can be killed together. This prevents orphaned processes
// when tests fail or are interrupted.
//
// The process group is automatically cleaned up via t.Cleanup() using SIGTERM
// followed by SIGKILL if needed.
//
// Parameters:
//   - cmd: exec.Cmd to run in new process group
//
// Returns:
//   - ProcessGroup handle for the tracked process group
//
// Example:
//
//	cmd := exec.Command("tmux", "new-session", "-d")
//	pg := helpers.TrackProcessGroup(t, cmd)
//	err := cmd.Start()
//	require.NoError(t, err)
//	// ... test code ...
//	// Cleanup happens automatically via t.Cleanup()
func TrackProcessGroup(t *testing.T, cmd *exec.Cmd) *ProcessGroup {
	t.Helper()

	// Set process group ID to create new group
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	pg := &ProcessGroup{
		cmd: cmd,
	}

	// Register cleanup handler
	t.Cleanup(func() {
		CleanupProcessGroup(t, pg)
	})

	return pg
}

// CleanupProcessGroup terminates all processes in the process group.
//
// Sends SIGTERM to the process group, waits briefly, then sends SIGKILL
// if processes are still running. This ensures complete cleanup even if
// processes ignore SIGTERM.
//
// The function is safe to call multiple times and handles cases where
// the process group no longer exists.
//
// Parameters:
//   - pg: ProcessGroup handle (from TrackProcessGroup)
//
// Example:
//
//	pg := helpers.TrackProcessGroup(t, cmd)
//	cmd.Start()
//	// ... test code ...
//	helpers.CleanupProcessGroup(t, pg) // Manual cleanup if needed
func CleanupProcessGroup(t *testing.T, pg *ProcessGroup) {
	t.Helper()

	if pg == nil || pg.cmd == nil || pg.cmd.Process == nil {
		return
	}

	// Get the process group ID
	pgid, err := syscall.Getpgid(pg.cmd.Process.Pid)
	if err != nil {
		// Process already exited
		return
	}

	// Send SIGTERM to entire process group (negative PID)
	_ = syscall.Kill(-pgid, syscall.SIGTERM)

	// Wait for process to exit
	_ = pg.cmd.Wait()

	// If still running, send SIGKILL to ensure cleanup
	if pg.cmd.ProcessState == nil || !pg.cmd.ProcessState.Exited() {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		_ = pg.cmd.Wait()
	}
}
