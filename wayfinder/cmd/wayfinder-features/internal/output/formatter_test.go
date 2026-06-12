package output

import (
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-features/internal/progress"
)

// setNoColor disables color output for the test by setting NO_COLOR in the
// environment. Restores the original value on cleanup.
func setNoColor(t *testing.T) {
	t.Helper()
	t.Setenv("NO_COLOR", "1")
}

func TestFormatStatus_Empty(t *testing.T) {
	setNoColor(t)

	p := progress.NewProgress("test-project", nil)
	got := FormatStatus(p)

	if !strings.Contains(got, "test-project") {
		t.Errorf("FormatStatus: missing project name; got %q", got)
	}
	if !strings.Contains(got, "0/0 features verified") {
		t.Errorf("FormatStatus: missing 0/0 progress; got %q", got)
	}
	if !strings.Contains(got, "All features complete") {
		t.Errorf("FormatStatus: expected 'All features complete' for empty feature list; got %q", got)
	}
}

func TestFormatStatus_AllPassing(t *testing.T) {
	setNoColor(t)

	now := time.Now()
	features := []progress.Feature{
		{ID: "F1", Status: progress.StatusPassing, VerifiedAt: &now},
		{ID: "F2", Status: progress.StatusPassing, VerifiedAt: &now},
	}
	p := progress.NewProgress("proj", features)
	got := FormatStatus(p)

	if !strings.Contains(got, "2/2 features verified (100%)") {
		t.Errorf("FormatStatus: expected 100%%; got %q", got)
	}
	if !strings.Contains(got, "All features complete") {
		t.Errorf("FormatStatus: expected 'All features complete'; got %q", got)
	}
}

func TestFormatStatus_Mixed(t *testing.T) {
	setNoColor(t)

	now := time.Now()
	features := []progress.Feature{
		{ID: "F1", Status: progress.StatusPassing, VerifiedAt: &now},
		{ID: "F2", Status: progress.StatusInProgress, StartedAt: &now},
		{ID: "F3", Status: progress.StatusFailing},
	}
	p := progress.NewProgress("myproj", features)
	got := FormatStatus(p)

	if !strings.Contains(got, "1/3 features verified") {
		t.Errorf("FormatStatus: expected '1/3 features verified'; got %q", got)
	}
	if !strings.Contains(got, "F2") {
		t.Errorf("FormatStatus: missing F2 in output; got %q", got)
	}
	// Next should point to in-progress F2.
	if !strings.Contains(got, "Complete F2") {
		t.Errorf("FormatStatus: expected 'Complete F2' as next item; got %q", got)
	}
}

func TestFormatStatus_AllFailing(t *testing.T) {
	setNoColor(t)

	features := []progress.Feature{
		{ID: "F1", Status: progress.StatusFailing},
		{ID: "F2", Status: progress.StatusFailing},
	}
	p := progress.NewProgress("proj", features)
	got := FormatStatus(p)

	if !strings.Contains(got, "0/2 features verified (0%)") {
		t.Errorf("FormatStatus: expected '0/2 features verified (0%%)'; got %q", got)
	}
	// First failing feature is next.
	if !strings.Contains(got, "Start F1") {
		t.Errorf("FormatStatus: expected 'Start F1' as next item; got %q", got)
	}
}
