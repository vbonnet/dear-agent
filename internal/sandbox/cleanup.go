package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// OrphanedResource represents a resource that should be cleaned up.
type OrphanedResource struct {
	Path         string
	Type         string // "mount" or "directory"
	LastModified time.Time
	Size         int64
}

// CleanupStats tracks cleanup operation results.
type CleanupStats struct {
	MountsDetected  int
	MountsCleanedUp int
	DirsDetected    int
	DirsCleanedUp   int
	Errors          []error
	TotalBytesFreed int64
}

func orphanCleanupPlatformError(goos string) error {
	switch goos {
	case "darwin", "linux":
		return nil
	default:
		return WithContext(
			NewError(ErrCodeUnsupportedPlatform,
				fmt.Sprintf("sandbox orphan cleanup is unsupported on %s", goos)),
			"goos", goos,
		)
	}
}

// DetectOrphanedMounts finds overlay mounts in /proc/mounts that might be orphaned.
// Returns mount points that contain the sandbox base directory path.
func DetectOrphanedMounts(sandboxBaseDir string) ([]OrphanedResource, error) {
	// Read /proc/mounts
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		// Not a Linux system or can't read mounts - this is expected on macOS
		return nil, nil //nolint:nilerr // Non-Linux systems don't have /proc/mounts
	}

	var orphaned []OrphanedResource
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		fsType := fields[2]
		mountPoint := fields[1]

		// Look for overlay mounts in our sandbox directory
		if fsType == "overlay" && strings.HasPrefix(mountPoint, sandboxBaseDir) {
			// Check if directory still exists
			info, err := os.Stat(mountPoint)
			if err != nil {
				continue
			}

			orphaned = append(orphaned, OrphanedResource{
				Path:         mountPoint,
				Type:         "mount",
				LastModified: info.ModTime(),
			})
		}
	}

	return orphaned, nil
}

