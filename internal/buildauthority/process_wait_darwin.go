//go:build darwin && arm64

package buildauthority

import (
	"math"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	darwinWaitPIDType         = 1 // P_PID from <sys/wait.h>.
	darwinSIGCHLD             = 20
	darwinSIGIgnored          = 1
	darwinSANoChildWait int32 = 0x20

	darwinChildExited = 1 // CLD_EXITED from <sys/signal.h>.
	darwinChildKilled = 2 // CLD_KILLED from <sys/signal.h>.
	darwinChildDumped = 3 // CLD_DUMPED from <sys/signal.h>.

	darwinProcessZombie int8 = 5 // SZOMB from <sys/proc.h>.

	darwinControlKernel = 1  // CTL_KERN from <sys/sysctl.h>.
	darwinKernelProcess = 14 // KERN_PROC from <sys/sysctl.h>.
	darwinProcessGroup  = 2  // KERN_PROC_PGRP from <sys/sysctl.h>.
)

// darwinOldSigaction is the entire Darwin/arm64 old-action result. The build
// fails if any field size, alignment, or offset drifts from the admitted ABI.
type darwinOldSigaction struct {
	handler uintptr
	mask    uint32
	flags   int32
}

var (
	_ [16 - unsafe.Sizeof(darwinOldSigaction{})]byte
	_ [unsafe.Sizeof(darwinOldSigaction{}) - 16]byte
	_ [8 - unsafe.Alignof(darwinOldSigaction{})]byte
	_ [unsafe.Alignof(darwinOldSigaction{}) - 8]byte
	_ [0 - unsafe.Offsetof(darwinOldSigaction{}.handler)]byte
	_ [unsafe.Offsetof(darwinOldSigaction{}.handler) - 0]byte
	_ [8 - unsafe.Offsetof(darwinOldSigaction{}.mask)]byte
	_ [unsafe.Offsetof(darwinOldSigaction{}.mask) - 8]byte
	_ [12 - unsafe.Offsetof(darwinOldSigaction{}.flags)]byte
	_ [unsafe.Offsetof(darwinOldSigaction{}.flags) - 12]byte
)

func (action darwinOldSigaction) admitted() bool {
	return action.handler != darwinSIGIgnored && action.flags&darwinSANoChildWait == 0
}

