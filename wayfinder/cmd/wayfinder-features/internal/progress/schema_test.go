package progress_test

import (
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-features/internal/progress"
)

func TestValidateStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status string
		want   bool
	}{
		{progress.StatusFailing, true},
		{progress.StatusInProgress, true},
		{progress.StatusPassing, true},
		{"", false},
		{"unknown", false},
		{"PASSING", false},
	}
	for _, tc := range cases {
		got := progress.ValidateStatus(tc.status)
		if got != tc.want {
			t.Errorf("ValidateStatus(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestNewProgress_Defaults(t *testing.T) {
	t.Parallel()
	features := []progress.Feature{
		{ID: "f1", Status: progress.StatusFailing},
	}
	p := progress.NewProgress("myproject", features)
	if p.Project != "myproject" {
		t.Errorf("Project = %q, want %q", p.Project, "myproject")
	}
	if p.Waypoint != progress.WaypointS5 {
		t.Errorf("Waypoint = %q, want %q", p.Waypoint, progress.WaypointS5)
	}
	if p.SchemaVersion != progress.SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", p.SchemaVersion, progress.SchemaVersion)
	}
	if len(p.Features) != 1 || p.Features[0].ID != "f1" {
		t.Errorf("Features = %v, want [{f1 failing}]", p.Features)
	}
	if p.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if p.LastUpdated.IsZero() {
		t.Error("LastUpdated should not be zero")
	}
}

func TestStatusConstants(t *testing.T) {
	t.Parallel()
	if progress.StatusFailing != "failing" {
		t.Errorf("StatusFailing = %q, want %q", progress.StatusFailing, "failing")
	}
	if progress.StatusInProgress != "in_progress" {
		t.Errorf("StatusInProgress = %q, want %q", progress.StatusInProgress, "in_progress")
	}
	if progress.StatusPassing != "passing" {
		t.Errorf("StatusPassing = %q, want %q", progress.StatusPassing, "passing")
	}
}
