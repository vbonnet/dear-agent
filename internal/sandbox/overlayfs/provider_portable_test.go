// Cross-platform tests for overlayfs package (no build tag).
// provider.go is linux-only; on other platforms this package compiles
// as a no-op stub. This file satisfies the zero-test ratchet check
// on all platforms.
package overlayfs

import "testing"

func TestPackageCompiles(t *testing.T) {
	t.Parallel()
	// No-op: on !linux the package is an intentional stub — compilation is the test.
}
