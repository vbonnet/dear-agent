package progress

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// --- ValidateStatus ---

func TestValidateStatus_Valid(t *testing.T) {
	for _, s := range []string{StatusFailing, StatusInProgress, StatusPassing} {
		if !ValidateStatus(s) {
			t.Errorf("ValidateStatus(%q) = false, want true", s)
		}
	}
}

func TestValidateStatus_Invalid(t *testing.T) {
	for _, s := range []string{"", "unknown", "done", "started"} {
		if ValidateStatus(s) {
			t.Errorf("ValidateStatus(%q) = true, want false", s)
		}
	}
}

// --- NewProgress ---

func TestNewProgress_Defaults(t *testing.T) {
	features := []Feature{{ID: "F-001", Status: StatusFailing}}
	p := NewProgress("My Project", features)

	if p.Project != "My Project" {
		t.Errorf("Project = %q, want My Project", p.Project)
	}
	if p.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", p.SchemaVersion, SchemaVersion)
	}
	if p.Waypoint != WaypointS5 {
		t.Errorf("Waypoint = %q, want %q", p.Waypoint, WaypointS5)
	}
	if p.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
	if len(p.Features) != 1 {
		t.Errorf("want 1 feature, got %d", len(p.Features))
	}
}

// --- ReadProgress / WriteProgress ---

func writeProgressFile(t *testing.T, dir string, p *Progress) string {
	t.Helper()
	path := filepath.Join(dir, "progress.json")
	if err := WriteProgress(path, p); err != nil {
		t.Fatalf("WriteProgress: %v", err)
	}
	return path
}

func TestReadWriteProgress_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	orig := NewProgress("Round Trip", []Feature{
		{ID: "F-001", Status: StatusPassing},
		{ID: "F-002", Status: StatusInProgress},
	})
	path := writeProgressFile(t, dir, orig)

	got, err := ReadProgress(path)
	if err != nil {
		t.Fatalf("ReadProgress: %v", err)
	}
	if got.Project != "Round Trip" {
		t.Errorf("Project = %q, want Round Trip", got.Project)
	}
	if len(got.Features) != 2 {
		t.Errorf("want 2 features, got %d", len(got.Features))
	}
	if got.Features[0].ID != "F-001" {
		t.Errorf("Features[0].ID = %q, want F-001", got.Features[0].ID)
	}
}

func TestReadProgress_FileNotFound(t *testing.T) {
	_, err := ReadProgress("/nonexistent/path/progress.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestReadProgress_CorruptedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ReadProgress(path)
	if err == nil {
		t.Error("expected error for corrupted JSON")
	}
}

func TestReadProgress_WrongSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	data, _ := json.Marshal(map[string]any{
		"schema_version": "99.0",
		"project":        "X",
		"waypoint":       "S5",
		"features":       []any{},
	})
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ReadProgress(path)
	if err == nil {
		t.Error("expected error for unsupported schema version")
	}
}

func TestWriteProgress_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "dir", "progress.json")
	p := NewProgress("Nested", nil)
	if err := WriteProgress(path, p); err != nil {
		t.Fatalf("WriteProgress: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file should exist after WriteProgress: %v", err)
	}
}
