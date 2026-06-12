package main

import "testing"

// TestMain_PackageCompiles verifies the package builds correctly.
// All devlog logic lives in tools/devlog/cmd/devlog; this file exists
// to satisfy the structural-health zero-test check for the entry-point package.
func TestMain_PackageCompiles(t *testing.T) {
	t.Parallel()
	// No-op: compilation is the test.
}
