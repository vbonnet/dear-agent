package delegation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTracker_RecordAndPending(t *testing.T) {
	dir := t.TempDir()
	tracker, err := NewTracker(dir)
	if err != nil {
		t.Fatal(err)
	}

	// No pending delegations initially
	pending, err := tracker.Pending("test-session")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending, got %d", len(pending))
	}

	// Record a delegation
	d := &Delegation{
		MessageID:   "msg-001",
		From:        "test-session",
		To:          "worker-1",
		TaskSummary: "implement feature X",
	}
	if err := tracker.Record(d); err != nil {
		t.Fatal(err)
	}

	// Should have 1 pending
	pending, err = tracker.Pending("test-session")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
	if pending[0].MessageID != "msg-001" {
		t.Errorf("expected msg-001, got %s", pending[0].MessageID)
	}
	if pending[0].Status != StatusPending {
		t.Errorf("expected pending, got %s", pending[0].Status)
	}
}

func TestTracker_Resolve(t *testing.T) {
	dir := t.TempDir()
	tracker, err := NewTracker(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Record two delegations
	for _, id := range []string{"msg-001", "msg-002"} {
		d := &Delegation{
			MessageID:   id,
			From:        "research",
			To:          "worker",
			TaskSummary: "task " + id,
		}
		if err := tracker.Record(d); err != nil {
			t.Fatal(err)
		}
	}

	// Resolve first
	if err := tracker.Resolve("research", "msg-001", StatusCompleted); err != nil {
		t.Fatal(err)
	}

	// Only one pending now
	pending, err := tracker.Pending("research")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
	if pending[0].MessageID != "msg-002" {
		t.Errorf("expected msg-002, got %s", pending[0].MessageID)
	}

	// Resolve second
	if err := tracker.Resolve("research", "msg-002", StatusCompleted); err != nil {
		t.Fatal(err)
	}

	pending, err = tracker.Pending("research")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending, got %d", len(pending))
	}
}

func TestTracker_ResolveNotFound(t *testing.T) {
	dir := t.TempDir()
	tracker, err := NewTracker(dir)
	if err != nil {
		t.Fatal(err)
	}

	d := &Delegation{
		MessageID:   "msg-001",
		From:        "sess",
		To:          "worker",
		TaskSummary: "task",
	}
	if err := tracker.Record(d); err != nil {
		t.Fatal(err)
	}

	err = tracker.Resolve("sess", "nonexistent", StatusCompleted)
	if err == nil {
		t.Error("expected error for nonexistent message ID")
	}
}

func TestTracker_FileCreated(t *testing.T) {
	dir := t.TempDir()
	tracker, err := NewTracker(dir)
	if err != nil {
		t.Fatal(err)
	}

	d := &Delegation{
		MessageID:   "msg-001",
		From:        "my-session",
		To:          "worker",
		TaskSummary: "task",
	}
	if err := tracker.Record(d); err != nil {
		t.Fatal(err)
	}

	// Verify file exists
	path := filepath.Join(dir, "my-session.jsonl")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected delegation file to be created")
	}
}

func TestTracker_PendingNoFile(t *testing.T) {
	dir := t.TempDir()
	tracker, err := NewTracker(dir)
	if err != nil {
		t.Fatal(err)
	}

	// No file = no pending (not an error)
	pending, err := tracker.Pending("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0, got %d", len(pending))
	}
}

func TestTracker_Cancel(t *testing.T) {
	dir := t.TempDir()
	tracker, err := NewTracker(dir)
	if err != nil {
		t.Fatal(err)
	}

	d := &Delegation{
		MessageID:   "msg-001",
		From:        "sess",
		To:          "worker",
		TaskSummary: "task",
	}
	if err := tracker.Record(d); err != nil {
		t.Fatal(err)
	}

	if err := tracker.Resolve("sess", "msg-001", StatusCancelled); err != nil {
		t.Fatal(err)
	}

	pending, err := tracker.Pending("sess")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after cancel, got %d", len(pending))
	}
}
