package helpers

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

var updateFlag = false // Set via -update flag

// CompareGolden compares actual output to golden file.
//
// In normal mode, reads the golden file and compares it to actual output.
// If they don't match, fails the test with a detailed error message.
//
// In update mode (enabled via SetUpdateFlag), writes actual output as the
// new golden file. Use this when updating expected outputs.
//
// Includes corruption detection to catch common issues:
//   - Empty golden files
//   - Binary corruption (null bytes)
//   - Invalid UTF-8
//
// Parameters:
//   - goldenPath: path to golden file (e.g., "testdata/golden/output.golden")
//   - actual: actual output to compare
//
// Example:
//
//	output := helpers.CapturePane(t, server, session)
//	helpers.CompareGolden(t, "testdata/golden/session-output.golden", output)
//
//	// To update golden files:
//	// helpers.SetUpdateFlag(true)
//	// go test -v
func CompareGolden(t *testing.T, goldenPath, actual string) {
	t.Helper()

	// Update mode: write actual as new golden
	if updateFlag {
		err := os.MkdirAll(filepath.Dir(goldenPath), 0o700)
		require.NoError(t, err)

		err = os.WriteFile(goldenPath, []byte(actual), 0o600)
		require.NoError(t, err, "Failed to update golden file")
		return
	}

	// Read golden file
	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("Golden file missing: %s\n\nTo create:\n  go test -update %s",
				goldenPath, t.Name())
		}
		t.Fatalf("Failed to read golden file %s: %v", goldenPath, err)
	}

	// Corruption detection
	if err := validateGoldenFile(expected); err != nil {
		t.Fatalf("Golden file corrupted: %s\n%v\n\nTo fix:\n  1. rm %s\n  2. go test -update %s\n  3. git diff testdata/",
			goldenPath, err, goldenPath, t.Name())
	}

	// Compare
	if string(expected) != actual {
		t.Errorf("Output mismatch for %s\n\nExpected:\n%s\n\nActual:\n%s\n\nDiff:\n%s",
			goldenPath, expected, actual, diff(expected, []byte(actual)))
	}
}

// validateGoldenFile detects common corruption patterns.
//
// Checks for:
//   - Empty files (likely corruption or incomplete write)
//   - Null bytes (binary corruption in what should be UTF-8 text)
//   - Invalid UTF-8 sequences
//
// Returns an error describing the corruption if found, nil otherwise.
func validateGoldenFile(data []byte) error {
	// Check for empty file
	if len(data) == 0 {
		return fmt.Errorf("golden file is empty (corrupted?)")
	}

	// Check for null bytes (binary corruption)
	if bytes.Contains(data, []byte{0x00}) {
		return fmt.Errorf("contains null bytes (expected UTF-8 text)")
	}

	// Check for invalid UTF-8
	if !utf8.Valid(data) {
		return fmt.Errorf("invalid UTF-8 encoding")
	}

	return nil
}

// diff returns simple line-by-line diff (basic implementation).
//
// For V1, this returns a helpful message directing users to use git diff.
// For V2, consider using github.com/pmezard/go-difflib for detailed diffs.
func diff(expected, actual []byte) string {
	// For V1: simple implementation
	// For V2: consider github.com/pmezard/go-difflib
	return "(use 'git diff testdata/' to see changes)"
}

// SetUpdateFlag enables golden file update mode.
//
// When enabled, CompareGolden will write actual output as the new golden
// file instead of comparing. Use this when updating expected test outputs.
//
// Example:
//
//	func TestMain(m *testing.M) {
//	    flag.BoolVar(&updateGolden, "update", false, "update golden files")
//	    flag.Parse()
//	    helpers.SetUpdateFlag(updateGolden)
//	    os.Exit(m.Run())
//	}
func SetUpdateFlag(update bool) {
	updateFlag = update
}
