package ops

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func setupTestRetryDir(t *testing.T) string {
	tmpDir := t.TempDir()
	t.Setenv("AGM_RETRY_BASE_DIR", tmpDir)
	t.Cleanup(func() {
		os.Unsetenv("AGM_RETRY_BASE_DIR")
	})
	return tmpDir
}

func TestRecoverPermissionPromptStall_NoOrchestrator(t *testing.T) {
	recovery := NewStallRecovery(&OpContext{}, "")
	event := StallEvent{
		SessionName: "test-session",
		StallType:   "permission_prompt",
		Duration:    10 * time.Minute,
		Evidence:    "Permission dialog open for 10m",
	}

	action, err := recovery.recoverPermissionPromptStall(context.Background(), event, "")
	if err == nil {
		t.Error("Expected error when orchestrator not configured")
	}
	if action.ActionType != "alert_orchestrator" {
		t.Errorf("ActionType = %v, want alert_orchestrator", action.ActionType)
	}
}

func TestRecoverNoCommitStall(t *testing.T) {
	mockStore := &mockStorage{
		sessions: []*manifest.Manifest{
			testManifest("worker-1", manifest.StateWorking, time.Now()),
		},
	}
	recovery := NewStallRecovery(&OpContext{Storage: mockStore}, "orchestrator")
	event := StallEvent{
		SessionName: "worker-1",
		StallType:   "no_commit",
		Duration:    20 * time.Minute,
		Evidence:    "No commits in 20m",
	}

	action, err := recovery.recoverNoCommitStall(context.Background(), event, "")
	if err != nil {
		t.Fatalf("recoverNoCommitStall() error = %v", err)
	}
	if action.ActionType != "nudge" {
		t.Errorf("ActionType = %v, want nudge", action.ActionType)
	}
	// Sent will be false because our mock doesn't implement delivery — that's expected.
}

func TestRecoverErrorLoopStall_WithOrchestrator(t *testing.T) {
	mockStore := &mockStorage{
		sessions: []*manifest.Manifest{
			testManifest("worker-2", manifest.StateWorking, time.Now()),
			testManifest("orchestrator", manifest.StateReady, time.Now()),
		},
	}
	recovery := NewStallRecovery(&OpContext{Storage: mockStore}, "orchestrator")
	event := StallEvent{
		SessionName: "worker-2",
		StallType:   "error_loop",
		Evidence:    "Error: permission denied appears 3 times",
	}

	injectDeliverableRouter(t, recovery)

	action, err := recovery.recoverErrorLoopStall(context.Background(), event, "")
	if err != nil {
		t.Fatalf("recoverErrorLoopStall() error = %v", err)
	}
	if action.ActionType != "log_diagnostic" {
		t.Errorf("ActionType = %v, want log_diagnostic", action.ActionType)
	}
	if !action.Sent {
		t.Error("action.Sent = false after a successful dispatch")
	}
}

// With no supervisor reachable the diagnostic is recorded durably but
// reaches nobody. It must report as not sent: counting the queue write as
// delivery would let Recover publish StallRecovered and stop the retry
// tracker from ever advancing toward max-retry escalation.
func TestRecoverErrorLoopStall_NoSupervisorIsNotDelivery(t *testing.T) {
	recovery := NewStallRecovery(&OpContext{}, "")
	queue := filepath.Join(t.TempDir(), "alerts.jsonl")
	router := NewAlertRouter(recovery.ctx)
	router.SetQueuePath(queue)
	recovery.SetAlertRouter(router)

	event := StallEvent{
		SessionName: "worker-2",
		StallType:   "error_loop",
		Evidence:    "Error: timeout appears 5 times",
	}

	action, err := recovery.recoverErrorLoopStall(context.Background(), event, "")
	if err == nil {
		t.Fatal("recoverErrorLoopStall() error = nil, want an undelivered-diagnostic error")
	}
	if action.Sent {
		t.Error("action.Sent = true for a diagnostic nobody received")
	}

	// It must still be durable, so a later drain can deliver it.
	records, readErr := ReadAlertRecords(queue, 10)
	if readErr != nil {
		t.Fatalf("ReadAlertRecords() error = %v", readErr)
	}
	if len(records) != 1 || records[0].Status != AlertStatusQueued {
		t.Fatalf("records = %v, want one queued record retained for retry", records)
	}
}