// darwinSiginfo is the exact Darwin/arm64 siginfo_t result used by waitid.
type darwinSiginfo struct {
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

var (
	_ [104 - unsafe.Sizeof(darwinSiginfo{})]byte
	_ [unsafe.Sizeof(darwinSiginfo{}) - 104]byte
	_ [8 - unsafe.Alignof(darwinSiginfo{})]byte
	_ [unsafe.Alignof(darwinSiginfo{}) - 8]byte
	_ [0 - unsafe.Offsetof(darwinSiginfo{}.signo)]byte
	_ [unsafe.Offsetof(darwinSiginfo{}.signo) - 0]byte
	_ [4 - unsafe.Offsetof(darwinSiginfo{}.errno)]byte
	_ [unsafe.Offsetof(darwinSiginfo{}.errno) - 4]byte
	_ [8 - unsafe.Offsetof(darwinSiginfo{}.code)]byte
	_ [unsafe.Offsetof(darwinSiginfo{}.code) - 8]byte
	_ [12 - unsafe.Offsetof(darwinSiginfo{}.pid)]byte
	_ [unsafe.Offsetof(darwinSiginfo{}.pid) - 12]byte
	_ [16 - unsafe.Offsetof(darwinSiginfo{}.uid)]byte
	_ [unsafe.Offsetof(darwinSiginfo{}.uid) - 16]byte
	_ [20 - unsafe.Offsetof(darwinSiginfo{}.status)]byte
	_ [unsafe.Offsetof(darwinSiginfo{}.status) - 20]byte
	_ [24 - unsafe.Offsetof(darwinSiginfo{}.addr)]byte
	_ [unsafe.Offsetof(darwinSiginfo{}.addr) - 24]byte
	_ [32 - unsafe.Offsetof(darwinSiginfo{}.value)]byte
	_ [unsafe.Offsetof(darwinSiginfo{}.value) - 32]byte
	_ [40 - unsafe.Offsetof(darwinSiginfo{}.band)]byte
	_ [unsafe.Offsetof(darwinSiginfo{}.band) - 40]byte
	_ [48 - unsafe.Offsetof(darwinSiginfo{}.pad)]byte
	_ [unsafe.Offsetof(darwinSiginfo{}.pad) - 48]byte
)

type darwinSnapshotResult struct {
	byteLength uintptr
	overflow   bool
}

// darwinGroupBuffer is allocated once per run. No size query or growing retry
// exists: the one kernel call receives exactly 4,096 KinfoProc rows.
type darwinGroupBuffer struct {
	rows [processGroupMemberLimit]unix.KinfoProc
}

func (result darwinSnapshotResult) count() (int, bool) {
	rowBytes := unsafe.Sizeof(unix.KinfoProc{})
	capacityBytes := unsafe.Sizeof(darwinGroupBuffer{}.rows)
	if result.overflow || result.byteLength > capacityBytes || result.byteLength%rowBytes != 0 {
		return 0, false
	}
	return int(result.byteLength / rowBytes), true
}

func (buffer *darwinGroupBuffer) member(index int) (pid, pgid int32, state int8) {
	member := &buffer.rows[index]
	return member.Proc.P_pid, member.Eproc.Pgid, member.Proc.P_stat
}

func realDarwinSigaction() (darwinOldSigaction, error) {
	var oldAction darwinOldSigaction
	_, _, errno := syscall.RawSyscall(
		syscall.SYS_SIGACTION,
		uintptr(darwinSIGCHLD),
		0,
		uintptr(unsafe.Pointer(&oldAction)),
	)
	runtime.KeepAlive(&oldAction)
	if errno != 0 {
		return darwinOldSigaction{}, errno
	}
	return oldAction, nil
}

func realDarwinWaitID(pid int) (darwinSiginfo, error) {
	// A fresh zero value is passed on every call, including retries by the event
	// owner. The raw adapter never retains a kernel-written record.
	var info darwinSiginfo
	_, _, errno := syscall.Syscall6(
		syscall.SYS_WAITID,
		darwinWaitPIDType,
		uintptr(pid),
		uintptr(unsafe.Pointer(&info)),
		syscall.WEXITED|syscall.WNOHANG|syscall.WNOWAIT,
		0,
		0,
	)
	runtime.KeepAlive(&info)
	if errno != 0 {
		return darwinSiginfo{}, errno
	}
	return info, nil
}

func realDarwinSignalGroup(processGroupID int) error {
	return syscall.Kill(-processGroupID, syscall.SIGKILL)
}

func realDarwinGroupSnapshot(processGroupID int, buffer *darwinGroupBuffer) (darwinSnapshotResult, error) {
	if buffer == nil {
		return darwinSnapshotResult{}, syscall.EFAULT
	}
	if processGroupID <= 0 || processGroupID > math.MaxInt32 {
		return darwinSnapshotResult{}, syscall.EINVAL
	}
	// #nosec G115 -- the bounds check above proves exact int32 representation.
	groupSelector := int32(processGroupID)
	mib := [4]int32{
		darwinControlKernel,
		darwinKernelProcess,
		darwinProcessGroup,
		groupSelector,
	}
	capacityBytes := unsafe.Sizeof(buffer.rows)
	byteLength := capacityBytes
	_, _, errno := syscall.Syscall6(
		syscall.SYS___SYSCTL,
		uintptr(unsafe.Pointer(&mib[0])),
		uintptr(len(mib)),
		uintptr(unsafe.Pointer(&buffer.rows[0])),
		uintptr(unsafe.Pointer(&byteLength)),
		0,
		0,
	)
	runtime.KeepAlive(&mib)
	runtime.KeepAlive(buffer)
	result := darwinSnapshotResult{
		byteLength: byteLength,
		overflow:   byteLength > capacityBytes,
	}
	if errno != 0 {
		return result, errno
	}
	return result, nil
}
