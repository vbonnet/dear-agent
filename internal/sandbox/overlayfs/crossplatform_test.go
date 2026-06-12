// Package overlayfs_test verifies cross-platform compile-time properties.
package overlayfs_test

import (
	"runtime"
	"testing"
)

// TestPlatformComment documents that full overlayfs tests require Linux.
func TestPlatformComment(t *testing.T) {
	// overlayfs only runs on Linux; this test documents that constraint.
	if runtime.GOOS == "linux" {
		t.Log("on Linux: overlayfs tests run via the linux-tagged test files")
	} else {
		t.Logf("on %s: full overlayfs tests require Linux; the other _test.go files are linux-only", runtime.GOOS)
	}
}
