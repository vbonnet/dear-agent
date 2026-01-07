package taskqueue

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func setupTestQueue(t *testing.T) (string, *Coordinator) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "test-queue.yaml")

	// Create initial queue
	yamlContent := `schema_version: "1.0"
last_updated: 2026-01-07T00:00:00Z
ready:
  - id: "bead-1"
    tier: 1
    title: "First Task"
    prompts:
      start: "Start first"
  - id: "bead-2"
    tier: 2
    title: "Second Task"
    prompts:
      start: "Start second"
in_progress: []
blocked:
  - id: "bead-3"
    tier: 2
    title: "Blocked Task"
    depends_on: ["bead-1"]
    prompts:
      start: "Start after bead-1"
completed: []
`
	if err := os.WriteFile(queuePath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test queue: %v", err)
	}

	coordinator := NewCoordinator(queuePath)
	if err := coordinator.Load(); err != nil {
		t.Fatalf("failed to load coordinator: %v", err)
	}

	return queuePath, coordinator
}

func TestCoordinator_Load(t *testing.T) {
	_, coordinator := setupTestQueue(t)

	queue := coordinator.GetQueue()
	if queue == nil {
		t.Fatal("queue should not be nil after Load()")
	}

	if len(queue.Ready) != 2 {
		t.Errorf("ready beads: got %d, want 2", len(queue.Ready))
	}
	if len(queue.Blocked) != 1 {
		t.Errorf("blocked beads: got %d, want 1", len(queue.Blocked))
	}
}

func TestCoordinator_Save(t *testing.T) {
	queuePath, coordinator := setupTestQueue(t)

	// Modify queue
	if err := coordinator.Claim("bead-1", "test-session"); err != nil {
		t.Fatalf("failed to claim bead: %v", err)
	}

	// Save
	if err := coordinator.Save(); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	// Reload to verify persistence
	coordinator2 := NewCoordinator(queuePath)
	if err := coordinator2.Load(); err != nil {
		t.Fatalf("failed to reload: %v", err)
	}

	queue := coordinator2.GetQueue()
	if len(queue.Ready) != 1 {
		t.Errorf("after save: ready beads got %d, want 1", len(queue.Ready))
	}
	if len(queue.InProgress) != 1 {
		t.Errorf("after save: in_progress beads got %d, want 1", len(queue.InProgress))
	}
	if queue.InProgress[0].ID != "bead-1" {
		t.Errorf("in_progress[0].id: got %s, want bead-1", queue.InProgress[0].ID)
	}
}

func TestCoordinator_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "atomic-test.yaml")

	// Create initial file
	if err := os.WriteFile(queuePath, []byte("schema_version: \"1.0\"\nlast_updated: 2026-01-07T00:00:00Z\nready: []\nin_progress: []\nblocked: []\ncompleted: []\n"), 0644); err != nil {
		t.Fatalf("failed to create initial file: %v", err)
	}

	coordinator := NewCoordinator(queuePath)
	if err := coordinator.Load(); err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	// Save and check no temp file remains
	if err := coordinator.Save(); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	tmpPath := queuePath + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("temp file should not exist after successful save")
	}
}

func TestCoordinator_QueryNext(t *testing.T) {
	_, coordinator := setupTestQueue(t)

	bead, err := coordinator.QueryNext()
	if err != nil {
		t.Fatalf("QueryNext failed: %v", err)
	}

	if bead == nil {
		t.Fatal("QueryNext should return first ready bead")
	}

	if bead.ID != "bead-1" {
		t.Errorf("QueryNext: got %s, want bead-1", bead.ID)
	}
}

func TestCoordinator_QueryNext_EmptyReady(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "empty.yaml")

	emptyYAML := `schema_version: "1.0"
last_updated: 2026-01-07T00:00:00Z
ready: []
in_progress: []
blocked: []
completed: []
`
	if err := os.WriteFile(queuePath, []byte(emptyYAML), 0644); err != nil {
		t.Fatalf("failed to write empty queue: %v", err)
	}

	coordinator := NewCoordinator(queuePath)
	if err := coordinator.Load(); err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	bead, err := coordinator.QueryNext()
	if err != nil {
		t.Fatalf("QueryNext failed: %v", err)
	}

	if bead != nil {
		t.Error("QueryNext should return nil when ready section is empty")
	}
}

func TestCoordinator_Claim(t *testing.T) {
	_, coordinator := setupTestQueue(t)

	err := coordinator.Claim("bead-1", "test-session")
	if err != nil {
		t.Fatalf("Claim failed: %v", err)
	}

	queue := coordinator.GetQueue()
	if len(queue.Ready) != 1 {
		t.Errorf("after claim: ready beads got %d, want 1", len(queue.Ready))
	}
	if len(queue.InProgress) != 1 {
		t.Errorf("after claim: in_progress beads got %d, want 1", len(queue.InProgress))
	}

	// Verify metadata update
	claimed := queue.InProgress[0]
	if claimed.ID != "bead-1" {
		t.Errorf("claimed bead id: got %s, want bead-1", claimed.ID)
	}
	if claimed.Metadata.SessionName != "test-session" {
		t.Errorf("session_name: got %s, want test-session", claimed.Metadata.SessionName)
	}
	if claimed.Metadata.Iterations != 1 {
		t.Errorf("iterations: got %d, want 1", claimed.Metadata.Iterations)
	}
}