// DetectOrphanedDirectories finds leftover sandbox directories.
// Looks for directories matching the sandbox pattern that are old.
func DetectOrphanedDirectories(sandboxBaseDir string, olderThan time.Duration) ([]OrphanedResource, error) {
	// Check if base directory exists
	if _, err := os.Stat(sandboxBaseDir); os.IsNotExist(err) {
		return nil, nil
	}

	var orphaned []OrphanedResource
	cutoff := time.Now().Add(-olderThan)

	// Walk the sandbox base directory
	entries, err := os.ReadDir(sandboxBaseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read sandbox directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirPath := filepath.Join(sandboxBaseDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Check if directory is old enough
		if info.ModTime().Before(cutoff) {
			// Calculate directory size
			size := calculateDirSize(dirPath)

			orphaned = append(orphaned, OrphanedResource{
				Path:         dirPath,
				Type:         "directory",
				LastModified: info.ModTime(),
				Size:         size,
			})
		}
	}

	return orphaned, nil
}

// CleanupOrphanedMounts attempts to unmount orphaned overlay mounts.
func CleanupOrphanedMounts(mounts []OrphanedResource) CleanupStats {
	return cleanupOrphanedMounts(mounts, orphanCleanupPlatformError(runtime.GOOS))
}

func cleanupOrphanedMounts(mounts []OrphanedResource, platformErr error) CleanupStats {
	stats := CleanupStats{
		MountsDetected: len(mounts),
	}
	if platformErr != nil {
		stats.Errors = append(stats.Errors, platformErr)
		return stats
	}

	for _, mount := range mounts {
		if mount.Type != "mount" {
			continue
		}

		// Try to unmount with retries
		err := unmountWithRetry(mount.Path, 3)
		if err != nil {
			stats.Errors = append(stats.Errors, fmt.Errorf("failed to unmount %s: %w", mount.Path, err))
			continue
		}

		stats.MountsCleanedUp++
	}

	return stats
}

// CleanupOrphanedDirectories attempts to remove orphaned directories.
func CleanupOrphanedDirectories(dirs []OrphanedResource) CleanupStats {
	return cleanupOrphanedDirectories(dirs, orphanCleanupPlatformError(runtime.GOOS))
}

func cleanupOrphanedDirectories(dirs []OrphanedResource, platformErr error) CleanupStats {
	stats := CleanupStats{
		DirsDetected: len(dirs),
	}
	if platformErr != nil {
		stats.Errors = append(stats.Errors, platformErr)
		return stats
	}

	for _, dir := range dirs {
		if dir.Type != "directory" {
			continue
		}

		// Try to remove with retries
		err := removeWithRetry(dir.Path)
		if err != nil {
			stats.Errors = append(stats.Errors, fmt.Errorf("failed to remove %s: %w", dir.Path, err))
			continue
		}

		stats.DirsCleanedUp++
		stats.TotalBytesFreed += dir.Size
	}

	return stats
}

// CleanupOrphaned performs full orphaned resource cleanup.
// Returns statistics about what was cleaned up.
func CleanupOrphaned(sandboxBaseDir string, olderThan time.Duration) (CleanupStats, error) {
	return cleanupOrphaned(
		sandboxBaseDir,
		olderThan,
		orphanCleanupPlatformError(runtime.GOOS),
	)
}

func cleanupOrphaned(
	sandboxBaseDir string,
	olderThan time.Duration,
	platformErr error,
) (CleanupStats, error) {
	var combined CleanupStats
	if platformErr != nil {
		return combined, platformErr
	}

	// Step 1: Detect and cleanup orphaned mounts
	mounts, err := DetectOrphanedMounts(sandboxBaseDir)
	if err != nil {
		return combined, fmt.Errorf("failed to detect orphaned mounts: %w", err)
	}

	mountStats := cleanupOrphanedMounts(mounts, nil)
	combined.MountsDetected = mountStats.MountsDetected
	combined.MountsCleanedUp = mountStats.MountsCleanedUp
	combined.Errors = append(combined.Errors, mountStats.Errors...)

	// Step 2: Detect and cleanup orphaned directories
	dirs, err := DetectOrphanedDirectories(sandboxBaseDir, olderThan)
	if err != nil {
		return combined, fmt.Errorf("failed to detect orphaned directories: %w", err)
	}

	dirStats := cleanupOrphanedDirectories(dirs, nil)
	combined.DirsDetected = dirStats.DirsDetected
	combined.DirsCleanedUp = dirStats.DirsCleanedUp
	combined.TotalBytesFreed = dirStats.TotalBytesFreed
	combined.Errors = append(combined.Errors, dirStats.Errors...)

	return combined, nil
}

// unmountWithRetry attempts to unmount with exponential backoff.
func unmountWithRetry(mountPoint string, maxRetries int) error {
	const retryDelay = 100 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryDelay * time.Duration(attempt))
		}

		// Try unmount using the exec package (cross-platform)
		// This will use the appropriate unmount command for the OS
		err := unmountPath(mountPoint)
		if err == nil {
			return nil
		}

		// If not mounted, that's OK
		if isNotMountedError(err) {
			return nil
		}

		// Last attempt, return error
		if attempt == maxRetries-1 {
			return err
		}
	}

	return fmt.Errorf("unmount failed after %d retries", maxRetries)
}

// removeWithRetry attempts to remove a directory with exponential backoff.
func removeWithRetry(dirPath string) error {
	const (
		maxRetries = 3
		retryDelay = 50 * time.Millisecond
	)

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryDelay * time.Duration(attempt))
		}

		err := os.RemoveAll(dirPath)
		if err == nil {
			return nil
		}

		// If doesn't exist, that's OK
		if os.IsNotExist(err) {
			return nil
		}

		// Last attempt, return error
		if attempt == maxRetries-1 {
			return err
		}
	}

	return fmt.Errorf("remove failed after %d retries", maxRetries)
}

// calculateDirSize calculates the total size of a directory.
func calculateDirSize(dirPath string) int64 {
	var size int64

	_ = filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // Skip inaccessible files during disk usage calculation
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	return size
}

// isNotMountedError checks if an error indicates the path is not mounted.
func isNotMountedError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "not mounted") ||
		strings.Contains(errStr, "not a mount point") ||
		strings.Contains(errStr, "EINVAL")
}
