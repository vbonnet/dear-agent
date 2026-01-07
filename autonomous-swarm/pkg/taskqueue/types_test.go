package taskqueue

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestTaskQueue_YAMLMarshal(t *testing.T) {
	tq := TaskQueue{
		SchemaVersion: "1.0",
		LastUpdated:   time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
		Ready: []Bead{
			{
				ID:    "bead-1",
				Tier:  1,
				Title: "Test Bead",
				Prompts: BeadPrompts{
					Start: "Start prompt",
				},
			},
		},
		InProgress: []Bead{},
		Blocked:    []Bead{},
		Completed:  []Bead{},
	}

	data, err := yaml.Marshal(&tq)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var tq2 TaskQueue
	if err := yaml.Unmarshal(data, &tq2); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if tq2.SchemaVersion != tq.SchemaVersion {
		t.Errorf("schema version mismatch: got %s, want %s", tq2.SchemaVersion, tq.SchemaVersion)
	}
	if len(tq2.Ready) != 1 {
		t.Errorf("ready beads count mismatch: got %d, want 1", len(tq2.Ready))
	}
	if tq2.Ready[0].ID != "bead-1" {
		t.Errorf("bead ID mismatch: got %s, want bead-1", tq2.Ready[0].ID)
	}
}

func TestBead_WithMetadata(t *testing.T) {
	bead := Bead{
		ID:    "test-bead",
		Tier:  2,
		Title: "Test Bead with Metadata",
		Metadata: BeadMetadata{
			SessionName:      "test-session",
			Iterations:       2,
			LastAttempt:      time.Date(2026, 1, 7, 12, 0, 0, 0, time.UTC),
			EscalationReason: "max iterations exceeded",
		},
	}

	if bead.Metadata.Iterations != 2 {
		t.Errorf("iterations mismatch: got %d, want 2", bead.Metadata.Iterations)
	}
	if bead.Metadata.SessionName != "test-session" {
		t.Errorf("session name mismatch: got %s, want test-session", bead.Metadata.SessionName)
	}
}

func TestBeadPrompts_AllFields(t *testing.T) {
	prompts := BeadPrompts{
		Start:    "Start the task",
		VerifyS8: "Verify files exist",
		VerifyS9: "Run tests",
		Done:     "Mark complete",
	}

	if prompts.Start != "Start the task" {
		t.Errorf("start prompt mismatch: got %s", prompts.Start)
	}
	if prompts.VerifyS8 != "Verify files exist" {
		t.Errorf("verify s8 mismatch: got %s", prompts.VerifyS8)
	}
	if prompts.VerifyS9 != "Run tests" {
		t.Errorf("verify s9 mismatch: got %s", prompts.VerifyS9)
	}
	if prompts.Done != "Mark complete" {
		t.Errorf("done prompt mismatch: got %s", prompts.Done)
	}
}

func TestBead_WithDependencies(t *testing.T) {
	bead := Bead{
		ID:        "dependent-bead",
		Tier:      1,
		Title:     "Depends on other beads",
		DependsOn: []string{"bead-1", "bead-2"},
		Prompts: BeadPrompts{
			Start: "Start after dependencies",
		},
	}

	if len(bead.DependsOn) != 2 {
		t.Errorf("dependencies count mismatch: got %d, want 2", len(bead.DependsOn))
	}
	if bead.DependsOn[0] != "bead-1" {
		t.Errorf("first dependency mismatch: got %s, want bead-1", bead.DependsOn[0])
	}
}
