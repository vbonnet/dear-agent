package integration_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sessionctx "github.com/vbonnet/dear-agent/engram/hooks-bin/internal/context"
	"github.com/vbonnet/dear-agent/engram/hooks-bin/internal/heartbeat"
	"github.com/vbonnet/dear-agent/engram/hooks-bin/internal/verification"
)

// TestScenario_PassiveWaiting verifies that when a Bash operation takes >30s,
// the heartbeat system records and reports it. We pre-seed heartbeat state with
// a 45-second-old operation, then verify the state has the operation tracked
// with correct duration.
func TestScenario_PassiveWaiting(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "heartbeat-state.json")

	// Pre-seed state with a Bash operation that started 45 seconds ago.
	started := time.Now().Add(-45 * time.Second)
	state := heartbeat.State{
		Operation: &heartbeat.OperationState{
			ToolName:       "Bash",
			StartedAt:      started,
			CommandPreview: "go test ./...",
		},
	}
	if err := heartbeat.SaveState(statePath, state); err != nil {
		t.Fatalf("SaveState() error: %v", err)
	}

	// Load it back and verify the operation is tracked.
	loaded := heartbeat.LoadState(statePath)
	if loaded.Operation == nil {
		t.Fatal("expected Operation to be tracked, got nil")
	}
	if loaded.Operation.ToolName != "Bash" {
		t.Errorf("Operation.ToolName = %q, want %q", loaded.Operation.ToolName, "Bash")
	}

	// The duration since the recorded start time should be >= 45s.
	dur := time.Since(loaded.Operation.StartedAt)
	if dur < 44*time.Second {
		t.Errorf("operation duration = %v, want >= 44s (passive waiting scenario)", dur)
	}
}

// TestScenario_SilentBatchOperations verifies that 5+ consecutive Edit
// operations trigger the batch detection system. After 3 consecutive file
// ops, IsInBatch() should return true.
func TestScenario_SilentBatchOperations(t *testing.T) {
	state := heartbeat.State{}
	params := map[string]interface{}{"file_path": "/tmp/test.go"}

	for i := 1; i <= 5; i++ {
		state.RecordToolUse("Edit", params)

		if i < 3 {
			if state.IsInBatch() {
				t.Errorf("IsInBatch() = true after %d ops, want false (threshold is 3)", i)
			}
		} else {
			if !state.IsInBatch() {
				t.Errorf("IsInBatch() = false after %d ops, want true (threshold is 3)", i)
			}
		}
	}

	if state.Batch == nil {
		t.Fatal("Batch should not be nil after 5 Edit operations")
	}
	if state.Batch.Count != 5 {
		t.Errorf("Batch.Count = %d, want 5", state.Batch.Count)
	}
}

// TestScenario_ContextLoss verifies that session context persists across a
// simulated "restart". We save a context with SessionID, CurrentSwarm, and
// OpenBeads, then load it back and verify all fields survived.
func TestScenario_ContextLoss(t *testing.T) {
	tmpDir := t.TempDir()
	ctxPath := filepath.Join(tmpDir, "session-ctx.json")

	original := &sessionctx.SessionContext{
		SessionID:    "sess-abc-123",
		SessionName:  "memory-persistence-worker",
		CurrentSwarm: "memory-persistence",
		CurrentPhase: "phase-3-integration",
		OpenBeads: []sessionctx.BeadInfo{
			{ID: "MP-001", Title: "heartbeat hook", Status: "in_progress", Priority: "P1"},
			{ID: "MP-002", Title: "context saver", Status: "open", Priority: "P1"},
		},
		ActiveTasks:   []string{"implement heartbeat", "write tests"},
		LastOperation: "created tracker.go",
	}

	if err := sessionctx.SaveContext(ctxPath, original); err != nil {
		t.Fatalf("SaveContext() error: %v", err)
	}

	// Simulate restart: load context from disk.
	loaded, err := sessionctx.LoadContext(ctxPath)
	if err != nil {
		t.Fatalf("LoadContext() error: %v", err)
	}

	if loaded.SessionID != original.SessionID {
		t.Errorf("SessionID = %q, want %q", loaded.SessionID, original.SessionID)
	}
	if loaded.CurrentSwarm != original.CurrentSwarm {
		t.Errorf("CurrentSwarm = %q, want %q", loaded.CurrentSwarm, original.CurrentSwarm)
	}
	if loaded.CurrentPhase != original.CurrentPhase {
		t.Errorf("CurrentPhase = %q, want %q", loaded.CurrentPhase, original.CurrentPhase)
	}
	if len(loaded.OpenBeads) != 2 {
		t.Fatalf("OpenBeads len = %d, want 2", len(loaded.OpenBeads))
	}
	if loaded.OpenBeads[0].ID != "MP-001" {
		t.Errorf("OpenBeads[0].ID = %q, want %q", loaded.OpenBeads[0].ID, "MP-001")
	}
	if loaded.OpenBeads[1].Status != "open" {
		t.Errorf("OpenBeads[1].Status = %q, want %q", loaded.OpenBeads[1].Status, "open")
	}
	if len(loaded.ActiveTasks) != 2 {
		t.Errorf("ActiveTasks len = %d, want 2", len(loaded.ActiveTasks))
	}
	if loaded.LastOperation != original.LastOperation {
		t.Errorf("LastOperation = %q, want %q", loaded.LastOperation, original.LastOperation)
	}
}

