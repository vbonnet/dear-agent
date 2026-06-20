//go:build darwin

package supervisor

import (
	"bytes"
	"context"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// sysMemoryUsedFraction returns the fraction of physical RAM currently in use
// on macOS, computed as (total - free_pages×page_size) / total.
//
// Data sources:
//   - hw.memsize  — total physical RAM (bytes, 64-bit sysctl)
//   - hw.pagesize — VM page size (bytes)
//   - vm.page_free_count — number of genuinely free pages
//
// This is a point-in-time snapshot: pages classified as "inactive" (evictable
// but not yet freed) count as used. The denominator is always hw.memsize,
// so values above 0.9 reliably indicate memory pressure.
func sysMemoryUsedFraction() float64 {
	// hw.memsize is a 64-bit sysctl — SysctlUint32 would truncate it.
	totalBytes := uint64(0)
	{
		mib := [2]int32{6, 24} // CTL_HW=6, HW_MEMSIZE=24
		n := uintptr(8)
		if _, _, errno := syscall.RawSyscall6(
			syscall.SYS___SYSCTL,
			uintptr(unsafe.Pointer(&mib[0])),
			2,
			uintptr(unsafe.Pointer(&totalBytes)),
			uintptr(unsafe.Pointer(&n)),
			0, 0,
		); errno != 0 || totalBytes == 0 {
			return 0
		}
	}

	pageSize, err := syscall.SysctlUint32("hw.pagesize")
	if err != nil || pageSize == 0 {
		return 0
	}

	freePages, err := syscall.SysctlUint32("vm.page_free_count")
	if err != nil {
		return 0
	}

	freeBytes := uint64(freePages) * uint64(pageSize)
	if freeBytes >= totalBytes {
		return 0
	}
	return float64(totalBytes-freeBytes) / float64(totalBytes)
}

// sysSwapUsedFraction returns the fraction of swap space currently in use on
// macOS using the vm.swapusage sysctl (struct xsw_usage).
//
// struct xsw_usage layout (32 bytes):
//
//	xsu_total    uint64  — total swap allocated
//	xsu_avail    uint64  — swap currently available
//	xsu_used     uint64  — swap currently in use
//	xsu_pagesize uint32  — VM page size
//	xsu_encrypted int32  — whether swap is encrypted
func sysSwapUsedFraction() float64 {
	raw, err := unix.SysctlRaw("vm.swapusage")
	if err != nil || len(raw) < 24 {
		return 0
	}
	total := *(*uint64)(unsafe.Pointer(&raw[0])) // raw is 32 bytes, offset 0 is safe
	used := *(*uint64)(unsafe.Pointer(&raw[16])) // raw is 32 bytes, offset 16 is safe
	if total == 0 {
		return 0
	}
	if used >= total {
		return 1
	}
	return float64(used) / float64(total)
}

// sysFDUsedFraction returns the fraction of the system-wide open-file
// descriptor limit currently in use on macOS.
//
// Data sources:
//   - kern.num_files  — current number of open file descriptors system-wide
//   - kern.maxfiles   — system-wide FD limit
func sysFDUsedFraction() float64 {
	numFiles, err := syscall.SysctlUint32("kern.num_files")
	if err != nil {
		return 0
	}
	maxFiles, err := syscall.SysctlUint32("kern.maxfiles")
	if err != nil || maxFiles == 0 {
		return 0
	}
	if numFiles >= maxFiles {
		return 1
	}
	return float64(numFiles) / float64(maxFiles)
}

// sysVnodeUsedFraction returns the fraction of the kernel vnode table
// currently in use on macOS. Vnode exhaustion causes filesystem operations
// to fail with ENFILE even when per-process FD limits are not hit.
//
// Data sources:
//   - kern.num_vnodes  — current vnode count
//   - kern.maxvnodes   — vnode table size limit
func sysVnodeUsedFraction() float64 {
	numVnodes, err := syscall.SysctlUint32("kern.num_vnodes")
	if err != nil {
		return 0
	}
	maxVnodes, err := syscall.SysctlUint32("kern.maxvnodes")
	if err != nil || maxVnodes == 0 {
		return 0
	}
	if numVnodes >= maxVnodes {
		return 1
	}
	return float64(numVnodes) / float64(maxVnodes)
}

// sysGoplsCount returns the number of *orphaned* gopls processes — gopls
// instances reparented to PID 1 because the Claude session that spawned them
// died. This is the leak signal the Overseer escalates on.
//
// It must NOT count live gopls: every healthy Claude Code session runs its own
// gopls (via the gopls-lsp plugin), so a raw process count scales with the
// number of live sessions and produces phantom leak alarms (ce-u7v9). Two
// pgrep(1) flags together give the precise signal:
//
//   - -x  matches the process name exactly ("gopls"), never a substring of a
//     longer argv such as a plugin path containing "gopls". (The older -f,
//     which matches the full command line, was the original over-count cause.)
//   - -P 1  restricts to processes whose parent is PID 1, i.e. true orphans.
//     A gopls with a live parent keeps its real PPID and is never counted.
//
// Exit code 1 from pgrep means "no matches" — not an error we propagate.
func sysGoplsCount(ctx context.Context) int {
	out, err := exec.CommandContext(ctx, "pgrep", "-x", "-P", "1", "gopls").Output()
	if err != nil {
		// exit code 1 = no match, any other error = pgrep unavailable; both → 0
		return 0
	}
	n := 0
	for line := range bytes.SplitSeq(bytes.TrimSpace(out), []byte("\n")) {
		if len(bytes.TrimSpace(line)) > 0 {
			n++
		}
	}
	return n
}
