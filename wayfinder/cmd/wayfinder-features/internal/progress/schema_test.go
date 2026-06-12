package progress

import (
	"testing"
	"time"
)

func TestValidateStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status string
		want   bool
	}{
		{StatusFailing, true},
		{StatusInProgress, true},
		{StatusPassing, true},
		{"unknown", false},
		{"", false},
		{"PASSING", false},
	}
	for _, tc := range cases {
		got := ValidateStatus(tc.status)
		if got != tc.want {
			t.Errorf("ValidateStatus(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestNewProgress_Defaults(t *testing.T) {
	t.Parallel()
	before := time.Now()
	p := NewProgress("my-project", nil)
	after := time.Now()

	if p.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", p.SchemaVersion, SchemaVersion)
	}
	if p.Project != "my-project" {
		t.Errorf("Project = %q, want %q", p.Project, "my-project")
	}
	if p.Waypoint != WaypointS5 {
		t.Errorf("Waypoint = %q, want %q", p.Waypoint, WaypointS5)
	}
	if p.CreatedAt.Before(before) || p.CreatedAt.After(after) {
		t.Errorf("CreatedAt %v is outside [%v, %v]", p.CreatedAt, before, after)
	}
	if p.LastUpdated.Before(before) || p.LastUpdated.After(after) {
		t.Errorf("LastUpdated %v is outside [%v, %v]", p.LastUpdated, before, after)
	}
	if len(p.Features) != 0 {
		t.Errorf("Features = %v, want empty", p.Features)
	}
}

func TestNewProgress_WithFeatures(t *testing.T) {
	t.Parallel()
	features := []Feature{
		{ID: "F1", Status: StatusFailing},
		{ID: "F2", Status: StatusPassing},
	}
	p := NewProgress("proj", features)
	if len(p.Features) != 2 {
		t.Fatalf("Features len = %d, want 2", len(p.Features))
	}
	if p.Features[0].ID != "F1" || p.Features[1].ID != "F2" {
		t.Errorf("Features = %v, want F1/F2", p.Features)
	}
}
