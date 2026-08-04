//go:build darwin

package steps

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const specAuditDarwinPIDWaitType = 1 // P_PID from <sys/wait.h>.

const (
	specAuditDarwinChildExited = 1 // CLD_EXITED from <sys/signal.h>.
	specAuditDarwinChildKilled = 2 // CLD_KILLED from <sys/signal.h>.
	specAuditDarwinChildDumped = 3 // CLD_DUMPED from <sys/signal.h>.
)

// specAuditDarwinSiginfo matches siginfo_t on Darwin's supported 64-bit Go
// architectures. Only code and pid are inspected, but the kernel requires a
// correctly sized and aligned output object.
type specAuditDarwinSiginfo struct {
	signo  int32
	errno  int32
	code   int32
	pid    int32
	uid    uint32
	status int32
	addr   uintptr
	value  uintptr
	band   int64
	pad    [7]uint64
}

// waitForSpecAuditCommandExitWithoutReaping waits until pid exits while
// leaving its zombie entry intact. Darwin's x/sys package does not expose a
// waitid wrapper, so call the kernel interface directly with the typed
// siginfo_t-compatible result above.
func waitForSpecAuditCommandExitWithoutReaping(pid int) error {
	var info specAuditDarwinSiginfo
	for {
		_, _, errno := syscall.Syscall6(
			syscall.SYS_WAITID,
			specAuditDarwinPIDWaitType,
			uintptr(pid),
			uintptr(unsafe.Pointer(&info)),
			syscall.WEXITED|syscall.WNOWAIT,
			0,
			0,
		)
		runtime.KeepAlive(&info)
		if errno == 0 {
			if int(info.pid) != pid {
				return fmt.Errorf("darwin waitid returned child pid %d, want %d", info.pid, pid)
			}
			switch info.code {
			case specAuditDarwinChildExited, specAuditDarwinChildKilled, specAuditDarwinChildDumped:
				return nil
			default:
				// Darwin can wake waitid(WEXITED) for SIGSTOP despite the
				// requested filter (Go issue 19314). Report that state instead
				// of pretending the live child is an unreaped zombie. The caller
				// will fail closed by killing the still-pinned process group.
				return fmt.Errorf("darwin waitid returned non-terminal child state code %d", info.code)
			}
		}
		if !errors.Is(errno, syscall.EINTR) {
			return errno
		}
	}
}

// specAuditProcessGroupEPERMComplete distinguishes Darwin's ordinary
// zombie-leader EPERM from an isolated group that still contains an
// unsignalable descendant. This classification relies on the runner's trusted,
// same-credential child model and requires the process-table snapshot to show
// the pinned leader and no other current group member.
func specAuditProcessGroupEPERMComplete(processGroupID int, directChildExitObserved bool) (bool, error) {
	if !directChildExitObserved {
		return false, nil
	}
	members, err := unix.SysctlKinfoProcSlice("kern.proc.pgrp", processGroupID)
	if err != nil {
		return false, err
	}
	leaderFound := false
	for _, member := range members {
		if int(member.Proc.P_pid) == processGroupID {
			leaderFound = true
		} else {
			return false, nil
		}
	}
	return leaderFound, nil
}
