// Package validation_test verifies the validation meta-package compiles.
// Actual validators live in sub-packages (engram, scope); this package
// is a doc-only anchor.
package validation_test

import "testing"

func TestPackageCompiles(t *testing.T) {
	t.Parallel()
}
