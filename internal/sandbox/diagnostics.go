package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// SystemCapabilities represents system sandbox capabilities.
type SystemCapabilities struct {
	Platform       string
	KernelVersion  string
	OverlayFSAvail bool
	APFSAvail      bool
	RootlessMount  bool
	MaxMounts      int
	MaxFDs         int
	Warnings       []string
}

// SandboxHealth represents the health status of a sandbox.
type SandboxHealth struct {
	ID              string
	Exists          bool
	MountActive     bool
	PathsValid      bool
	DiskUsage       int64
	FileCount       int
	Issues          []string
	Recommendations []string
}

// ResourceUsage represents current system resource usage.
type ResourceUsage struct {
	ActiveMounts   int
	OpenFDs        int
	DiskSpaceFree  int64
	DiskSpaceTotal int64
	MemoryFree     int64
	MemoryTotal    int64
	Warnings       []string
}

// DiagnosticReport combines all diagnostic information.
type DiagnosticReport struct {
	Timestamp    string
	Capabilities SystemCapabilities
	Resources    ResourceUsage
	Sandboxes    []SandboxHealth
	Summary      string
}

// DiagnoseSystem checks system capabilities for sandbox support.
func DiagnoseSystem() (*SystemCapabilities, error) {
	caps := &SystemCapabilities{
		Platform: runtime.GOOS,
		Warnings: make([]string, 0),
	}

	// Check kernel version (Linux only)
	if runtime.GOOS == "linux" {
		version, err := getKernelVersion()
		if err != nil {
			caps.Warnings = append(caps.Warnings, fmt.Sprintf("failed to read kernel version: %v", err))
		} else {
			caps.KernelVersion = version
			// Check for rootless mount support (5.11+)
			if isKernelAtLeast(version, 5, 11) {
				caps.RootlessMount = true
			} else {
				caps.Warnings = append(caps.Warnings, fmt.Sprintf("kernel %s is too old for rootless mounts (need 5.11+)", version))
			}
		}

		// Check OverlayFS availability
		caps.OverlayFSAvail = checkOverlayFSSupport()
		if !caps.OverlayFSAvail {
			caps.Warnings = append(caps.Warnings, "OverlayFS not available (check kernel modules)")
		}
	}

	// Check APFS availability (macOS only)
	if runtime.GOOS == "darwin" {
		caps.APFSAvail = checkAPFSSupport()
		if !caps.APFSAvail {
			caps.Warnings = append(caps.Warnings, "APFS reflink cloning not available")
		}
	}

	// Check resource limits
	var rlimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit); err == nil {
		caps.MaxFDs = int(rlimit.Cur) //nolint:gosec // Resource limits won't overflow int
		if caps.MaxFDs < 1024 {
			caps.Warnings = append(caps.Warnings, fmt.Sprintf("low file descriptor limit: %d (recommended: 4096+)", caps.MaxFDs))
		}
	}

	// Estimate max mounts (Linux only)
	if runtime.GOOS == "linux" {
		maxMounts, err := getMaxMounts()
		if err == nil {
			caps.MaxMounts = maxMounts
			if maxMounts < 1000 {
				caps.Warnings = append(caps.Warnings, fmt.Sprintf("low mount limit: %d", maxMounts))
			}
		}
	}

	return caps, nil
}

// DiagnoseSandbox checks the health of a specific sandbox.
func DiagnoseSandbox(sandbox *Sandbox) (*SandboxHealth, error) {
	health := &SandboxHealth{
		ID:              sandbox.ID,
		Issues:          make([]string, 0),
		Recommendations: make([]string, 0),
	}

	// Check if paths exist
	if _, err := os.Stat(sandbox.MergedPath); err != nil {
		health.Issues = append(health.Issues, fmt.Sprintf("merged path missing: %s", sandbox.MergedPath))
		health.PathsValid = false
	} else {
		health.PathsValid = true
	}

	if sandbox.UpperPath != "" {
		if _, err := os.Stat(sandbox.UpperPath); err != nil {
			health.Issues = append(health.Issues, fmt.Sprintf("upper path missing: %s", sandbox.UpperPath))
			health.PathsValid = false
		}
	}

	// Check if mount is active (Linux only)
	if runtime.GOOS == "linux" {
		active, err := isMountActive(sandbox.MergedPath)
		if err != nil {
			health.Issues = append(health.Issues, fmt.Sprintf("failed to check mount status: %v", err))
		} else {
			health.MountActive = active
			if !active && sandbox.Type == "overlayfs-native" {
				health.Issues = append(health.Issues, "OverlayFS mount not active")
				health.Recommendations = append(health.Recommendations, "Recreate sandbox or check for orphaned mounts")
			}
		}
	}

	// Calculate disk usage
	if health.PathsValid {
		usage, count, err := calculateDiskUsage(sandbox.UpperPath)
		if err == nil {
			health.DiskUsage = usage
			health.FileCount = count
			if usage > 1024*1024*1024 { // > 1GB
				health.Recommendations = append(health.Recommendations, fmt.Sprintf("large disk usage: %d MB, consider cleanup", usage/(1024*1024)))
			}
		}
	}

	health.Exists = len(health.Issues) == 0

	return health, nil
}