func TestIsSafeForAutoApproval(t *testing.T) {
	recovery := NewStallRecovery(&OpContext{}, "")

	tests := []struct {
		evidence string
		want     bool
	}{
		{"permission denied in git status", true},
		{"error running ls command", true},
		{"cat: cannot open file", true},
		{"unknown error in curl request", false},
		{"timeout waiting for response", false},
	}

	for _, tt := range tests {
		if got := recovery.isSafeForAutoApproval(tt.evidence); got != tt.want {
			t.Errorf("isSafeForAutoApproval(%q) = %v, want %v", tt.evidence, got, tt.want)
		}
	}
}

func TestRecover_PermissionPromptStall(t *testing.T) {
	setupTestRetryDir(t)
	mockStore := &mockStorage{
		sessions: []*manifest.Manifest{
			testManifest("test-session", manifest.StatePermissionPrompt, time.Now()),
			testManifest("orchestrator", manifest.StateReady, time.Now()),
		},
	}
	recovery := NewStallRecovery(&OpContext{Storage: mockStore}, "orchestrator")
	// Inject the delivery seam: this test is about the recovery decision,
	// not about whether a tmux pane exists in the test environment.
	sent := injectDeliverableRouter(t, recovery)

	event := StallEvent{
		SessionName: "test-session",
		StallType:   "permission_prompt",
		Duration:    10 * time.Minute,
	}

	action, err := recovery.Recover(context.Background(), event)
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if action.ActionType != "alert_orchestrator" {
		t.Errorf("ActionType = %v, want alert_orchestrator", action.ActionType)
	}
	if !action.Sent {
		t.Error("action.Sent = false after a successful dispatch")
	}
	if len(*sent) != 1 || (*sent)[0] != "orchestrator" {
		t.Errorf("sent = %v, want delivery to the pinned orchestrator", *sent)
	}
}

func TestRecover_UnknownStall(t *testing.T) {
	recovery := NewStallRecovery(&OpContext{}, "")
	event := StallEvent{
		SessionName: "test-session",
		StallType:   "unknown_type",
	}

	_, err := recovery.Recover(context.Background(), event)
	if err == nil {
		t.Error("Expected error for unknown stall type")
	}
}

// injectDeliverableRouter gives a StallRecovery an isolated alert queue and a
// delivery seam that always succeeds, so recovery-decision tests do not
// depend on a live tmux pane existing in the test environment, and never
// touch the developer's real alert queue.
func injectDeliverableRouter(t *testing.T, recovery *StallRecovery) *[]string {
	t.Helper()
	if recovery.ctx.Storage == nil {
		// Routing needs a discoverable live supervisor before it can call
		// the send seam at all.
		recovery.ctx.Storage = &mockStorage{sessions: []*manifest.Manifest{
			testManifest("vroom-orchestrator", manifest.StateReady, time.Now()),
		}}
	}
	router := NewAlertRouter(recovery.ctx)
	router.SetQueuePath(filepath.Join(t.TempDir(), "alerts.jsonl"))
	var sent []string
	router.sendMessage = func(_ context.Context, recipient, _ string) error {
		sent = append(sent, recipient)
		return nil
	}
	recovery.SetAlertRouter(router)
	return &sent
}

// injectIsolatedRouter gives a StallRecovery its own alert queue without
// making delivery possible, for tests that assert the undelivered path.
func injectIsolatedRouter(t *testing.T, recovery *StallRecovery) *AlertRouter {
	t.Helper()
	router := NewAlertRouter(recovery.ctx)
	router.SetQueuePath(filepath.Join(t.TempDir(), "alerts.jsonl"))
	recovery.SetAlertRouter(router)
	return router
}
