//go:build linux

package sandbox

import (
	"syscall"
)

// unmountPath unmounts a filesystem on Linux.
func unmountPath(mountPoint string) error {
	// Try graceful unmount first
	err := syscall.Unmount(mountPoint, 0)
	if err == nil {
		return nil
	}

	// If already unmounted, that's OK
	if err == syscall.EINVAL {
		return nil
	}

	// Try force unmount
	err = syscall.Unmount(mountPoint, syscall.MNT_FORCE)
	if err == syscall.EINVAL {
		return nil
	}

	return err
}
