package ops

import (
	"os"
	"syscall"
)

// unmountBestEffort attempts to unmount the given path.
// Returns nil if the path is not mounted or unmount succeeds.
func unmountBestEffort(path string) error {
	// Skip if path doesn't exist
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	err := syscall.Unmount(path, 0)
	if err == nil || err == syscall.EINVAL || err == syscall.ENOENT {
		return nil
	}
	return err
}
