//go:build darwin

package supervisor

import (
	"syscall"
	"unsafe"
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
