//go:build darwin || linux

package boundedexec

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// configureProcessGroup puts the command in its own process group so that
// cancellation can address the whole tree.
//
// exec.CommandContext's default cancel signals only the direct child. For the
// commands Wayfinder gates run, that child is a launcher: `go test` spawns
// per-package binaries, npm spawns a script, a provider CLI spawns a worker.
// Killing the launcher alone leaves those descendants running past the gate's
// own failure report, still holding the inherited output pipes and still
// mutating build caches. Bounding the caller's wait without this hides the leak.
func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessTree kills the command's whole process group.
func killProcessTree(cmd *exec.Cmd) error {
	if cmd.Process == nil || cmd.Process.Pid <= 0 {
		return nil
	}
	// The negative PID addresses the group. Safe here because os/exec invokes
	// Cancel before reaping, so the PID still names this group and cannot have
	// been recycled.
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		// Already gone. Reported distinctly because "nothing to kill" is not
		// the same as "we killed it", and the classification depends on that.
		return os.ErrProcessDone
	}
	return err
}

// endedByCancellation reports whether the process died from a signal, which is
// what a cancellation kill looks like here. Unix distinguishes an exited status
// from a signalled one, so the recorded state answers this on its own.
func endedByCancellation(state *os.ProcessState, _ bool) bool {
	return !state.Exited()
}

// reraise re-sends the signal that this package absorbed, after the handler has
// been restored, so the interrupt still terminates Wayfinder itself and does so
// as the signal the supervisor actually sent.
//
// It can return: a signal the process inherited as ignored is delivered and
// discarded. Callers must stay correct in that case rather than treating this
// as unreachable.
func reraise(sig os.Signal) {
	unixSig, ok := sig.(syscall.Signal)
	if !ok {
		unixSig = syscall.SIGINT
	}
	if err := syscall.Kill(os.Getpid(), unixSig); err != nil {
		os.Exit(signalExitCode(unixSig))
	}
}

// signalExitCode is the conventional shell status for death by a signal.
func signalExitCode(sig syscall.Signal) int { return 128 + int(sig) }
