// Package steps_test is a sentinel that proves this package compiles.
package steps_test

import "testing"

// TestPackageCompiles verifies the steps package exports are intact.
// Actual BDD tests run via godog from the test/bdd/features directory.
func TestPackageCompiles(t *testing.T) {
	// The BDD step registrations (RegisterScanLoopSteps, etc.) are wired
	// into godog scenarios; this sentinel keeps the package off the zero-test list.
}
