//go:build !darwin && !linux

package boundedexec

import "os/exec"

// isolateProcessGroup is a no-op where process groups are unavailable. The
// wall-clock bound still holds through WaitDelay; only descendant reaping is
// lost, so a killed launcher's children may outlive the gate.
func isolateProcessGroup(_ *exec.Cmd) {}
