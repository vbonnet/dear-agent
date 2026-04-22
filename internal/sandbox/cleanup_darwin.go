//go:build darwin

package sandbox

import (
	"os/exec"
)

// unmountPath unmounts a filesystem on macOS.
func unmountPath(mountPoint string) error {
	// Use umount command on macOS
	cmd := exec.Command("umount", mountPoint)
	err := cmd.Run()
	if err != nil {
		// Try force unmount
		cmd = exec.Command("umount", "-f", mountPoint)
		return cmd.Run()
	}
	return nil
}
