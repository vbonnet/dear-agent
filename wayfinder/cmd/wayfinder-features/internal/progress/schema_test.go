package progress

import (
	"testing"
)

func TestValidateStatus_ValidValues(t *testing.T) {
	for _, s := range []string{StatusFailing, StatusInProgress, StatusPassing} {
		if !ValidateStatus(s) {
			t.Errorf("ValidateStatus(%q) = false, want true", s)
		}
	}
}

func TestValidateStatus_InvalidValue(t *testing.T) {
	for _, s := range []string{"", "done", "unknown", "PASSING"} {
		if ValidateStatus(s) {
			t.Errorf("ValidateStatus(%q) = true, want false", s)
		}
	}
}

func TestNewProgress_FieldsSet(t *testing.T) {
	features := []Feature{{ID: "F-1", Status: StatusFailing}}
	prog := NewProgress("my-project", features)

	if prog.Project != "my-project" {
		t.Errorf("Project = %q, want %q", prog.Project, "my-project")
	}
	if prog.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", prog.SchemaVersion, SchemaVersion)
	}
	if prog.Waypoint != WaypointS5 {
		t.Errorf("Waypoint = %q, want %q", prog.Waypoint, WaypointS5)
	}
	if len(prog.Features) != 1 {
		t.Errorf("Features count = %d, want 1", len(prog.Features))
	}
	if prog.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}
