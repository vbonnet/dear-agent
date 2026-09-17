//go:build !linux && !darwin

package sandbox

import "runtime"

// unmountPath refuses mount cleanup on unsupported platforms.
func unmountPath(_ string) error {
	return orphanCleanupPlatformError(runtime.GOOS)
}
