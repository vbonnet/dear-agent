package testutil

import (
	"os"
	"testing"
)

// RequireTestMode ensures ENGRAM_TEST_MODE is set, preventing test pollution
// Call this at the start of any test that creates sessions
func RequireTestMode(t *testing.T) {
	t.Helper()

	testMode := os.Getenv("ENGRAM_TEST_MODE")
	if testMode != "1" && testMode != "true" {
		t.Fatal("ENGRAM_TEST_MODE must be set to prevent test session pollution\n" +
			"Run tests with: ENGRAM_TEST_MODE=1 ENGRAM_TEST_WORKSPACE=test go test ./...")
	}

	// Ensure test workspace is set
	testWorkspace := os.Getenv("ENGRAM_TEST_WORKSPACE")
	if testWorkspace == "" {
		t.Fatal("ENGRAM_TEST_WORKSPACE must be set for test isolation\n" +
			"Run tests with: ENGRAM_TEST_MODE=1 ENGRAM_TEST_WORKSPACE=test go test ./...")
	}
}

// SetupTestEnvironment sets up test mode for unit tests
// Use this in TestMain or individual test setup functions
//
// This function:
// - Sets ENGRAM_TEST_MODE=1 to enable test isolation checks
// - Sets ENGRAM_TEST_WORKSPACE=test (or existing value if already set)
// - Sets WORKSPACE=test to ensure Dolt uses test database
// - Registers cleanup to restore environment after test
func SetupTestEnvironment(t *testing.T) {
	t.Helper()

	// Set test mode if not already set
	if os.Getenv("ENGRAM_TEST_MODE") == "" {
		t.Setenv("ENGRAM_TEST_MODE", "1")
		t.Cleanup(func() {
			os.Unsetenv("ENGRAM_TEST_MODE")
		})
	}

	// Set test workspace if not already set
	// Note: Workspace is a NAME (e.g., "test"), not a file path
	// Dolt uses workspace name to select database (workspace "test" → database "test")
	if os.Getenv("ENGRAM_TEST_WORKSPACE") == "" {
		t.Setenv("ENGRAM_TEST_WORKSPACE", "test")
		t.Cleanup(func() {
			os.Unsetenv("ENGRAM_TEST_WORKSPACE")
		})
	}

	// Set WORKSPACE to test value to ensure Dolt uses test database
	testWorkspace := os.Getenv("ENGRAM_TEST_WORKSPACE")
	if testWorkspace == "" {
		testWorkspace = "test" // Fallback
	}

	// Save original WORKSPACE value
	originalWorkspace := os.Getenv("WORKSPACE")

	// Set WORKSPACE to test workspace
	t.Setenv("WORKSPACE", testWorkspace)

	// Register cleanup to restore original WORKSPACE
	t.Cleanup(func() {
		if originalWorkspace != "" {
			t.Setenv("WORKSPACE", originalWorkspace)
		} else {
			os.Unsetenv("WORKSPACE")
		}
	})
}
