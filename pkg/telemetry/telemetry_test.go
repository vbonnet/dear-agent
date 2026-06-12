package telemetry

import (
	"testing"
)

func TestLevelConstants_Distinct(t *testing.T) {
	levels := []Level{LevelInfo, LevelWarn, LevelError, LevelCritical}
	seen := map[Level]bool{}
	for _, l := range levels {
		if seen[l] {
			t.Errorf("level %v is duplicated", l)
		}
		seen[l] = true
	}
}

func TestEventListenerIsInterface(t *testing.T) {
	// Verify EventListener is assignable to a nil pointer — compile-time check.
	var _ EventListener = nil
}
