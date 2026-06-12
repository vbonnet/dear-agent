// Package validation is a doc-only umbrella; its sub-packages hold the logic.
// This test keeps the package off the zero-test list.
package validation

import "testing"

func TestPackageDoc(t *testing.T) {
	// pkg/validation is an umbrella with no logic of its own.
	// Real validators live in pkg/validation/engram and pkg/validation/scope.
}
