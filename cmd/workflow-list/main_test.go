package main

import "testing"

// TestMain_PackageCompiles verifies the package builds. All workflow logic
// lives in pkg/workflow; this file satisfies the zero-test ratchet check.
func TestMain_PackageCompiles(t *testing.T) {
	t.Parallel()
}