// TestScenario_ForgottenVerification verifies that pending verifications are
// tracked and that CheckEscalations returns results when ToolUsesSince
// exceeds the escalation threshold.
func TestScenario_ForgottenVerification(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "verification-state.json")

	state := verification.State{
		Pending: []verification.PendingVerification{
			{
				ID:            "v-001",
				Type:          "bead_close",
				CreatedAt:     time.Now().Add(-10 * time.Minute),
				Message:       "Bead MP-001 closed but successor not picked",
				ToolUsesSince: verification.EscalationThreshold + 2, // exceeds threshold
				SwarmLabel:    "memory-persistence",
				BeadID:        "MP-001",
			},
			{
				ID:            "v-002",
				Type:          "notification_send",
				CreatedAt:     time.Now().Add(-5 * time.Minute),
				Message:       "Notification sent to coordinator",
				ToolUsesSince: 1, // below threshold
				Recipient:     "coordinator",
			},
		},
	}

	if err := verification.SaveState(statePath, state); err != nil {
		t.Fatalf("SaveState() error: %v", err)
	}

	// Reload and check escalations.
	loaded := verification.LoadState(statePath)
	results := verification.CheckEscalations(&loaded)

	if len(results) != 1 {
		t.Fatalf("CheckEscalations() returned %d results, want 1 (only the one above threshold)", len(results))
	}

	r := results[0]
	if !r.Escalated {
		t.Error("expected Escalated = true")
	}
	if r.Verification.ID != "v-001" {
		t.Errorf("escalated verification ID = %q, want %q", r.Verification.ID, "v-001")
	}
	if r.Verification.ToolUsesSince < verification.EscalationThreshold {
		t.Errorf("ToolUsesSince = %d, should be >= %d", r.Verification.ToolUsesSince, verification.EscalationThreshold)
	}
	if r.Message == "" {
		t.Error("escalation Message should not be empty")
	}
}

// TestScenario_TaskTrackingGap verifies that HeartbeatState.Tasks tracks
// TaskCreate usage. When Tasks is nil (no task creates recorded), this is the
// condition that would trigger a prompt reminding the agent to create tasks.
func TestScenario_TaskTrackingGap(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "heartbeat-state.json")

	// Simulate a session with several tool uses but zero task creates.
	state := heartbeat.State{
		Batch: &heartbeat.BatchState{
			Count:    8,
			ToolType: "Edit",
		},
	}

	if err := heartbeat.SaveState(statePath, state); err != nil {
		t.Fatalf("SaveState() error: %v", err)
	}

	loaded := heartbeat.LoadState(statePath)

	// Tasks is nil = no TaskCreate calls have been made.
	// This is the trigger condition for the "no tasks created" prompt.
	if loaded.Tasks != nil {
		t.Error("Tasks should be nil when no TaskCreate calls have been made")
	}

	// Verify the gap detection: marshal to JSON and confirm tasks key is absent.
	data, err := json.Marshal(loaded)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}
	if strings.Contains(string(data), `"tasks"`) {
		t.Error("JSON should not contain 'tasks' key when Tasks is nil (omitempty)")
	}

	// Now simulate a TaskCreate by populating the field.
	loaded.Tasks = &heartbeat.TaskTrackingState{
		TotalCreated:    1,
		LastTaskSubject: "Fix login bug",
		LastUpdatedAt:   time.Now(),
	}
	if loaded.Tasks == nil {
		t.Error("Tasks should not be nil after setting it")
	}
	if loaded.Tasks.TotalCreated != 1 {
		t.Errorf("TotalCreated = %d, want 1", loaded.Tasks.TotalCreated)
	}
}

// TestScenario_ContextDisplayOnResume verifies that a saved context with
// swarm, workers, and beads produces a non-empty Summary(). This ensures
// the session-start display hook has content to show on resume.
func TestScenario_ContextDisplayOnResume(t *testing.T) {
	ctx := &sessionctx.SessionContext{
		SessionID:     "sess-resume-test",
		SessionName:   "swarm-coordinator",
		CurrentSwarm:  "memory-persistence",
		CurrentPhase:  "phase-2-implementation",
		LastOperation: "completed heartbeat tracker",
		ActiveTasks:   []string{"write integration tests", "deploy hooks"},
		OpenBeads: []sessionctx.BeadInfo{
			{ID: "MP-003", Title: "integration tests", Status: "in_progress", Priority: "P1"},
			{ID: "MP-004", Title: "deploy pipeline", Status: "open", Priority: "P2"},
		},
	}
	ctx.AddWorker("worker-hooks", "implementing heartbeat hook")
	ctx.AddWorker("worker-tests", "writing unit tests")

	summary := ctx.Summary()

	if summary == "" {
		t.Fatal("Summary() should not be empty for a rich context")
	}

	requiredFields := []struct {
		label string
		value string
	}{
		{"session ID", "sess-resume-test"},
		{"swarm name", "memory-persistence"},
		{"phase", "phase-2-implementation"},
		{"worker count", "2 active"},
		{"worker name", "worker-hooks"},
		{"bead count", "2 tracked"},
		{"bead ID", "MP-003"},
		{"task", "write integration tests"},
		{"last operation", "completed heartbeat tracker"},
	}

	for _, rf := range requiredFields {
		if !strings.Contains(summary, rf.value) {
			t.Errorf("Summary() missing %s (%q) in output:\n%s", rf.label, rf.value, summary)
		}
	}
}