func TestCoordinator_Claim_NotFound(t *testing.T) {
	_, coordinator := setupTestQueue(t)

	err := coordinator.Claim("nonexistent", "test-session")
	if err == nil {
		t.Fatal("Claim should fail for nonexistent bead")
	}
}

func TestCoordinator_Complete(t *testing.T) {
	_, coordinator := setupTestQueue(t)

	// Claim first
	if err := coordinator.Claim("bead-1", "test-session"); err != nil {
		t.Fatalf("Claim failed: %v", err)
	}

	// Complete
	if err := coordinator.Complete("bead-1"); err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	queue := coordinator.GetQueue()
	if len(queue.InProgress) != 0 {
		t.Errorf("after complete: in_progress beads got %d, want 0", len(queue.InProgress))
	}
	if len(queue.Completed) != 1 {
		t.Errorf("after complete: completed beads got %d, want 1", len(queue.Completed))
	}
	if queue.Completed[0].ID != "bead-1" {
		t.Errorf("completed[0].id: got %s, want bead-1", queue.Completed[0].ID)
	}

	// Verify bead-3 was unblocked (depends on bead-1)
	if len(queue.Blocked) != 0 {
		t.Errorf("after complete: blocked beads got %d, want 0 (bead-3 should be unblocked)", len(queue.Blocked))
	}
	if len(queue.Ready) != 2 {
		t.Errorf("after complete: ready beads got %d, want 2 (bead-2 + unblocked bead-3)", len(queue.Ready))
	}
}

func TestCoordinator_Complete_NotFound(t *testing.T) {
	_, coordinator := setupTestQueue(t)

	err := coordinator.Complete("nonexistent")
	if err == nil {
		t.Fatal("Complete should fail for nonexistent bead")
	}
}

func TestCoordinator_Unblock(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "unblock-test.yaml")

	yamlContent := `schema_version: "1.0"
last_updated: 2026-01-07T00:00:00Z
ready: []
in_progress: []
blocked:
  - id: "bead-2"
    tier: 2
    title: "Depends on bead-1"
    depends_on: ["bead-1"]
    prompts:
      start: "Start after bead-1"
  - id: "bead-3"
    tier: 2
    title: "Depends on both"
    depends_on: ["bead-1", "bead-2"]
    prompts:
      start: "Start after both"
completed:
  - id: "bead-1"
    tier: 1
    title: "Completed"
    prompts:
      start: "Done"
`
	if err := os.WriteFile(queuePath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test queue: %v", err)
	}

	coordinator := NewCoordinator(queuePath)
	if err := coordinator.Load(); err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	// Unblock
	if err := coordinator.Unblock(); err != nil {
		t.Fatalf("Unblock failed: %v", err)
	}

	queue := coordinator.GetQueue()
	// bead-2 should be unblocked (only depends on bead-1 which is completed)
	// bead-3 should still be blocked (depends on bead-2 which is not completed)
	if len(queue.Ready) != 1 {
		t.Errorf("ready beads: got %d, want 1 (bead-2)", len(queue.Ready))
	}
	if len(queue.Blocked) != 1 {
		t.Errorf("blocked beads: got %d, want 1 (bead-3)", len(queue.Blocked))
	}
	if queue.Ready[0].ID != "bead-2" {
		t.Errorf("ready[0].id: got %s, want bead-2", queue.Ready[0].ID)
	}
}

func TestCoordinator_ConcurrentAccess(t *testing.T) {
	_, coordinator := setupTestQueue(t)

	var wg sync.WaitGroup
	numGoroutines := 10

	// Concurrent reads (QueryNext)
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_, _ = coordinator.QueryNext()
		}()
	}
	wg.Wait()

	// Concurrent writes (Claim)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = coordinator.Claim("bead-1", "session-1")
	}()
	go func() {
		defer wg.Done()
		_ = coordinator.Claim("bead-2", "session-2")
	}()
	wg.Wait()

	// Verify state is consistent
	queue := coordinator.GetQueue()
	totalBeads := len(queue.Ready) + len(queue.InProgress) + len(queue.Blocked) + len(queue.Completed)
	if totalBeads != 3 {
		t.Errorf("total beads after concurrent access: got %d, want 3", totalBeads)
	}
}

func TestCoordinator_LoadWithoutLoad(t *testing.T) {
	coordinator := NewCoordinator("/tmp/dummy.yaml")

	// Operations should fail if Load() not called
	if _, err := coordinator.QueryNext(); err == nil {
		t.Error("QueryNext should fail if Load() not called")
	}
	if err := coordinator.Claim("bead-1", "session"); err == nil {
		t.Error("Claim should fail if Load() not called")
	}
	if err := coordinator.Complete("bead-1"); err == nil {
		t.Error("Complete should fail if Load() not called")
	}
	if err := coordinator.Save(); err == nil {
		t.Error("Save should fail if Load() not called")
	}
}
