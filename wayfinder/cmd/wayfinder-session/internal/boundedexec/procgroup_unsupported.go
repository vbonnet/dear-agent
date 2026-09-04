//go:build !darwin && !linux

package boundedexec

import "os/exec"

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
