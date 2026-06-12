package telemetry_test

import (
	"testing"

	"github.com/vbonnet/dear-agent/pkg/telemetry"
)

func TestLevelConstants(t *testing.T) {
	levels := []telemetry.Level{
		telemetry.LevelInfo,
		telemetry.LevelWarn,
		telemetry.LevelError,
		telemetry.LevelCritical,
	}
	seen := make(map[telemetry.Level]bool, len(levels))
	for _, l := range levels {
		if seen[l] {
			t.Errorf("duplicate level value: %v", l)
		}
		seen[l] = true
	}
}
