package trigger

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestShouldInjectNoCooldown(t *testing.T) {
	s := NewTriggerState()
	s.RecordInjection("engrams/test.ai.md")

	if !s.ShouldInject("engrams/test.ai.md", "") {
		t.Error("expected ShouldInject to return true with empty cooldown")
	}
}

func TestShouldInjectWithinCooldown(t *testing.T) {
	s := NewTriggerState()
	s.RecordInjection("engrams/test.ai.md")

	if s.ShouldInject("engrams/test.ai.md", "1h") {
		t.Error("expected ShouldInject to return false within cooldown period")
	}
}

func TestShouldInjectAfterCooldown(t *testing.T) {
	s := NewTriggerState()
	// Set last injection to 2 hours ago.
	s.LastInjected["engrams/test.ai.md"] = time.Now().Add(-2 * time.Hour)

	if !s.ShouldInject("engrams/test.ai.md", "1h") {
		t.Error("expected ShouldInject to return true after cooldown elapsed")
	}
}

func TestLoadSaveState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trigger-state.json")

	// Create and save state.
	s := NewTriggerState()
	s.RecordInjection("engrams/test.ai.md")
	s.RecordInjection("engrams/other.ai.md")

	if err := s.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load it back.
	loaded, err := LoadTriggerState(path)
	if err != nil {
		t.Fatalf("LoadTriggerState failed: %v", err)
	}

	if len(loaded.LastInjected) != 2 {
		t.Errorf("expected 2 entries in LastInjected, got %d", len(loaded.LastInjected))
	}

	if _, ok := loaded.LastInjected["engrams/test.ai.md"]; !ok {
		t.Error("expected 'engrams/test.ai.md' in loaded state")
	}
	if _, ok := loaded.LastInjected["engrams/other.ai.md"]; !ok {
		t.Error("expected 'engrams/other.ai.md' in loaded state")
	}
}

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")

	state, err := LoadTriggerState(path)
	if err != nil {
		t.Fatalf("LoadTriggerState for missing file should not error, got: %v", err)
	}

	if state == nil {
		t.Fatal("expected non-nil state")
	}

	if len(state.LastInjected) != 0 {
		t.Errorf("expected empty LastInjected, got %d entries", len(state.LastInjected))
	}

	// Verify the empty state still works.
	if !state.ShouldInject("anything", "1h") {
		t.Error("expected ShouldInject to return true for never-injected engram")
	}
}

func TestSaveCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "state.json")

	// Ensure parent dir exists for this test.
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	s := NewTriggerState()
	if err := s.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected file to be created")
	}
}
