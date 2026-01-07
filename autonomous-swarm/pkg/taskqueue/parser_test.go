package taskqueue

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTaskQueue_ValidYAML(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "valid-queue.yaml")

	// Write valid YAML
	validYAML := `schema_version: "1.0"
last_updated: 2026-01-07T00:00:00Z
ready:
  - id: "bead-1"
    tier: 1
    title: "Test Bead 1"
    prompts:
      start: "Start prompt"
  - id: "bead-2"
    tier: 2
    title: "Test Bead 2"
    depends_on: ["bead-1"]
    prompts:
      start: "Start after bead-1"
in_progress: []
blocked:
  - id: "bead-3"
    tier: 1
    title: "Blocked Bead"
    depends_on: ["bead-1"]
    prompts:
      start: "Waiting for dependencies"
completed: []
`
	if err := os.WriteFile(yamlPath, []byte(validYAML), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Parse
	tq, err := ParseTaskQueue(yamlPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Validate
	if tq.SchemaVersion != "1.0" {
		t.Errorf("schema_version: got %s, want 1.0", tq.SchemaVersion)
	}
	if len(tq.Ready) != 2 {
		t.Errorf("ready beads: got %d, want 2", len(tq.Ready))
	}
	if tq.Ready[0].ID != "bead-1" {
		t.Errorf("ready[0].id: got %s, want bead-1", tq.Ready[0].ID)
	}
	if tq.Ready[1].Tier != 2 {
		t.Errorf("ready[1].tier: got %d, want 2", tq.Ready[1].Tier)
	}
	if len(tq.Blocked) != 1 {
		t.Errorf("blocked beads: got %d, want 1", len(tq.Blocked))
	}
}

func TestParseTaskQueue_MissingSchemaVersion(t *testing.T) {
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "no-schema.yaml")

	invalidYAML := `last_updated: 2026-01-07T00:00:00Z
ready:
  - id: "bead-1"
    tier: 1
    title: "Test Bead"
    prompts:
      start: "Start"
in_progress: []
blocked: []
completed: []
`
	if err := os.WriteFile(yamlPath, []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := ParseTaskQueue(yamlPath)
	if err == nil {
		t.Fatal("expected error for missing schema_version, got nil")
	}
	if err.Error() != "validation failed: schema_version is required" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestParseTaskQueue_MalformedYAML(t *testing.T) {
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "malformed.yaml")

	malformedYAML := `schema_version: "1.0"
ready:
  - id: "bead-1"
    tier: invalid-tier
    title: "Test"
`
	if err := os.WriteFile(yamlPath, []byte(malformedYAML), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := ParseTaskQueue(yamlPath)
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
}

func TestParseTaskQueue_FileNotFound(t *testing.T) {
	_, err := ParseTaskQueue("/nonexistent/path/queue.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestValidateBead_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name        string
		bead        Bead
		expectError string
	}{
		{
			name: "missing id",
			bead: Bead{
				Tier:  1,
				Title: "Test",
				Prompts: BeadPrompts{
					Start: "Start",
				},
			},
			expectError: "id is required",
		},
		{
			name: "missing title",
			bead: Bead{
				ID:   "bead-1",
				Tier: 1,
				Prompts: BeadPrompts{
					Start: "Start",
				},
			},
			expectError: "title is required for bead bead-1",
		},
		{
			name: "missing start prompt",
			bead: Bead{
				ID:      "bead-1",
				Tier:    1,
				Title:   "Test",
				Prompts: BeadPrompts{},
			},
			expectError: "prompts.start is required for bead bead-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBead(&tt.bead, "test", 0)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != "test[0]: "+tt.expectError {
				t.Errorf("got error %q, want %q", err.Error(), "test[0]: "+tt.expectError)
			}
		})
	}
}

