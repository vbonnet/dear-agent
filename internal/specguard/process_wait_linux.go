//go:build linux

package specguard

import (
	"errors"
	"syscall"

	"golang.org/x/sys/unix"
)

// waitForGitCommandExitWithoutReaping waits until pid exits while leaving its
// zombie entry intact. exec.Cmd.Wait subsequently performs the single reap and
// preserves the direct child's real exit status.
func waitForGitCommandExitWithoutReaping(pid int) error {
	var info unix.Siginfo
	for {
		err := unix.Waitid(unix.P_PID, pid, &info, unix.WEXITED|unix.WNOWAIT, nil)
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}

func gitProcessGroupEPERMComplete(_ int, _ bool) (bool, error) {
	return false, nil
}
