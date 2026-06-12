package output

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-features/internal/progress"
)

// TestFormatStatus_NoColor tests FormatStatus with NO_COLOR set to avoid
// terminal-specific colour escape sequences in assertions.
func TestFormatStatus_NoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	prog := progress.NewProgress("My Project", []progress.Feature{
		{ID: "F-001", Status: progress.StatusPassing},
		{ID: "F-002", Status: progress.StatusInProgress},
		{ID: "F-003", Status: progress.StatusFailing},
	})
	got := FormatStatus(prog)

	if !strings.Contains(got, "My Project") {
		t.Errorf("output should contain project name, got: %q", got)
	}
	if !strings.Contains(got, "F-001") {
		t.Errorf("output should contain F-001, got: %q", got)
	}
	if !strings.Contains(got, "passing") {
		t.Errorf("output should contain 'passing' status, got: %q", got)
	}
	if !strings.Contains(got, "F-002") {
		t.Errorf("output should contain F-002, got: %q", got)
	}
	if !strings.Contains(got, "in_progress") {
		t.Errorf("output should contain 'in_progress' status, got: %q", got)
	}
	if !strings.Contains(got, "Next:") {
		t.Errorf("output should contain 'Next:' section, got: %q", got)
	}
	if !strings.Contains(got, "Progress: 1/3") {
		t.Errorf("output should show progress fraction 1/3, got: %q", got)
	}
}

func TestFormatStatus_AllPassing(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	prog := progress.NewProgress("Done", []progress.Feature{
		{ID: "F-001", Status: progress.StatusPassing},
		{ID: "F-002", Status: progress.StatusPassing},
	})
	got := FormatStatus(prog)

	if !strings.Contains(got, "All features complete") {
		t.Errorf("expected 'All features complete' when all passing, got: %q", got)
	}
	if !strings.Contains(got, "Progress: 2/2") {
		t.Errorf("expected full progress fraction 2/2, got: %q", got)
	}
}

func TestFormatStatus_EmptyFeatures(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	prog := progress.NewProgress("Empty", nil)
	got := FormatStatus(prog)

	// Should not panic and should contain the project header
	if !strings.Contains(got, "Empty") {
		t.Errorf("output should contain project name, got: %q", got)
	}
}
