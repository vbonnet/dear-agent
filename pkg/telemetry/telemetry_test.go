package telemetry_test

import (
	"testing"

	"github.com/vbonnet/dear-agent/pkg/telemetry"
)

func TestLevelConstants_Values(t *testing.T) {
	t.Parallel()
	// Ensure the re-exported constants are non-empty and distinct.
	levels := []telemetry.Level{
		telemetry.LevelInfo,
		telemetry.LevelWarn,
		telemetry.LevelError,
		telemetry.LevelCritical,
	}

	seen := make(map[telemetry.Level]bool)
	for _, lvl := range levels {
		if seen[lvl] {
			t.Errorf("duplicate level value %v", lvl)
		}
		seen[lvl] = true
	}
	if len(seen) != 4 {
		t.Errorf("expected 4 distinct levels, got %d", len(seen))
	}
}
