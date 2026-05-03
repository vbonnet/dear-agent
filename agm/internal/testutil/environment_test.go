package testutil

import (
	"os"
	"testing"
)

func TestSetupTestEnvironment(t *testing.T) {
	// Save original values
	origTestMode := os.Getenv("ENGRAM_TEST_MODE")
	origTestWorkspace := os.Getenv("ENGRAM_TEST_WORKSPACE")
	origWorkspace := os.Getenv("WORKSPACE")

	// Clear env vars to test setup from scratch
	os.Unsetenv("ENGRAM_TEST_MODE")
	os.Unsetenv("ENGRAM_TEST_WORKSPACE")
	os.Unsetenv("WORKSPACE")

	t.Cleanup(func() {
		// Restore originals
		if origTestMode != "" {
			t.Setenv("ENGRAM_TEST_MODE", origTestMode)
		}
		if origTestWorkspace != "" {
			t.Setenv("ENGRAM_TEST_WORKSPACE", origTestWorkspace)
		}
		if origWorkspace != "" {
			t.Setenv("WORKSPACE", origWorkspace)
		}
	})

	// Run in a subtest so Cleanup fires before our checks
	t.Run("sets_env_vars", func(t *testing.T) {
		SetupTestEnvironment(t)

		if got := os.Getenv("ENGRAM_TEST_MODE"); got != "1" {
			t.Errorf("expected ENGRAM_TEST_MODE=1, got %q", got)
		}
		if got := os.Getenv("ENGRAM_TEST_WORKSPACE"); got != "test" {
			t.Errorf("expected ENGRAM_TEST_WORKSPACE=test, got %q", got)
		}
		if got := os.Getenv("WORKSPACE"); got != "test" {
			t.Errorf("expected WORKSPACE=test, got %q", got)
		}
	})
}

func TestSetupTestEnvironmentPreservesExisting(t *testing.T) {
	origTestMode := os.Getenv("ENGRAM_TEST_MODE")
	origTestWorkspace := os.Getenv("ENGRAM_TEST_WORKSPACE")
	origWorkspace := os.Getenv("WORKSPACE")

	t.Cleanup(func() {
		if origTestMode != "" {
			t.Setenv("ENGRAM_TEST_MODE", origTestMode)
		} else {
			os.Unsetenv("ENGRAM_TEST_MODE")
		}
		if origTestWorkspace != "" {
			t.Setenv("ENGRAM_TEST_WORKSPACE", origTestWorkspace)
		} else {
			os.Unsetenv("ENGRAM_TEST_WORKSPACE")
		}
		if origWorkspace != "" {
			t.Setenv("WORKSPACE", origWorkspace)
		} else {
			os.Unsetenv("WORKSPACE")
		}
	})

	// Pre-set values
	t.Setenv("ENGRAM_TEST_MODE", "true")
	t.Setenv("ENGRAM_TEST_WORKSPACE", "custom")

	t.Run("preserves_existing", func(t *testing.T) {
		SetupTestEnvironment(t)

		// Should keep existing values
		if got := os.Getenv("ENGRAM_TEST_MODE"); got != "true" {
			t.Errorf("expected ENGRAM_TEST_MODE=true (preserved), got %q", got)
		}
		if got := os.Getenv("ENGRAM_TEST_WORKSPACE"); got != "custom" {
			t.Errorf("expected ENGRAM_TEST_WORKSPACE=custom (preserved), got %q", got)
		}
		// WORKSPACE should be set to custom workspace
		if got := os.Getenv("WORKSPACE"); got != "custom" {
			t.Errorf("expected WORKSPACE=custom, got %q", got)
		}
	})
}

func TestRequireTestMode(t *testing.T) {
	origTestMode := os.Getenv("ENGRAM_TEST_MODE")
	origTestWorkspace := os.Getenv("ENGRAM_TEST_WORKSPACE")

	t.Cleanup(func() {
		if origTestMode != "" {
			t.Setenv("ENGRAM_TEST_MODE", origTestMode)
		} else {
			os.Unsetenv("ENGRAM_TEST_MODE")
		}
		if origTestWorkspace != "" {
			t.Setenv("ENGRAM_TEST_WORKSPACE", origTestWorkspace)
		} else {
			os.Unsetenv("ENGRAM_TEST_WORKSPACE")
		}
	})

	// Set both required env vars
	t.Setenv("ENGRAM_TEST_MODE", "1")
	t.Setenv("ENGRAM_TEST_WORKSPACE", "test")

	// This should not fatal
	t.Run("succeeds_when_set", func(t *testing.T) {
		RequireTestMode(t) // Should not fail
	})
}
