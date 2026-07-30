//go:build darwin

package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	csOpsStatus      = 0
	csValid          = 0x00000001
	csGetTaskAllow   = 0x00000004
	csInvalidAllowed = 0x00000020
	csRuntime        = 0x00010000
	csDebugged       = 0x10000000
)

func validateProcessImage(pid int) error {
	var status uint32
	_, _, errno := unix.Syscall6(
		unix.SYS_CSOPS, //nolint:staticcheck // x/sys/unix exposes no libSystem csops wrapper.
		uintptr(pid),
		csOpsStatus,
		uintptr(unsafe.Pointer(&status)),
		uintptr(unsafe.Sizeof(status)),
		0,
		0,
	)
	runtime.KeepAlive(&status)
	if errno != 0 {
		return errno
	}
	return validateDarwinCodeStatus(status)
}

func validateDarwinCodeStatus(status uint32) error {
	if status&csValid == 0 || status&csRuntime == 0 {
		return errors.New("launcher is not a valid hardened-runtime process")
	}
	if status&(csGetTaskAllow|csInvalidAllowed|csDebugged) != 0 {
		return errors.New("launcher permits injected or debug-modified code")
	}
	return nil
}

func processParentPID(pid int) (int, error) {
	const maxInt32 = int64(1<<31 - 1)
	if pid <= 1 || int64(pid) > maxInt32 {
		return 0, errors.New("process PID is outside the Darwin kernel range")
	}
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return 0, err
	}
	if info == nil || info.Proc.P_pid != int32(pid) {
		return 0, errors.New("process identity changed while inspecting ancestry")
	}
	return int(info.Eproc.Ppid), nil
}

func processExecutablePath(pid int) (string, error) {
	raw, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return "", fmt.Errorf("read process executable: %w", err)
	}
	if len(raw) < 5 {
		return "", errors.New("process executable response is truncated")
	}
	pathBytes := raw[4:]
	end := bytes.IndexByte(pathBytes, 0)
	if end <= 0 {
		return "", errors.New("process executable path is missing")
	}
	return string(pathBytes[:end]), nil
}

func processCodeIdentity(pid int) (string, error) {
	const csOpsCDHash = 5
	const csOpsSyscall = unix.SYS_CSOPS //nolint:staticcheck // x/sys/unix exposes no libSystem csops wrapper.
	var digest [20]byte
	_, _, errno := unix.Syscall6(
		csOpsSyscall,
		uintptr(pid),
		csOpsCDHash,
		uintptr(unsafe.Pointer(&digest[0])),
		uintptr(len(digest)),
		0,
		0,
	)
	runtime.KeepAlive(&digest)
	if errno != 0 {
		return "", errno
	}
	return codeIdentityAlgorithm() + ":" + hex.EncodeToString(digest[:]), nil
}

func codeIdentityAlgorithm() string { return "darwin-cdhash" }
func codeIdentityHexLength() int    { return 40 }
