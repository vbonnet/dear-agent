package daemon

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbonnet/dear-agent/pkg/enforcement"
)

// newTypedDetector builds a ViolationDetector whose database contains a single
// pattern with the given ID, so detectPatternType can resolve ownership by ID.
func newTypedDetector(t *testing.T, id string) *enforcement.ViolationDetector {
	t.Helper()
	d, err := enforcement.NewDetector(&enforcement.PatternDatabase{
		Patterns: []enforcement.Pattern{
			{ID: id, Regex: `\b` + id + `\b`},
		},
	})
	require.NoError(t, err)
	return d
}

// TestDetectPatternType verifies the pattern type is resolved from the database
// that owns the pattern ID, rather than always defaulting to "bash".
func TestDetectPatternType(t *testing.T) {
	m := &SessionMonitor{
		bashDetector:  newTypedDetector(t, "bash-pattern"),
		beadsDetector: newTypedDetector(t, "beads-pattern"),
		gitDetector:   newTypedDetector(t, "git-pattern"),
	}

	tests := []struct {
		name    string
		pattern *enforcement.Pattern
		want    string
	}{
		{"bash pattern", &enforcement.Pattern{ID: "bash-pattern"}, "bash"},
		{"beads pattern", &enforcement.Pattern{ID: "beads-pattern"}, "beads"},
		{"git pattern", &enforcement.Pattern{ID: "git-pattern"}, "git"},
		{"unknown pattern defaults to bash", &enforcement.Pattern{ID: "mystery"}, "bash"},
		{"nil pattern defaults to bash", nil, "bash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.detectPatternType(tt.pattern); got != tt.want {
				t.Errorf("detectPatternType(%v) = %q, want %q", tt.pattern, got, tt.want)
			}
		})
	}
}

// TestDetectPatternType_NilDetectors verifies nil detectors are handled safely
// and the function still falls back to "bash".
func TestDetectPatternType_NilDetectors(t *testing.T) {
	m := &SessionMonitor{}
	if got := m.detectPatternType(&enforcement.Pattern{ID: "anything"}); got != "bash" {
		t.Errorf("detectPatternType with nil detectors = %q, want %q", got, "bash")
	}
}
