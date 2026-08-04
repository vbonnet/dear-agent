//go:build linux

package steps

import (
	"errors"
	"syscall"

	"golang.org/x/sys/unix"
)

// waitForSpecAuditCommandExitWithoutReaping waits until pid exits while
// leaving its zombie entry intact. Cmd.Wait subsequently performs the one
// reap and preserves the direct child's real exit status.
func waitForSpecAuditCommandExitWithoutReaping(pid int) error {
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

func specAuditProcessGroupEPERMComplete(_ int, _ bool) (bool, error) {
	return false, nil
}