func TestValidateBead_TierConstraints(t *testing.T) {
	tests := []struct {
		name string
		tier int
		fail bool
	}{
		{"tier 0 invalid", 0, true},
		{"tier 1 valid", 1, false},
		{"tier 2 valid", 2, false},
		{"tier 3 valid", 3, false},
		{"tier 4 valid", 4, false},
		{"tier 5 invalid", 5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bead := Bead{
				ID:    "test-bead",
				Tier:  tt.tier,
				Title: "Test",
				Prompts: BeadPrompts{
					Start: "Start",
				},
			}

			err := validateBead(&bead, "test", 0)
			if tt.fail && err == nil {
				t.Errorf("expected error for tier %d, got nil", tt.tier)
			}
			if !tt.fail && err != nil {
				t.Errorf("unexpected error for tier %d: %v", tt.tier, err)
			}
		})
	}
}

func TestParseTaskQueue_CompleteExample(t *testing.T) {
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "complete.yaml")

	completeYAML := `schema_version: "1.0"
last_updated: 2026-01-07T12:30:45Z
ready:
  - id: "foundation-setup"
    tier: 1
    title: "Foundation: Go module + types"
    prompts:
      start: "Initialize Go module and define core types"
      verify_s8: "Verify types.go files exist"
      verify_s9: "Run go test ./..."
      done: "Foundation complete"
in_progress:
  - id: "yaml-parser"
    tier: 2
    title: "Implement YAML parser"
    depends_on: ["foundation-setup"]
    prompts:
      start: "Create parser.go with validation"
    metadata:
      session_name: "parser-session"
      iterations: 1
      last_attempt: 2026-01-07T12:00:00Z
blocked: []
completed:
  - id: "planning"
    tier: 1
    title: "S7 Planning"
    prompts:
      start: "Create implementation plan"
    metadata:
      session_name: "planning-session"
      iterations: 1
      last_attempt: 2026-01-06T10:00:00Z
`
	if err := os.WriteFile(yamlPath, []byte(completeYAML), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	tq, err := ParseTaskQueue(yamlPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Validate structure
	if len(tq.Ready) != 1 {
		t.Errorf("ready: got %d beads, want 1", len(tq.Ready))
	}
	if len(tq.InProgress) != 1 {
		t.Errorf("in_progress: got %d beads, want 1", len(tq.InProgress))
	}
	if len(tq.Completed) != 1 {
		t.Errorf("completed: got %d beads, want 1", len(tq.Completed))
	}

	// Validate in_progress bead with metadata
	inProgress := tq.InProgress[0]
	if inProgress.ID != "yaml-parser" {
		t.Errorf("in_progress[0].id: got %s, want yaml-parser", inProgress.ID)
	}
	if inProgress.Metadata.SessionName != "parser-session" {
		t.Errorf("metadata.session_name: got %s, want parser-session", inProgress.Metadata.SessionName)
	}
	if inProgress.Metadata.Iterations != 1 {
		t.Errorf("metadata.iterations: got %d, want 1", inProgress.Metadata.Iterations)
	}

	// Validate dependencies
	if len(inProgress.DependsOn) != 1 || inProgress.DependsOn[0] != "foundation-setup" {
		t.Errorf("depends_on: got %v, want [foundation-setup]", inProgress.DependsOn)
	}

	// Validate prompts
	if inProgress.Prompts.Start == "" {
		t.Error("prompts.start should not be empty")
	}

	// Validate timestamp parsing
	if tq.LastUpdated.Year() != 2026 {
		t.Errorf("last_updated year: got %d, want 2026", tq.LastUpdated.Year())
	}
}

func TestParseTaskQueue_EmptySections(t *testing.T) {
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "empty.yaml")

	emptyYAML := `schema_version: "1.0"
last_updated: 2026-01-07T00:00:00Z
ready: []
in_progress: []
blocked: []
completed: []
`
	if err := os.WriteFile(yamlPath, []byte(emptyYAML), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	tq, err := ParseTaskQueue(yamlPath)
	if err != nil {
		t.Fatalf("unexpected error for empty sections: %v", err)
	}

	if len(tq.Ready) != 0 || len(tq.InProgress) != 0 || len(tq.Blocked) != 0 || len(tq.Completed) != 0 {
		t.Error("expected all sections to be empty")
	}
}
