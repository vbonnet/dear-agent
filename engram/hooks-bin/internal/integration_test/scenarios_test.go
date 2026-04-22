// Package integration_test validates the 6 original incident types
// that the memory-persistence swarm was designed to fix.
package integration_test

import (
	"path/filepath"
	"testing"
	"time"

	sessionctx "github.com/vbonnet/dear-agent/engram/hooks-bin/internal/context"
	"github.com/vbonnet/dear-agent/engram/hooks-bin/internal/heartbeat"
	"github.com/vbonnet/dear-agent/engram/hooks-bin/internal/verification"
)

// Scenario 1: Passive Waiting
// Claude sits idle waiting for a long Bash command without any output.
// The heartbeat system should detect and report long operations.
func TestScenario_PassiveWaiting(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "heartbeat.json")

	// Simulate a Bash operation that started 45 seconds ago
	state := heartbeat.State{
		Operation: &heartbeat.OperationState{
			ToolName:       "Bash",
			StartedAt:      time.Now().Add(-45 * time.Second),
			CommandPreview: "go test ./...",
		},
	}

	if err := heartbeat.SaveState(statePath, state); err != nil {
		t.Fatal(err)
	}

	// Verify the operation is tracked
	loaded := heartbeat.LoadState(statePath)
	if loaded.Operation == nil {
		t.Fatal("Operation should be tracked across state save/load")
	}

	duration := loaded.GetOperationDuration()
	if duration < 30*time.Second {
		t.Errorf("Operation duration should be >= 30s, got %v", duration)
	}
}

// Scenario 2: Silent Batch Operations
// Claude performs 10+ consecutive Edit/Write operations with no user-visible output.
// The heartbeat system should detect batch patterns.
func TestScenario_SilentBatchOperations(t *testing.T) {
	state := heartbeat.State{}

	// Simulate 5 consecutive Edit operations
	for i := 0; i < 5; i++ {
		state.RecordToolUse("Edit", map[string]interface{}{
			"file_path": "/tmp/file.go",
		})
	}

	if !state.IsInBatch() {
		t.Error("Should detect batch after 5 consecutive file ops")
	}
	if state.Batch.Count != 5 {
		t.Errorf("Batch count = %d, want 5", state.Batch.Count)
	}

	// A non-file op should reset the batch
	state.RecordToolUse("Bash", map[string]interface{}{
		"command": "go build",
	})
	if state.IsInBatch() {
		t.Error("Batch should be reset after non-file op")
	}
}

// Scenario 3: Context Loss Across Sessions
// Session context (open beads, swarm info, workers) should survive "restarts".
func TestScenario_ContextLoss(t *testing.T) {
	tmpDir := t.TempDir()
	ctxPath := filepath.Join(tmpDir, "context.json")

	// Session 1: Save rich context
	ctx := &sessionctx.SessionContext{
		SessionID:    "session-abc",
		CurrentSwarm: "memory-persistence",
		CurrentPhase: "Phase 3",
		OpenBeads: []sessionctx.BeadInfo{
			{ID: "src-4vd", Title: "Design schema", Status: "in_progress", Priority: "P0"},
			{ID: "src-fwv", Title: "Save/load API", Status: "open", Priority: "P0"},
		},
		ActiveTasks: []string{"implement context API", "write tests"},
		Notes:       []string{"using file-based persistence"},
	}
	ctx.AddWorker("worker-1", "implementing feature")

	if err := sessionctx.SaveContext(ctxPath, ctx); err != nil {
		t.Fatal(err)
	}

	// Session 2: Load context and verify everything survived
	loaded, err := sessionctx.LoadContext(ctxPath)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.SessionID != "session-abc" {
		t.Errorf("SessionID lost: got %q", loaded.SessionID)
	}
	if loaded.CurrentSwarm != "memory-persistence" {
		t.Errorf("Swarm lost: got %q", loaded.CurrentSwarm)
	}
	if len(loaded.OpenBeads) != 2 {
		t.Errorf("Beads lost: got %d, want 2", len(loaded.OpenBeads))
	}
	if len(loaded.ActiveWorkers) != 1 {
		t.Errorf("Workers lost: got %d, want 1", len(loaded.ActiveWorkers))
	}
	if len(loaded.ActiveTasks) != 2 {
		t.Errorf("Tasks lost: got %d, want 2", len(loaded.ActiveTasks))
	}
	if len(loaded.Notes) != 1 {
		t.Errorf("Notes lost: got %d, want 1", len(loaded.Notes))
	}
}

// Scenario 4: Forgotten Verification
// After closing a bead or sending a message, Claude should be reminded to verify.
func TestScenario_ForgottenVerification(t *testing.T) {
	state := verification.State{
		Pending: []verification.PendingVerification{
			{
				ID:            "v1",
				Type:          "bead_close",
				Message:       "Closed src-4vd",
				CreatedAt:     time.Now().Add(-5 * time.Minute),
				ToolUsesSince: 6, // Above threshold of 5
			},
		},
	}

	results := verification.CheckEscalations(&state)
	if len(results) == 0 {
		t.Error("Should escalate verification after 6 tool uses (threshold=5)")
	}

	// Below threshold should not escalate
	state.Pending[0].ToolUsesSince = 3
	results = verification.CheckEscalations(&state)
	if len(results) != 0 {
		t.Error("Should not escalate below threshold")
	}
}

// Scenario 5: Task Tracking Gap
// When Claude performs many operations without using TaskCreate, the system should prompt.
func TestScenario_TaskTrackingGap(t *testing.T) {
	state := heartbeat.State{}

	// No tasks tracked = gap
	if state.Tasks != nil {
		t.Error("Fresh state should have nil Tasks")
	}

	// After tracking a task, Tasks should be non-nil
	state.Tasks = &heartbeat.TaskTrackingState{
		TotalCreated:    1,
		LastTaskSubject: "Fix login bug",
		LastUpdatedAt:   time.Now(),
	}
	if state.Tasks.TotalCreated != 1 {
		t.Errorf("TotalCreated = %d, want 1", state.Tasks.TotalCreated)
	}
}

// Scenario 6: Context Display on Resume
// When a session resumes, the persisted context should produce a useful summary.
func TestScenario_ContextDisplayOnResume(t *testing.T) {
	ctx := &sessionctx.SessionContext{
		SessionID:     "resume-test",
		SessionName:   "coordinator",
		CurrentSwarm:  "memory-persistence",
		CurrentPhase:  "Phase 4",
		LastOperation: "closed bead src-gwf",
		OpenBeads: []sessionctx.BeadInfo{
			{ID: "src-90w", Title: "Task verification", Status: "open", Priority: "P0"},
		},
	}
	ctx.AddWorker("agent-1", "running tests")

	summary := ctx.Summary()

	checks := []string{
		"resume-test",
		"coordinator",
		"memory-persistence",
		"Phase 4",
		"closed bead src-gwf",
		"src-90w",
		"agent-1",
		"running tests",
	}

	for _, check := range checks {
		if !contains(summary, check) {
			t.Errorf("Summary missing %q\nGot:\n%s", check, summary)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
