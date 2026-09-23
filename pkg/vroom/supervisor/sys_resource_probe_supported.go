//go:build linux || darwin

package supervisor

import (
	"context"
	"syscall"
)

// Snapshot implements ResourceProbe on the supported resource-probe platforms.
func (p *SysResourceProbe) Snapshot(ctx context.Context) (ResourceSnapshot, error) {
	diskPath := p.DiskPath
	if diskPath == "" {
		diskPath = "/"
	}

	snap := ResourceSnapshot{}

	// Disk.
	var fs syscall.Statfs_t
	if err := syscall.Statfs(diskPath, &fs); err == nil && fs.Blocks > 0 && fs.Bsize > 0 {
		bsize := uint64(fs.Bsize) //nolint:gosec,nolintlint // Bsize is checked > 0 above; int64→uint64 is safe; directive is platform-conditional
		total := fs.Blocks * bsize
		avail := fs.Bavail * bsize
		if total > 0 {
			used := total - avail
			snap.DiskUsedFraction = float64(used) / float64(total)
		}
		snap.DiskFreeBytes = avail

		// Inodes (ce-6fel): exhaustion fails writes while blocks remain free.
		// Guard Ffree ≤ Files — some filesystems (APFS) report a huge virtual
		// Files total with Ffree tracking it; the fraction still lands in 0..1.
		if fs.Files > 0 && fs.Ffree <= fs.Files {
			snap.InodeUsedFraction = float64(fs.Files-fs.Ffree) / float64(fs.Files)
		}
	}

	// Memory and swap — platform-specific (see sys_resource_probe_{linux,darwin}.go).
	snap.MemoryUsedFraction = sysMemoryUsedFraction()
	snap.FreePhysicalMemoryBytes = sysFreeMemoryBytes()
	snap.SwapUsedFraction = sysSwapUsedFraction()

	// On Darwin, refine MemoryUsedFraction and FreePhysicalMemoryBytes to
	// include inactive (reclaimable) pages from vm_stat(1) — vm.page_inactive_count
	// is not exposed as a sysctl on macOS. On Linux this is a no-op
	// (MemAvailable already accounts for reclaimable cache).
	snap.MemoryUsedFraction, snap.FreePhysicalMemoryBytes = sysCorrectMemoryMetrics(ctx, snap)

	// FD pressure, vnode pressure, and gopls accumulation — platform-specific.
	snap.OpenFDFraction = sysFDUsedFraction()
	snap.VnodeUsedFraction = sysVnodeUsedFraction()
	snap.GoplsProcesses = sysGoplsCount(ctx)

	// macOS kernel memory-pressure level (Darwin only; 0 elsewhere = "unknown").
	snap.MemorystatusLevel = sysMemorystatusLevel()

	return snap, nil
}
