package context

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadContext_FileNotExists(t *testing.T) {
	ctx, err := LoadContext("/nonexistent/path/session.json")
	if err != nil {
		t.Errorf("expected nil error for missing file, got %v", err)
	}
	if ctx == nil {
		t.Fatal("expected non-nil empty context")
	}
	if ctx.SessionID != "" {
		t.Errorf("expected empty SessionID, got %q", ctx.SessionID)
	}
}

func TestLoadContext_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bad.json")
	os.WriteFile(path, []byte("not json{{{"), 0644)

	ctx, err := LoadContext(path)
	if err != nil {
		t.Errorf("expected nil error for invalid JSON, got %v", err)
	}
	if ctx == nil {
		t.Fatal("expected non-nil empty context")
	}
	if ctx.SessionID != "" {
		t.Errorf("expected empty SessionID, got %q", ctx.SessionID)
	}
}

func TestSaveAndLoadContext_Roundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "session.json")

	original := &SessionContext{
		SessionID:     "test-session-123",
		SessionName:   "coordinator",
		CurrentSwarm:  "memory-persistence",
		CurrentPhase:  "Phase 3",
		LastOperation: "created schema doc",
		ActiveTasks:   []string{"task 1", "task 2"},
		Notes:         []string{"note A"},
		ActiveWorkers: []WorkerInfo{
			{SessionName: "worker-1", Status: "active", Task: "implement API", StartedAt: time.Now()},
		},
		OpenBeads: []BeadInfo{
			{ID: "src-4vd", Title: "Design schema", Status: "in_progress", Priority: "P0"},
		},
	}

	err := SaveContext(path, original)
	if err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}

	loaded, err := LoadContext(path)
	if err != nil {
		t.Fatalf("LoadContext failed: %v", err)
	}

	if loaded.SessionID != original.SessionID {
		t.Errorf("SessionID: got %q, want %q", loaded.SessionID, original.SessionID)
	}
	if loaded.SessionName != original.SessionName {
		t.Errorf("SessionName: got %q, want %q", loaded.SessionName, original.SessionName)
	}
	if loaded.CurrentSwarm != original.CurrentSwarm {
		t.Errorf("CurrentSwarm: got %q, want %q", loaded.CurrentSwarm, original.CurrentSwarm)
	}
	if loaded.CurrentPhase != original.CurrentPhase {
		t.Errorf("CurrentPhase: got %q, want %q", loaded.CurrentPhase, original.CurrentPhase)
	}
	if loaded.LastOperation != original.LastOperation {
		t.Errorf("LastOperation: got %q, want %q", loaded.LastOperation, original.LastOperation)
	}
	if len(loaded.ActiveTasks) != 2 {
		t.Errorf("ActiveTasks: got %d items, want 2", len(loaded.ActiveTasks))
	}
	if len(loaded.ActiveWorkers) != 1 {
		t.Errorf("ActiveWorkers: got %d, want 1", len(loaded.ActiveWorkers))
	}
	if loaded.ActiveWorkers[0].SessionName != "worker-1" {
		t.Errorf("Worker name: got %q, want %q", loaded.ActiveWorkers[0].SessionName, "worker-1")
	}
	if len(loaded.OpenBeads) != 1 {
		t.Errorf("OpenBeads: got %d, want 1", len(loaded.OpenBeads))
	}
	if loaded.OpenBeads[0].ID != "src-4vd" {
		t.Errorf("Bead ID: got %q, want %q", loaded.OpenBeads[0].ID, "src-4vd")
	}
	if loaded.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set by SaveContext")
	}
}

func TestSaveContext_CreatesParentDirs(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "sub", "deep", "session.json")

	err := SaveContext(path, &SessionContext{SessionID: "test"})
	if err != nil {
		t.Fatalf("SaveContext should create parent dirs: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("context file should exist after save")
	}
}

func TestSaveContext_UpdatesTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "session.json")

	ctx := &SessionContext{SessionID: "ts-test"}
	before := time.Now().Add(-time.Second)

	err := SaveContext(path, ctx)
	if err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}

	if ctx.UpdatedAt.Before(before) {
		t.Error("UpdatedAt should be updated to current time")
	}
}

