//go:build darwin

package specguard

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const gitDarwinPIDWaitType = 1 // P_PID from <sys/wait.h>.

// gitDarwinProcessZombie is SZOMB from Darwin's <sys/proc.h>. x/sys exposes
// ExternProc.P_stat but not the corresponding process-state constants.
const gitDarwinProcessZombie int8 = 5

const (
	gitDarwinProcessGroupDrainGrace = 500 * time.Millisecond
	gitDarwinProcessGroupPoll       = 5 * time.Millisecond
)

const (
	gitDarwinChildExited = 1 // CLD_EXITED from <sys/signal.h>.
	gitDarwinChildKilled = 2 // CLD_KILLED from <sys/signal.h>.
	gitDarwinChildDumped = 3 // CLD_DUMPED from <sys/signal.h>.
)

// gitDarwinSiginfo matches siginfo_t on Darwin's supported 64-bit Go
// architectures. Only code and pid are inspected, but the kernel requires a
// correctly sized and aligned output object.
type gitDarwinSiginfo struct {
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

// waitForGitCommandExitWithoutReaping waits until pid exits while leaving its
// zombie entry intact. Darwin's x/sys package does not expose waitid, so call
// the kernel interface directly with a typed siginfo_t-compatible result.
func waitForGitCommandExitWithoutReaping(pid int) error {
	var info gitDarwinSiginfo
	for {
		_, _, callErr := syscall.Syscall6(
			syscall.SYS_WAITID,
			gitDarwinPIDWaitType,
			uintptr(pid),
			uintptr(unsafe.Pointer(&info)),
			syscall.WEXITED|syscall.WNOWAIT,
			0,
			0,
		)
		runtime.KeepAlive(&info)
		if callErr == 0 {
			if int(info.pid) != pid {
				return fmt.Errorf("darwin waitid returned Git child pid %d, want %d", info.pid, pid)
			}
			switch info.code {
			case gitDarwinChildExited, gitDarwinChildKilled, gitDarwinChildDumped:
				return nil
			default:
				// Darwin can wake waitid(WEXITED) for SIGSTOP despite the
				// requested filter (Go issue 19314). Do not mistake the live
				// child for an unreaped zombie; the caller must fail closed while
				// its PID still pins the process-group ID.
				return fmt.Errorf("darwin waitid returned non-terminal Git child state code %d", info.code)
			}
		}
		if !errors.Is(callErr, syscall.EINTR) {
			return callErr
		}
	}
}

// gitProcessGroupEPERMComplete distinguishes Darwin's ordinary zombie-only
// EPERM from an isolated group that still contains an unsignalable live
// descendant. The trusted, same-credential Git model is complete only when
// the process table contains the pinned leader and every remaining member has
// already reached the terminal zombie state. A bounded drain is permitted
// only after an earlier group signal succeeded; otherwise uncertainty fails
// immediately rather than treating natural descendant exit as cleanup proof.
func gitProcessGroupEPERMComplete(processGroupID int, directChildExitObserved, terminationSignaled bool) (bool, error) {
	if !directChildExitObserved {
		return false, nil
	}
	deadline := time.Now().Add(gitDarwinProcessGroupDrainGrace)
	for {
		members, err := unix.SysctlKinfoProcSlice("kern.proc.pgrp", processGroupID)
		if err != nil {
			if !errors.Is(err, syscall.ESRCH) {
				return false, err
			}
			// A vanished process group is absence of evidence, not evidence of
			// cleanup: this classifier is complete only when it observes the
			// pinned leader zombie. Returning true here would assert group
			// termination the caller never proved, so treat ESRCH as an empty
			// member set and let the bounded drain and leader check decide.
			members = nil
		}
		if gitDarwinProcessGroupTerminal(members, processGroupID) {
			return true, nil
		}
		if !terminationSignaled || !time.Now().Before(deadline) {
			return false, nil
		}
		time.Sleep(gitDarwinProcessGroupPoll)
	}
}

func gitDarwinProcessGroupTerminal(members []unix.KinfoProc, processGroupID int) bool {
	leaderFound := false
	for _, member := range members {
		if int(member.Eproc.Pgid) != processGroupID || member.Proc.P_stat != gitDarwinProcessZombie {
			return false
		}
		leaderFound = leaderFound || int(member.Proc.P_pid) == processGroupID
	}
	return leaderFound
}
