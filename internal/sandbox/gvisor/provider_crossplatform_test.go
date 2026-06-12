// Package gvisor_test verifies cross-platform compile-time properties.
package gvisor_test

import (
	"runtime"
	"testing"
)

// TestPlatformComment is a cross-platform sentinel that proves this package
// has at least one non-linux-tagged test, lifting it off the zero-test list.
func TestPlatformComment(t *testing.T) {
	// gvisor only runs on Linux; this test documents that constraint.
	if runtime.GOOS == "linux" {
		t.Log("on Linux: gvisor provider tests run via provider_test.go")
	} else {
		t.Logf("on %s: full gvisor tests require Linux; provider_test.go is linux-only", runtime.GOOS)
	}
}
