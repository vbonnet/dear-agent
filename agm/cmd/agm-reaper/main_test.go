package main

import "testing"

// TestMain_PackageCompiles verifies the package builds. All reaper logic
// lives in agm/internal/reaper; this file satisfies the zero-test ratchet check.
func TestMain_PackageCompiles(t *testing.T) {
	t.Parallel()
}