func TestAddWorker(t *testing.T) {
	ctx := &SessionContext{SessionID: "test"}
	ctx.AddWorker("worker-alpha", "implement feature X")

	if len(ctx.ActiveWorkers) != 1 {
		t.Fatalf("expected 1 worker, got %d", len(ctx.ActiveWorkers))
	}
	w := ctx.ActiveWorkers[0]
	if w.SessionName != "worker-alpha" {
		t.Errorf("worker name: got %q, want %q", w.SessionName, "worker-alpha")
	}
	if w.Task != "implement feature X" {
		t.Errorf("worker task: got %q, want %q", w.Task, "implement feature X")
	}
	if w.Status != "active" {
		t.Errorf("worker status: got %q, want %q", w.Status, "active")
	}
	if w.StartedAt.IsZero() {
		t.Error("worker StartedAt should be set")
	}
}

func TestRemoveWorker(t *testing.T) {
	ctx := &SessionContext{
		SessionID: "test",
		ActiveWorkers: []WorkerInfo{
			{SessionName: "w1", Status: "active", Task: "task 1"},
			{SessionName: "w2", Status: "active", Task: "task 2"},
		},
	}

	ctx.RemoveWorker("w1")

	if ctx.ActiveWorkers[0].Status != "completed" {
		t.Errorf("w1 status: got %q, want %q", ctx.ActiveWorkers[0].Status, "completed")
	}
	if ctx.ActiveWorkers[1].Status != "active" {
		t.Errorf("w2 status should remain active, got %q", ctx.ActiveWorkers[1].Status)
	}
}

func TestRemoveWorker_NotFound(t *testing.T) {
	ctx := &SessionContext{
		SessionID: "test",
		ActiveWorkers: []WorkerInfo{
			{SessionName: "w1", Status: "active", Task: "task 1"},
		},
	}

	// Should not panic when worker is not found.
	ctx.RemoveWorker("nonexistent")

	if ctx.ActiveWorkers[0].Status != "active" {
		t.Error("existing worker should remain unchanged")
	}
}

func TestUpdateBeads(t *testing.T) {
	ctx := &SessionContext{
		SessionID: "test",
		OpenBeads: []BeadInfo{
			{ID: "old-1", Title: "Old bead", Status: "open", Priority: "P2"},
		},
	}

	newBeads := []BeadInfo{
		{ID: "src-abc", Title: "New bead A", Status: "in_progress", Priority: "P0"},
		{ID: "src-def", Title: "New bead B", Status: "open", Priority: "P1"},
	}

	ctx.UpdateBeads(newBeads)

	if len(ctx.OpenBeads) != 2 {
		t.Fatalf("expected 2 beads, got %d", len(ctx.OpenBeads))
	}
	if ctx.OpenBeads[0].ID != "src-abc" {
		t.Errorf("bead 0 ID: got %q, want %q", ctx.OpenBeads[0].ID, "src-abc")
	}
	if ctx.OpenBeads[1].ID != "src-def" {
		t.Errorf("bead 1 ID: got %q, want %q", ctx.OpenBeads[1].ID, "src-def")
	}
}

func TestAddNote(t *testing.T) {
	ctx := &SessionContext{SessionID: "test"}

	ctx.AddNote("first note")
	ctx.AddNote("second note")

	if len(ctx.Notes) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(ctx.Notes))
	}
	if ctx.Notes[0] != "first note" {
		t.Errorf("note 0: got %q, want %q", ctx.Notes[0], "first note")
	}
}

func TestAddNote_CapAt20(t *testing.T) {
	ctx := &SessionContext{SessionID: "test"}

	for i := range 25 {
		ctx.AddNote(strings.Repeat("x", i+1))
	}

	if len(ctx.Notes) != maxNotes {
		t.Fatalf("expected %d notes (capped), got %d", maxNotes, len(ctx.Notes))
	}

	// The first 5 notes should have been dropped (added 25, cap 20).
	// Remaining notes should start with the 6th one added (length 6).
	if len(ctx.Notes[0]) != 6 {
		t.Errorf("oldest surviving note should have length 6, got %d", len(ctx.Notes[0]))
	}
}

