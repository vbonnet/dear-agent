package verification

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadState_FileNotExists(t *testing.T) {
	state := LoadState("/nonexistent/path/state.json")
	if len(state.Pending) != 0 {
		t.Errorf("expected empty state, got %d pending", len(state.Pending))
	}
}

func TestLoadState_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "state.json")
	os.WriteFile(path, []byte("not json"), 0644)

	state := LoadState(path)
	if len(state.Pending) != 0 {
		t.Errorf("expected empty state for invalid JSON, got %d pending", len(state.Pending))
	}
}

func TestSaveAndLoadState(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "state.json")

	state := State{
		Pending: []PendingVerification{
			{
				ID:         "test-1",
				Type:       "bead_close",
				Message:    "5 remaining beads",
				SwarmLabel: "my-swarm",
				BeadID:     "src-123",
			},
		},
	}

	err := SaveState(path, state)
	if err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	loaded := LoadState(path)
	if len(loaded.Pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(loaded.Pending))
	}
	if loaded.Pending[0].ID != "test-1" {
		t.Errorf("expected ID test-1, got %s", loaded.Pending[0].ID)
	}
	if loaded.Pending[0].SwarmLabel != "my-swarm" {
		t.Errorf("expected swarm my-swarm, got %s", loaded.Pending[0].SwarmLabel)
	}
}

func TestSaveState_CreatesDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "sub", "dir", "state.json")

	err := SaveState(path, State{})
	if err != nil {
		t.Fatalf("SaveState should create parent dirs: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("state file should exist after save")
	}
}

func TestAddPending(t *testing.T) {
	state := State{}
	state.AddPending(PendingVerification{
		ID:      "v1",
		Type:    "bead_close",
		Message: "test",
	})

	if len(state.Pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(state.Pending))
	}
	if state.Pending[0].ToolUsesSince != 0 {
		t.Error("new pending should have 0 tool uses")
	}
	if state.Pending[0].CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestIncrementAll(t *testing.T) {
	state := State{
		Pending: []PendingVerification{
			{ID: "v1", ToolUsesSince: 2},
			{ID: "v2", ToolUsesSince: 0},
		},
	}

	state.IncrementAll()

	if state.Pending[0].ToolUsesSince != 3 {
		t.Errorf("expected 3, got %d", state.Pending[0].ToolUsesSince)
	}
	if state.Pending[1].ToolUsesSince != 1 {
		t.Errorf("expected 1, got %d", state.Pending[1].ToolUsesSince)
	}
}

func TestRemoveByType(t *testing.T) {
	state := State{
		Pending: []PendingVerification{
			{ID: "v1", Type: "bead_close"},
			{ID: "v2", Type: "notification_send"},
			{ID: "v1", Type: "notification_send"},
		},
	}

	state.RemoveByType("bead_close", "v1")

	if len(state.Pending) != 2 {
		t.Fatalf("expected 2 remaining, got %d", len(state.Pending))
	}
	for _, v := range state.Pending {
		if v.Type == "bead_close" && v.ID == "v1" {
			t.Error("should have removed bead_close v1")
		}
	}
}

func TestRemoveBySwarm(t *testing.T) {
	state := State{
		Pending: []PendingVerification{
			{ID: "v1", Type: "bead_close", SwarmLabel: "swarm-a"},
			{ID: "v2", Type: "bead_close", SwarmLabel: "swarm-b"},
			{ID: "v3", Type: "notification_send"},
		},
	}

	state.RemoveBySwarm("swarm-a")

	if len(state.Pending) != 2 {
		t.Fatalf("expected 2 remaining, got %d", len(state.Pending))
	}
}

func TestPruneOld(t *testing.T) {
	now := time.Now()
	state := State{
		Pending: []PendingVerification{
			{ID: "old", CreatedAt: now.Add(-2 * time.Hour)},
			{ID: "recent", CreatedAt: now.Add(-5 * time.Minute)},
		},
	}

	state.PruneOld(1 * time.Hour)

	if len(state.Pending) != 1 {
		t.Fatalf("expected 1 remaining after prune, got %d", len(state.Pending))
	}
	if state.Pending[0].ID != "recent" {
		t.Errorf("expected recent to survive, got %s", state.Pending[0].ID)
	}
}
