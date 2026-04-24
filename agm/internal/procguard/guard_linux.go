//go:build linux

package procguard

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ApplyNprocLimit sets RLIMIT_NPROC on a running process identified by pid.
// This uses the prlimit2 syscall to constrain the maximum number of processes
// the target process (and its children) can create.
func ApplyNprocLimit(pid int, limit uint64) error {
	rlim := unix.Rlimit{
		Cur: limit,
		Max: limit,
	}
	// prlimit(pid, RLIMIT_NPROC, &new, nil)
	_, _, errno := syscall.RawSyscall6(
		unix.SYS_PRLIMIT64,
		uintptr(pid),
		uintptr(unix.RLIMIT_NPROC),
		uintptr(unsafe.Pointer(&rlim)),
		0, // don't read old limit
		0,
		0,
	)
	if errno != 0 {
		return fmt.Errorf("prlimit RLIMIT_NPROC on pid %d: %w", pid, errno)
	}
	return nil
}
