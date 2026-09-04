//go:build darwin || linux

package boundedexec

import (
	"errors"
	"os/exec"
	"syscall"
)

// isolateProcessGroup puts the command in its own process group and makes
// cancellation kill that whole group.
//
// exec.CommandContext's default cancel signals only the direct child. For the
// commands Wayfinder gates run, the direct child is a launcher: `go test`
// spawns per-package test binaries, npm spawns a script, a provider CLI spawns
// a worker. Killing the launcher alone leaves those descendants running past
// the gate's own failure report, still holding the inherited output pipes and
// still mutating build caches. Bounding the caller's wait without this only
// hides the leak.
func isolateProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil || cmd.Process.Pid <= 0 {
			return nil
		}
		// Negative PID addresses the group. Safe here because os/exec invokes
		// Cancel before reaping, so the PID still names this group and cannot
		// have been recycled.
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return nil // already gone, which is the outcome we wanted
		}
		return err
	}
}
