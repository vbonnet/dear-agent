package autoconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEngine_Apply(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	engine := NewEngine("test-project", DefaultBounds())

	retro := Retrospective{
		SessionID: "s1",
		Timestamp: time.Now(),
		Proposals: []Proposal{
			{Key: "context.compaction_threshold", NewValue: "0.7", Reason: "cost", Magnitude: 0.2},
			{Key: "context.max_tokens", NewValue: "4000", Reason: "efficiency", Magnitude: 0.1},
		},
	}

	applied, err := engine.Apply(retro)
	if err != nil {
		t.Fatal(err)
	}

	if len(applied) != 2 {
		t.Fatalf("applied = %d, want 2", len(applied))
	}

	// Verify config file was written.
	configPath, _ := AutoConfigPath("test-project")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	// Verify modifications log.
	logPath, _ := ModificationsLogPath()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("log file not created: %v", err)
	}

	var entry ModificationLog
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("parse log: %v", err)
	}
	if !entry.Applied {
		t.Error("log entry should be marked applied")
	}
	if len(entry.Proposals) != 2 {
		t.Errorf("logged proposals = %d, want 2", len(entry.Proposals))
	}
}

func TestEngine_MagnitudeBounds(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	bounds := Bounds{MaxMagnitude: 0.15, MaxChanges: 5}
	engine := NewEngine("test-project", bounds)

	retro := Retrospective{
		SessionID: "s1",
		Proposals: []Proposal{
			{Key: "a", NewValue: "1", Magnitude: 0.1},  // within bounds
			{Key: "b", NewValue: "2", Magnitude: 0.5},  // exceeds magnitude
			{Key: "c", NewValue: "3", Magnitude: 0.15}, // at boundary
		},
	}

	applied, err := engine.Apply(retro)
	if err != nil {
		t.Fatal(err)
	}

	if len(applied) != 2 {
		t.Fatalf("applied = %d, want 2 (a and c)", len(applied))
	}
	if applied[0].Key != "a" || applied[1].Key != "c" {
		t.Errorf("wrong proposals applied: %v", applied)
	}
}

func TestEngine_MaxChanges(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	bounds := Bounds{MaxMagnitude: 1.0, MaxChanges: 2}
	engine := NewEngine("test-project", bounds)

	retro := Retrospective{
		SessionID: "s1",
		Proposals: []Proposal{
			{Key: "a", NewValue: "1", Magnitude: 0.1},
			{Key: "b", NewValue: "2", Magnitude: 0.1},
			{Key: "c", NewValue: "3", Magnitude: 0.1},
		},
	}

	applied, err := engine.Apply(retro)
	if err != nil {
		t.Fatal(err)
	}

	if len(applied) != 2 {
		t.Errorf("applied = %d, want 2", len(applied))
	}
}

func TestEngine_NoProposals(t *testing.T) {
	engine := NewEngine("test", DefaultBounds())

	retro := Retrospective{SessionID: "s1"}
	applied, err := engine.Apply(retro)
	if err != nil {
		t.Fatal(err)
	}
	if applied != nil {
		t.Errorf("expected nil applied, got %v", applied)
	}
}

func TestAutoConfigPath(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	path, err := AutoConfigPath("myproject")
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(tmpHome, ".engram", "auto-config", "myproject.yaml")
	if path != expected {
		t.Errorf("path = %q, want %q", path, expected)
	}
}
