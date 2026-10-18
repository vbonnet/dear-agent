//go:build !darwin && !linux

package boundedexec

import (
	"os"
	"os/exec"
)

// interruptExitCode is the conventional shell status for death by SIGINT, used
// where re-raising the signal itself is unavailable.
const interruptExitCode = 130

// configureProcessGroup is a no-op where process groups are unavailable.
func configureProcessGroup(_ *exec.Cmd) {}

// killProcessTree falls back to killing the direct child only. The wall-clock
// bound still holds through WaitDelay; only descendant reaping is lost, so a
// killed launcher's children may outlive the gate.
func killProcessTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

// endedByCancellation falls back to whether cancellation ran, because Windows
// reports every reaped process as Exited, including one killed at the deadline.
// The state cannot separate a deadline kill from an ordinary exit there, so a
// timeout would otherwise be reported as a plain build or test failure.
func endedByCancellation(_ *os.ProcessState, cancelled bool) bool {
	return cancelled
}

// reraise exits directly, because sending a signal to the current process is
// not supported on the platforms selected by this file.
func reraise(_ os.Signal) {
	os.Exit(interruptExitCode)
}