// DiagnoseResources checks current system resource usage.
func DiagnoseResources() (*ResourceUsage, error) {
	resources := &ResourceUsage{
		Warnings: make([]string, 0),
	}

	// Count active mounts (Linux only)
	if runtime.GOOS == "linux" {
		mounts, err := countActiveMounts()
		if err == nil {
			resources.ActiveMounts = mounts
			if mounts > 1000 {
				resources.Warnings = append(resources.Warnings, fmt.Sprintf("high mount count: %d", mounts))
			}
		}
	}

	// Count open file descriptors
	fds := countOpenFDs()
	if fds > 0 {
		resources.OpenFDs = fds
		var rlimit syscall.Rlimit
		if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit); err == nil {
			limit := int(rlimit.Cur) //nolint:gosec // Resource limits won't overflow int
			usage := float64(fds) / float64(limit) * 100
			if usage > 80 {
				resources.Warnings = append(resources.Warnings, fmt.Sprintf("high FD usage: %d/%d (%.1f%%)", fds, limit, usage))
			}
		}
	}

	// Check disk space
	wd, err := os.Getwd()
	if err == nil {
		free, total, err := getDiskSpace(wd)
		if err == nil {
			resources.DiskSpaceFree = free
			resources.DiskSpaceTotal = total
			usagePct := float64(total-free) / float64(total) * 100
			if usagePct > 90 {
				resources.Warnings = append(resources.Warnings, fmt.Sprintf("low disk space: %.1f%% used", usagePct))
			}
		}
	}

	return resources, nil
}

// GenerateDiagnosticReport creates a comprehensive diagnostic report.
func GenerateDiagnosticReport(sandboxes []*Sandbox) (*DiagnosticReport, error) {
	report := &DiagnosticReport{
		Timestamp: fmt.Sprintf("%d", time.Now().Unix()),
		Sandboxes: make([]SandboxHealth, 0),
	}

	// Diagnose system
	caps, err := DiagnoseSystem()
	if err != nil {
		return nil, fmt.Errorf("failed to diagnose system: %w", err)
	}
	report.Capabilities = *caps

	// Diagnose resources
	resources, err := DiagnoseResources()
	if err != nil {
		return nil, fmt.Errorf("failed to diagnose resources: %w", err)
	}
	report.Resources = *resources

	// Diagnose each sandbox
	for _, sb := range sandboxes {
		health, err := DiagnoseSandbox(sb)
		if err != nil {
			continue // Skip failed diagnostics
		}
		report.Sandboxes = append(report.Sandboxes, *health)
	}

	// Generate summary
	var summary strings.Builder
	fmt.Fprintf(&summary, "Platform: %s\n", caps.Platform)
	if caps.KernelVersion != "" {
		fmt.Fprintf(&summary, "Kernel: %s\n", caps.KernelVersion)
	}
	fmt.Fprintf(&summary, "Sandboxes: %d active\n", len(report.Sandboxes))
	fmt.Fprintf(&summary, "Warnings: %d system, %d resource\n",
		len(caps.Warnings), len(resources.Warnings))
	report.Summary = summary.String()

	return report, nil
}

// Helper functions

func getKernelVersion() (string, error) {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return "", err
	}
	parts := strings.Fields(string(data))
	for i, part := range parts {
		if part == "version" && i+1 < len(parts) {
			return strings.TrimRight(parts[i+1], "+-"), nil
		}
	}
	return "unknown", nil
}

func isKernelAtLeast(version string, major, minor int) bool {
	var vMajor, vMinor int
	_, err := fmt.Sscanf(version, "%d.%d", &vMajor, &vMinor)
	if err != nil {
		return false
	}
	return vMajor > major || (vMajor == major && vMinor >= minor)
}

func checkOverlayFSSupport() bool {
	// Check if overlay module is loaded or available
	data, err := os.ReadFile("/proc/filesystems")
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "overlay")
}

func checkAPFSSupport() bool {
	// Check if running on APFS by testing current directory
	cmd := exec.Command("df", "-T", ".")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), "apfs")
}

func getMaxMounts() (int, error) {
	data, err := os.ReadFile("/proc/sys/fs/mount-max")
	if err != nil {
		// Default if not available (non-Linux or old kernel)
		return 100000, nil //nolint:nilerr // Return reasonable default on non-Linux systems
	}
	var maxMounts int
	_, err = fmt.Sscanf(string(data), "%d", &maxMounts)
	if err != nil {
		return 0, err
	}
	return maxMounts, nil
}

func isMountActive(path string) (bool, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false, err
	}
	return strings.Contains(string(data), path), nil
}

func countActiveMounts() (int, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return 0, err
	}
	return len(strings.Split(string(data), "\n")) - 1, nil
}

func countOpenFDs() int {
	// Count files in /proc/self/fd
	fds, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		// Fallback: not available on all systems
		return 0
	}
	return len(fds)
}

func getDiskSpace(path string) (free, total int64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	free = int64(stat.Bavail) * int64(stat.Bsize)  //nolint:gosec // Disk sizes won't overflow int64
	total = int64(stat.Blocks) * int64(stat.Bsize) //nolint:gosec // Disk sizes won't overflow int64
	return free, total, nil
}

func calculateDiskUsage(path string) (usage int64, fileCount int, err error) {
	if path == "" {
		return 0, 0, nil
	}
	err = filepath.Walk(path, func(_ string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil //nolint:nilerr // Skip inaccessible files during disk usage calculation
		}
		if !info.IsDir() {
			usage += info.Size()
			fileCount++
		}
		return nil
	})
	return usage, fileCount, err
}
