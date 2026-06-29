//go:build darwin

package supervisor

import (
	"bytes"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// sysMemoryUsedFraction returns the fraction of physical RAM currently in use
// on macOS, computed as (total - free_pages×page_size) / total.
//
// Data sources:
//   - hw.memsize         — total physical RAM (bytes, 64-bit sysctl)
//   - hw.pagesize        — VM page size (bytes)
//   - vm.page_free_count — genuinely free pages (not yet allocated)
//
// This is a conservative baseline: inactive (reclaimable) pages are counted as
// used here. sysCorrectMemoryMetrics() refines both this value and
// sysFreeMemoryBytes() by adding inactive pages obtained from vm_stat(1), since
// vm.page_inactive_count is not exposed as a sysctl on macOS.
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

// sysFreeMemoryBytes returns the number of genuinely free physical RAM bytes
// on macOS (vm.page_free_count × hw.pagesize). This is the conservative floor;
// sysCorrectMemoryMetrics() adds inactive (reclaimable) pages obtained from
// vm_stat(1) to report the true available headroom. Returns 0 if either sysctl
// is unavailable (treated as "unknown" by the monitor, never as "0 bytes free").
func sysFreeMemoryBytes() uint64 {
	pageSize, err := syscall.SysctlUint32("hw.pagesize")
	if err != nil || pageSize == 0 {
		return 0
	}
	freePages, err := syscall.SysctlUint32("vm.page_free_count")
	if err != nil {
		return 0
	}
	return uint64(freePages) * uint64(pageSize)
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
//
// macOS operates the vnode table as a fixed-size LRU cache: num_vnodes equals
// maxvnodes in normal steady state because the kernel continuously fills and
// recycles it. A ratio of 1.0 is therefore the expected warm-cache baseline,
// not an alarm signal. This function returns 0 when num_vnodes >= maxvnodes
// to suppress chronic false-positive escalations; real vnode exhaustion (where
// the kernel cannot recycle fast enough) surfaces as ENFILE/EMFILE errors in
// gopls and build tools, which the gopls-process and FD-fraction metrics catch.
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
		// LRU warm-cache steady state — not a pressure signal on Darwin.
		return 0
	}
	return float64(numVnodes) / float64(maxVnodes)
}

// sysCorrectMemoryMetrics refines the sysctl-derived MemoryUsedFraction and
// FreePhysicalMemoryBytes by adding the inactive-page count obtained from
// vm_stat(1). Inactive pages are reclaimable LRU cache — they are not "free"
// but they are available without going to swap. Without this correction, both
// metrics read as near-100% / near-zero on any machine with a warm VM cache,
// producing chronic false-positive pressure escalations.
//
// vm.page_inactive_count is not exposed as a sysctl on macOS (the key returns
// ENOENT), so vm_stat(1) is the only available data source. The subprocess is
// fast (< 5ms) and runs once per probe tick (typically every 30+ seconds). On
// any error, the original sysctl-derived values are returned unchanged.
func sysCorrectMemoryMetrics(ctx context.Context, snap ResourceSnapshot) (memFraction float64, freeBytes uint64) {
	out, err := exec.CommandContext(ctx, "vm_stat").Output()
	if ctx.Err() != nil {
		return snap.MemoryUsedFraction, snap.FreePhysicalMemoryBytes
	}
	if err != nil {
		return snap.MemoryUsedFraction, snap.FreePhysicalMemoryBytes
	}

	var inactivePages uint64
	for line := range strings.SplitSeq(string(out), "\n") {
		key, val, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok || strings.TrimSpace(key) != "Pages inactive" {
			continue
		}
		// val looks like "  396211."
		s := strings.TrimRight(strings.TrimSpace(val), ".")
		n, parseErr := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
		if parseErr == nil {
			inactivePages = n
		}
		break
	}

	if inactivePages == 0 {
		return snap.MemoryUsedFraction, snap.FreePhysicalMemoryBytes
	}

	pageSize, err := syscall.SysctlUint32("hw.pagesize")
	if err != nil || pageSize == 0 {
		return snap.MemoryUsedFraction, snap.FreePhysicalMemoryBytes
	}

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
		); errno != 0 {
			return snap.MemoryUsedFraction, snap.FreePhysicalMemoryBytes
		}
	}
	if totalBytes == 0 {
		return snap.MemoryUsedFraction, snap.FreePhysicalMemoryBytes
	}

	inactiveBytes := inactivePages * uint64(pageSize)
	newFreeBytes := snap.FreePhysicalMemoryBytes + inactiveBytes
	if newFreeBytes >= totalBytes {
		return 0, newFreeBytes
	}
	return float64(totalBytes-newFreeBytes) / float64(totalBytes), newFreeBytes
}

// sysMemorystatusLevel returns the macOS kernel memory-pressure level from
// kern.memorystatus_level (0–100). The kernel reports 100 when memory is
// plentiful and lowers the value as pressure rises; values below 40 indicate
// high pressure (the kernel begins aggressive eviction and may OOM-kill
// processes). Returns 0 when the sysctl is unavailable (treated as "unknown").
func sysMemorystatusLevel() int {
	v, err := syscall.SysctlUint32("kern.memorystatus_level")
	if err != nil {
		return 0
	}
	return int(v)
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
