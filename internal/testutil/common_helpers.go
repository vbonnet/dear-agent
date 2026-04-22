// Package testutil provides common test helpers for all package testing.
// Consolidates shared helpers from B4.2 and B4.4 sub-projects.
package testutil

import (
	"os"
	"testing"
	"time"
)

// SetupTempDir creates a temporary directory for testing and registers cleanup
func SetupTempDir(t *testing.T) string {
	t.Helper()
	tmpdir, err := os.MkdirTemp("", "engram-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpdir)
	})
	return tmpdir
}

// NowNano returns current time in nanoseconds for performance testing
func NowNano() int64 {
	return time.Now().UnixNano()
}

// HasAnthropicAPIKey checks if ANTHROPIC_API_KEY is set
func HasAnthropicAPIKey() bool {
	return os.Getenv("ANTHROPIC_API_KEY") != ""
}

// SkipIfRoot skips the test if running as root user.
// Root users bypass Unix filesystem permission checks, which breaks permission-related tests.
func SkipIfRoot(t *testing.T) {
	t.Helper()
	if os.Getuid() == 0 {
		t.Skip("Skipping test: requires non-root user for filesystem permission checks")
	}
}
