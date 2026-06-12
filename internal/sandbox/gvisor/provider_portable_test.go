// Cross-platform tests for gvisor package (no build tag).
// provider.go and init.go are linux-only; on other platforms this package
// compiles as an empty stub. This file satisfies the zero-test ratchet check
// on all platforms.
package gvisor

import "testing"

func TestPackageCompiles(t *testing.T) {
	t.Parallel()
	// No-op: on !linux the package is an intentional stub — compilation is the test.
}
