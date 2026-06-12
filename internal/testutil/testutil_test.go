package testutil

import (
	"os"
	"testing"
)

func TestSetupTempDir_CreatesDir(t *testing.T) {
	t.Parallel()
	dir := SetupTempDir(t)
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("SetupTempDir: directory does not exist: %v", err)
	}
}

func TestNowNano_Positive(t *testing.T) {
	t.Parallel()
	if n := NowNano(); n <= 0 {
		t.Errorf("NowNano() = %d, want > 0", n)
	}
}

func TestHasAnthropicAPIKey_ReturnsBool(t *testing.T) {
	t.Parallel()
	// Just verify the function doesn't panic; return value depends on env.
	_ = HasAnthropicAPIKey()
}

func TestSkipIfRoot_NoopWhenNonRoot(t *testing.T) {
	t.Parallel()
	if os.Getuid() != 0 {
		SkipIfRoot(t) // Should be a no-op.
	}
}
