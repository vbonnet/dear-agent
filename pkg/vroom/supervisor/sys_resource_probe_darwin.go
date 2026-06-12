//go:build darwin

package supervisor

import (
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
	total := *(*uint64)(unsafe.Pointer(&raw[0]))  // raw is 32 bytes, offset 0 is safe
	used := *(*uint64)(unsafe.Pointer(&raw[16])) // raw is 32 bytes, offset 16 is safe
	if total == 0 {
		return 0
	}
	if used >= total {
		return 1
	}
	return float64(used) / float64(total)
}
