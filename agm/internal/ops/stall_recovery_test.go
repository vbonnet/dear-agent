package ops

import (
	"context"
	"os"
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
	// Note: Sent will be false because our mock returns false for delivery
	if !action.Sent {
		// This is expected since our mock doesn't implement delivery
	}
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

	action, err := recovery.recoverErrorLoopStall(context.Background(), event, "")
	if err != nil {
		t.Fatalf("recoverErrorLoopStall() error = %v", err)
	}
	if action.ActionType != "log_diagnostic" {
		t.Errorf("ActionType = %v, want log_diagnostic", action.ActionType)
	}
}

func TestRecoverErrorLoopStall_NoOrchestrator(t *testing.T) {
	recovery := NewStallRecovery(&OpContext{}, "")
	event := StallEvent{
		SessionName: "worker-2",
		StallType:   "error_loop",
		Evidence:    "Error: timeout appears 5 times",
	}

	action, err := recovery.recoverErrorLoopStall(context.Background(), event, "")
	if err != nil {
		t.Fatalf("recoverErrorLoopStall() error = %v", err)
	}
	if !action.Sent {
		t.Error("Expected action to be sent even without orchestrator")
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
