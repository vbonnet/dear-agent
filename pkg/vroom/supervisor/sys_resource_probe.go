package supervisor

import (
	"context"
	"syscall"
)

// SysResourceProbe implements ResourceProbe by reading real OS metrics.
// It reports disk usage for a configurable path (default "/") and memory
// usage via platform-specific syscalls. CPU usage is always reported as 0
// (requires a sampling window — reserved for a follow-up).
//
// Disk stats use syscall.Statfs, which is available on Linux and Darwin.
// Memory stats use platform-specific helpers in sys_resource_probe_{linux,darwin}.go.
// On other platforms both fields return 0 and no error.
type SysResourceProbe struct {
	// DiskPath is the filesystem path to measure disk usage for.
	// Defaults to "/" when empty.
	DiskPath string
}

// NewSysResourceProbe returns a probe that reads real OS metrics.
func NewSysResourceProbe() *SysResourceProbe {
	return &SysResourceProbe{DiskPath: "/"}
}

// Snapshot implements ResourceProbe.
func (p *SysResourceProbe) Snapshot(_ context.Context) (ResourceSnapshot, error) {
	diskPath := p.DiskPath
	if diskPath == "" {
		diskPath = "/"
	}

	snap := ResourceSnapshot{}

	// Disk.
	var fs syscall.Statfs_t
	if err := syscall.Statfs(diskPath, &fs); err == nil && fs.Blocks > 0 {
		total := fs.Blocks * uint64(fs.Bsize)
		avail := fs.Bavail * uint64(fs.Bsize)
		if total > 0 {
			used := total - avail
			snap.DiskUsedFraction = float64(used) / float64(total)
		}
	}

	// Memory — platform-specific (see sys_resource_probe_{linux,darwin}.go).
	snap.MemoryUsedFraction = sysMemoryUsedFraction()

	return snap, nil
}