func TestSummary(t *testing.T) {
	ctx := &SessionContext{
		SessionID:     "sess-123",
		SessionName:   "coordinator",
		CurrentSwarm:  "memory-persistence",
		CurrentPhase:  "Phase 3",
		LastOperation: "wrote schema doc",
		ActiveWorkers: []WorkerInfo{
			{SessionName: "w1", Status: "active", Task: "implement API"},
			{SessionName: "w2", Status: "completed", Task: "write tests"},
		},
		OpenBeads: []BeadInfo{
			{ID: "src-4vd", Title: "Design schema", Status: "in_progress", Priority: "P0"},
		},
		ActiveTasks: []string{"task A", "task B"},
	}

	summary := ctx.Summary()

	checks := []string{
		"sess-123",
		"coordinator",
		"memory-persistence",
		"Phase 3",
		"wrote schema doc",
		"1 active",
		"w1: implement API",
		"src-4vd",
		"Design schema",
		"task A",
		"task B",
	}

	for _, check := range checks {
		if !strings.Contains(summary, check) {
			t.Errorf("summary missing %q\nGot:\n%s", check, summary)
		}
	}

	// Completed worker should not appear in active list.
	if strings.Contains(summary, "w2: write tests") {
		t.Error("completed worker w2 should not appear in active workers list")
	}
}

func TestSummary_Minimal(t *testing.T) {
	ctx := &SessionContext{SessionID: "minimal"}
	summary := ctx.Summary()

	if !strings.Contains(summary, "minimal") {
		t.Errorf("minimal summary should contain session ID, got:\n%s", summary)
	}
	if strings.Contains(summary, "Swarm:") {
		t.Error("summary should not show Swarm line when empty")
	}
}

func TestContextPath(t *testing.T) {
	path := Path("/tmp/test/.claude/session-context", "abc-123")
	expected := "/tmp/test/.claude/session-context/abc-123.json"
	if path != expected {
		t.Errorf("got %q, want %q", path, expected)
	}
}

func TestPruneOldContexts(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an "old" file by writing it and then backdating its mod time.
	oldPath := filepath.Join(tmpDir, "old-session.json")
	os.WriteFile(oldPath, []byte(`{"session_id":"old"}`), 0644)
	oldTime := time.Now().Add(-48 * time.Hour)
	os.Chtimes(oldPath, oldTime, oldTime)

	// Create a "recent" file.
	recentPath := filepath.Join(tmpDir, "recent-session.json")
	os.WriteFile(recentPath, []byte(`{"session_id":"recent"}`), 0644)

	err := PruneOldContexts(tmpDir, 24*time.Hour)
	if err != nil {
		t.Fatalf("PruneOldContexts failed: %v", err)
	}

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("old context file should have been pruned")
	}
	if _, err := os.Stat(recentPath); os.IsNotExist(err) {
		t.Error("recent context file should survive pruning")
	}
}

func TestPruneOldContexts_NonexistentDir(t *testing.T) {
	err := PruneOldContexts("/nonexistent/dir", 24*time.Hour)
	if err != nil {
		t.Errorf("should return nil for nonexistent dir, got: %v", err)
	}
}

func TestPruneOldContexts_SkipsNonJSON(t *testing.T) {
	tmpDir := t.TempDir()

	txtPath := filepath.Join(tmpDir, "notes.txt")
	os.WriteFile(txtPath, []byte("not json"), 0644)
	oldTime := time.Now().Add(-48 * time.Hour)
	os.Chtimes(txtPath, oldTime, oldTime)

	err := PruneOldContexts(tmpDir, 24*time.Hour)
	if err != nil {
		t.Fatalf("PruneOldContexts failed: %v", err)
	}

	if _, err := os.Stat(txtPath); os.IsNotExist(err) {
		t.Error("non-JSON file should not be pruned")
	}
}

func TestSessionContext_JSONMarshal(t *testing.T) {
	ctx := &SessionContext{
		SessionID:   "marshal-test",
		SessionName: "test-session",
		UpdatedAt:   time.Now(),
	}

	data, err := json.Marshal(ctx)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var roundtrip SessionContext
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if roundtrip.SessionID != ctx.SessionID {
		t.Errorf("roundtrip SessionID: got %q, want %q", roundtrip.SessionID, ctx.SessionID)
	}
}
