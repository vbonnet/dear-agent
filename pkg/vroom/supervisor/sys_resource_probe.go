package supervisor

// SysResourceProbe implements ResourceProbe by reading real OS metrics.
// It reports disk usage for a configurable path (default "/") and memory
// usage via platform-specific syscalls. CPU usage is always reported as 0
// (requires a sampling window — reserved for a follow-up).
//
// Disk stats use syscall.Statfs on Linux and Darwin.
// Memory stats use platform-specific helpers in sys_resource_probe_{linux,darwin}.go.
// FD/vnode pressure and gopls process counts are also platform-specific.
// Other platforms return an explicit unsupported error before sampling.
type SysResourceProbe struct {
	// DiskPath is the filesystem path to measure disk usage for.
	// Defaults to "/" when empty.
	DiskPath string
}

// NewSysResourceProbe returns a probe that reads real OS metrics.
func NewSysResourceProbe() *SysResourceProbe {
	return &SysResourceProbe{DiskPath: "/"}
}
