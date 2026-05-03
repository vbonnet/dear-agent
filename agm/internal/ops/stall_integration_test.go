package ops

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// --- Helper to build integration test context ---

// setupIntegrationTestRetryDir isolates retry state to a temporary directory
// for this test, preventing cross-test contamination from persisted state.
func setupIntegrationTestRetryDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGM_RETRY_BASE_DIR", tmpDir)
	t.Cleanup(func() {
		os.Unsetenv("AGM_RETRY_BASE_DIR")
	})
}

// integrationCtx creates an OpContext using dolt.MockAdapter (richer than mockStorage)
// and the shared mockTmux, suitable for detect→recover pipeline tests.
func integrationCtx(sessions []*manifest.Manifest, tmuxSessions ...string) *OpContext {
	mock := dolt.NewMockAdapter()
	for _, s := range sessions {
		_ = mock.CreateSession(s)
	}
	return &OpContext{
		Storage: mock,
		Tmux:    newMockTmux(tmuxSessions...),
	}
}

// --- 1. Full detect→recover pipeline ---

func TestIntegration_DetectAndRecover_PermissionPrompt(t *testing.T) {
	now := time.Now()
	sessions := []*manifest.Manifest{
		testManifest("worker-1", manifest.StatePermissionPrompt, now.Add(-10*time.Minute)),
		testManifest("orchestrator", manifest.StateReady, now),
	}

	ctx := integrationCtx(sessions, "worker-1", "orchestrator")
	detector := NewStallDetector(ctx)
	recovery := NewStallRecovery(ctx, "orchestrator")

	events, err := detector.DetectStalls(context.Background())
	if err != nil {
		t.Fatalf("DetectStalls() error = %v", err)
	}

	// Should find the permission prompt stall
	var permEvent *StallEvent
	for i := range events {
		if events[i].StallType == "permission_prompt" && events[i].SessionName == "worker-1" {
			permEvent = &events[i]
			break
		}
	}
	if permEvent == nil {
		t.Fatal("Expected permission_prompt stall event for worker-1")
	}
	if permEvent.Severity != "critical" {
		t.Errorf("Severity = %q, want %q", permEvent.Severity, "critical")
	}

	// Run recovery
	action, err := recovery.Recover(context.Background(), *permEvent)
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if action.ActionType != "alert_orchestrator" {
		t.Errorf("ActionType = %q, want %q", action.ActionType, "alert_orchestrator")
	}
	if action.SessionName != "worker-1" {
		t.Errorf("SessionName = %q, want %q", action.SessionName, "worker-1")
	}
}

func TestIntegration_DetectAndRecover_ErrorLoop_WithOrchestrator(t *testing.T) {
	now := time.Now()
	sessions := []*manifest.Manifest{
		testManifest("worker-err", manifest.StateWorking, now.Add(-5*time.Minute)),
		testManifest("orchestrator", manifest.StateReady, now),
	}

	ctx := integrationCtx(sessions, "worker-err", "orchestrator")
	recovery := NewStallRecovery(ctx, "orchestrator")

	// Simulate an error loop event (tmux capture is unavailable in tests,
	// so we construct the event directly as DetectStalls would)
	event := StallEvent{
		SessionName:       "worker-err",
		DetectedAt:        now,
		StallType:         "error_loop",
		Duration:          0,
		Evidence:          "Error pattern 'permission denied' appears 5 times in output",
		Severity:          "warning",
		RecommendedAction: "Send diagnostic to orchestrator or manually review error",
	}

	action, err := recovery.Recover(context.Background(), event)
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if action.ActionType != "log_diagnostic" {
		t.Errorf("ActionType = %q, want %q", action.ActionType, "log_diagnostic")
	}
	if action.Description == "" {
		t.Error("Expected non-empty Description")
	}
}

func TestIntegration_DetectAndRecover_NoCommit_Nudge(t *testing.T) {
	now := time.Now()
	sessions := []*manifest.Manifest{
		testManifest("worker-stale", manifest.StateWorking, now.Add(-20*time.Minute)),
	}

	ctx := integrationCtx(sessions, "worker-stale")
	recovery := NewStallRecovery(ctx, "")

	// Simulate no-commit event (git log unavailable in test env)
	event := StallEvent{
		SessionName:       "worker-stale",
		DetectedAt:        now,
		StallType:         "no_commit",
		Duration:          20 * time.Minute,
		Evidence:          "No commits in 20m while in WORKING state",
		Severity:          "warning",
		RecommendedAction: "Send nudge message to worker or check for blocking errors",
	}

	action, err := recovery.Recover(context.Background(), event)
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if action.ActionType != "nudge" {
		t.Errorf("ActionType = %q, want %q", action.ActionType, "nudge")
	}
}

// --- 2. Edge cases ---

func TestIntegration_SessionOfflineMidRecovery(t *testing.T) {
	setupIntegrationTestRetryDir(t)
	now := time.Now()

	// Session starts as PERMISSION_PROMPT (stalled)
	sessions := []*manifest.Manifest{
		testManifest("flaky-worker", manifest.StatePermissionPrompt, now.Add(-10*time.Minute)),
	}

	ctx := integrationCtx(sessions)
	detector := NewStallDetector(ctx)

	events, err := detector.DetectStalls(context.Background())
	if err != nil {
		t.Fatalf("DetectStalls() error = %v", err)
	}

	// Should detect the stall
	if len(events) == 0 {
		t.Fatal("Expected at least one stall event")
	}

	// Now simulate the session going offline by updating state
	// Recovery with no orchestrator should fail gracefully
	recovery := NewStallRecovery(ctx, "")
	action, err := recovery.Recover(context.Background(), events[0])

	// Permission prompt recovery requires orchestrator, so should error
	if err == nil {
		t.Error("Expected error when no orchestrator configured for permission_prompt recovery")
	}
	if action.ActionType != "alert_orchestrator" {
		t.Errorf("ActionType = %q, want %q", action.ActionType, "alert_orchestrator")
	}
}

func TestIntegration_MultipleSimultaneousStalls(t *testing.T) {
	now := time.Now()
	sessions := []*manifest.Manifest{
		testManifest("worker-perm-1", manifest.StatePermissionPrompt, now.Add(-6*time.Minute)),
		testManifest("worker-perm-2", manifest.StatePermissionPrompt, now.Add(-15*time.Minute)),
		testManifest("worker-idle", manifest.StateWorking, now.Add(-25*time.Minute)),
		testManifest("healthy-session", manifest.StateWorking, now.Add(-2*time.Minute)),
		testManifest("done-session", manifest.StateDone, now.Add(-60*time.Minute)),
	}

	ctx := integrationCtx(sessions, "worker-perm-1", "worker-perm-2", "worker-idle", "healthy-session")
	detector := NewStallDetector(ctx)

	events, err := detector.DetectStalls(context.Background())
	if err != nil {
		t.Fatalf("DetectStalls() error = %v", err)
	}

	// Count stall types
	permStalls := 0
	for _, e := range events {
		if e.StallType == "permission_prompt" {
			permStalls++
		}
	}

	// Both worker-perm-1 (6min > 5min) and worker-perm-2 (15min > 5min) should stall
	if permStalls != 2 {
		t.Errorf("Expected 2 permission_prompt stalls, got %d", permStalls)
	}

	// healthy-session (2min) should not appear
	for _, e := range events {
		if e.SessionName == "healthy-session" {
			t.Error("healthy-session should not have any stall events")
		}
	}

	// done-session should not appear (skipped for error loop, not permission_prompt state)
	for _, e := range events {
		if e.SessionName == "done-session" {
			t.Error("done-session should not have any stall events")
		}
	}
}

func TestIntegration_ArchivedSessionsExcluded(t *testing.T) {
	now := time.Now()
	archived := testManifest("archived-worker", manifest.StatePermissionPrompt, now.Add(-30*time.Minute))
	archived.Lifecycle = manifest.LifecycleArchived

	sessions := []*manifest.Manifest{archived}

	ctx := integrationCtx(sessions)
	detector := NewStallDetector(ctx)

	events, err := detector.DetectStalls(context.Background())
	if err != nil {
		t.Fatalf("DetectStalls() error = %v", err)
	}

	if len(events) != 0 {
		t.Errorf("Expected 0 stall events for archived session, got %d", len(events))
	}
}

func TestIntegration_RecoverAllStallTypes(t *testing.T) {
	now := time.Now()
	sessions := []*manifest.Manifest{
		testManifest("worker-target", manifest.StateWorking, now),
		testManifest("orchestrator", manifest.StateReady, now),
	}

	ctx := integrationCtx(sessions, "worker-target", "orchestrator")
	recovery := NewStallRecovery(ctx, "orchestrator")

	tests := []struct {
		name       string
		event      StallEvent
		wantAction string
	}{
		{
			name: "permission_prompt",
			event: StallEvent{
				SessionName: "worker-target",
				StallType:   "permission_prompt",
				Duration:    10 * time.Minute,
				Evidence:    "Permission dialog open for 10m",
			},
			wantAction: "alert_orchestrator",
		},
		{
			name: "no_commit",
			event: StallEvent{
				SessionName: "worker-target",
				StallType:   "no_commit",
				Duration:    20 * time.Minute,
				Evidence:    "No commits in 20m",
			},
			wantAction: "nudge",
		},
		{
			name: "error_loop",
			event: StallEvent{
				SessionName: "worker-target",
				StallType:   "error_loop",
				Evidence:    "Error pattern 'timeout' appears 4 times",
			},
			wantAction: "log_diagnostic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action, err := recovery.Recover(context.Background(), tt.event)
			if err != nil {
				t.Fatalf("Recover(%s) error = %v", tt.name, err)
			}
			if action.ActionType != tt.wantAction {
				t.Errorf("ActionType = %q, want %q", action.ActionType, tt.wantAction)
			}
		})
	}
}

// --- 3. Threshold tuning ---

func TestIntegration_PermissionThreshold_ExactBoundary(t *testing.T) {
	now := time.Now()
	detector := NewStallDetector(&OpContext{})

	tests := []struct {
		name     string
		duration time.Duration
		wantNil  bool
	}{
		{"4m59s - below threshold", 4*time.Minute + 59*time.Second, true},
		{"5m - at threshold", 5 * time.Minute, false},
		{"5m1s - above threshold", 5*time.Minute + 1*time.Second, false},
		{"10m - well above", 10 * time.Minute, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := testManifest("test", manifest.StatePermissionPrompt, now.Add(-tt.duration))
			event := detector.detectPermissionPromptStall(session, now)
			if tt.wantNil && event != nil {
				t.Errorf("Expected nil for duration %v, got event", tt.duration)
			}
			if !tt.wantNil && event == nil {
				t.Errorf("Expected event for duration %v, got nil", tt.duration)
			}
		})
	}
}

func TestIntegration_NoCommitThreshold_ExactBoundary(t *testing.T) {
	now := time.Now()
	detector := NewStallDetector(&OpContext{})

	tests := []struct {
		name     string
		duration time.Duration
		wantNil  bool
	}{
		{"14m59s - below threshold", 14*time.Minute + 59*time.Second, true},
		{"15m - at threshold", 15 * time.Minute, false},
		{"15m1s - above threshold", 15*time.Minute + 1*time.Second, false},
		{"30m - well above", 30 * time.Minute, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := testManifest("worker-test", manifest.StateWorking, now.Add(-tt.duration))
			event := detector.detectNoCommitStall(session, now)
			// Note: countRecentCommits will return -1 in test (no git repo),
			// so the no-commit stall is skipped. We verify the duration gate
			// by checking that short durations are always nil.
			if tt.wantNil && event != nil {
				t.Errorf("Expected nil for duration %v, got event", tt.duration)
			}
			// For durations above threshold, event may be nil due to git returning -1,
			// so we only assert the below-threshold case deterministically.
		})
	}
}

func TestIntegration_ErrorLoopThreshold_Boundary(t *testing.T) {
	tests := []struct {
		name       string
		threshold  int
		errorCount int
		wantStall  bool
	}{
		{"2 errors with threshold 3", 3, 2, false},
		{"3 errors with threshold 3", 3, 3, true},
		{"4 errors with threshold 3", 3, 4, true},
		{"1 error with threshold 1", 1, 1, true},
		{"5 errors with threshold 5", 5, 5, true},
		{"4 errors with threshold 5", 5, 4, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build error output
			output := ""
			for i := 0; i < tt.errorCount; i++ {
				output += "error: permission denied\n"
			}

			patterns := extractErrorPatterns(output)
			hasStall := false
			for _, count := range patterns {
				if count >= tt.threshold {
					hasStall = true
					break
				}
			}

			if hasStall != tt.wantStall {
				t.Errorf("errorCount=%d threshold=%d: got stall=%v, want %v",
					tt.errorCount, tt.threshold, hasStall, tt.wantStall)
			}
		})
	}
}

func TestIntegration_CustomThresholds(t *testing.T) {
	now := time.Now()
	sessions := []*manifest.Manifest{
		testManifest("session-a", manifest.StatePermissionPrompt, now.Add(-2*time.Minute)),
	}

	ctx := integrationCtx(sessions)
	detector := NewStallDetector(ctx)

	// Default 5min threshold — 2min should not trigger
	events, err := detector.DetectStalls(context.Background())
	if err != nil {
		t.Fatalf("DetectStalls() error = %v", err)
	}
	for _, e := range events {
		if e.StallType == "permission_prompt" {
			t.Error("2min should not trigger default 5min permission threshold")
		}
	}

	// Lower threshold to 1min — 2min should now trigger
	detector.PermissionTimeout = 1 * time.Minute
	events, err = detector.DetectStalls(context.Background())
	if err != nil {
		t.Fatalf("DetectStalls() error = %v", err)
	}
	foundPerm := false
	for _, e := range events {
		if e.StallType == "permission_prompt" {
			foundPerm = true
		}
	}
	if !foundPerm {
		t.Error("2min should trigger when permission threshold is 1min")
	}
}

// --- 4. JSON event output format ---

func TestIntegration_StallEventFields(t *testing.T) {
	now := time.Now()
	sessions := []*manifest.Manifest{
		testManifest("json-test", manifest.StatePermissionPrompt, now.Add(-10*time.Minute)),
	}

	ctx := integrationCtx(sessions)
	detector := NewStallDetector(ctx)

	events, err := detector.DetectStalls(context.Background())
	if err != nil {
		t.Fatalf("DetectStalls() error = %v", err)
	}

	var event *StallEvent
	for i := range events {
		if events[i].StallType == "permission_prompt" {
			event = &events[i]
			break
		}
	}
	if event == nil {
		t.Fatal("Expected permission_prompt event")
	}

	// Validate all fields are populated
	if event.SessionName != "json-test" {
		t.Errorf("SessionName = %q, want %q", event.SessionName, "json-test")
	}
	if event.DetectedAt.IsZero() {
		t.Error("DetectedAt should not be zero")
	}
	if event.StallType != "permission_prompt" {
		t.Errorf("StallType = %q, want %q", event.StallType, "permission_prompt")
	}
	if event.Duration < 10*time.Minute {
		t.Errorf("Duration = %v, want >= 10m", event.Duration)
	}
	if event.Evidence == "" {
		t.Error("Evidence should not be empty")
	}
	if event.Severity != "critical" {
		t.Errorf("Severity = %q, want %q", event.Severity, "critical")
	}
	if event.RecommendedAction == "" {
		t.Error("RecommendedAction should not be empty")
	}
}

func TestIntegration_RecoveryActionFields(t *testing.T) {
	now := time.Now()
	sessions := []*manifest.Manifest{
		testManifest("action-test", manifest.StateWorking, now),
		testManifest("orchestrator", manifest.StateReady, now),
	}

	ctx := integrationCtx(sessions, "action-test", "orchestrator")
	recovery := NewStallRecovery(ctx, "orchestrator")

	event := StallEvent{
		SessionName: "action-test",
		StallType:   "error_loop",
		Evidence:    "Error pattern 'timeout' appears 3 times",
	}

	action, err := recovery.Recover(context.Background(), event)
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}

	// Validate all RecoveryAction fields
	if action.SessionName != "action-test" {
		t.Errorf("SessionName = %q, want %q", action.SessionName, "action-test")
	}
	if action.ActionType != "log_diagnostic" {
		t.Errorf("ActionType = %q, want %q", action.ActionType, "log_diagnostic")
	}
	if action.Description == "" {
		t.Error("Description should not be empty")
	}
	// Error should be empty on success
	if action.Error != "" {
		t.Errorf("Error = %q, want empty on success", action.Error)
	}
}

// --- 5. Dry-run mode ---

func TestIntegration_DryRunDetectsWithoutRecovery(t *testing.T) {
	now := time.Now()
	sessions := []*manifest.Manifest{
		testManifest("dry-worker", manifest.StatePermissionPrompt, now.Add(-10*time.Minute)),
		testManifest("orchestrator", manifest.StateReady, now),
	}

	ctx := integrationCtx(sessions, "dry-worker", "orchestrator")
	ctx.DryRun = true

	detector := NewStallDetector(ctx)
	recovery := NewStallRecovery(ctx, "orchestrator")

	// Detection works the same in dry-run
	events, err := detector.DetectStalls(context.Background())
	if err != nil {
		t.Fatalf("DetectStalls() error = %v", err)
	}

	if len(events) == 0 {
		t.Fatal("Expected stall events even in dry-run mode")
	}

	// Verify dry-run flag is accessible for CLI to gate recovery
	if !ctx.DryRun {
		t.Error("DryRun should be true")
	}

	// Recovery should still work (CLI is responsible for gating, not the ops layer)
	// This verifies the pipeline is intact — CLI checks ctx.DryRun before calling Recover
	action, err := recovery.Recover(context.Background(), events[0])
	if err != nil {
		t.Fatalf("Recover() should succeed even with DryRun context (CLI gates): %v", err)
	}
	if action.ActionType == "" {
		t.Error("ActionType should be set even in dry-run context")
	}
}

// --- Additional edge cases ---

func TestIntegration_RecoverUnknownStallType(t *testing.T) {
	ctx := integrationCtx(nil)
	recovery := NewStallRecovery(ctx, "")

	event := StallEvent{
		SessionName: "test",
		StallType:   "cosmic_ray",
	}

	_, err := recovery.Recover(context.Background(), event)
	if err == nil {
		t.Error("Expected error for unknown stall type")
	}
}

func TestIntegration_EmptySessionPool(t *testing.T) {
	ctx := integrationCtx(nil)
	detector := NewStallDetector(ctx)

	events, err := detector.DetectStalls(context.Background())
	if err != nil {
		t.Fatalf("DetectStalls() error = %v", err)
	}
	if len(events) != 0 {
		t.Errorf("Expected 0 events, got %d", len(events))
	}
}

func TestIntegration_OnlyWorkerSessionsGetNoCommitCheck(t *testing.T) {
	now := time.Now()
	sessions := []*manifest.Manifest{
		testManifest("orchestrator-main", manifest.StateWorking, now.Add(-30*time.Minute)),
		testManifest("my-worker-1", manifest.StateWorking, now.Add(-30*time.Minute)),
	}

	ctx := integrationCtx(sessions)
	detector := NewStallDetector(ctx)

	events, err := detector.DetectStalls(context.Background())
	if err != nil {
		t.Fatalf("DetectStalls() error = %v", err)
	}

	// orchestrator-main should NOT get a no_commit stall (not a worker)
	for _, e := range events {
		if e.SessionName == "orchestrator-main" && e.StallType == "no_commit" {
			t.Error("orchestrator-main should not get no_commit stall check")
		}
	}
}

func TestIntegration_WorkerByTag(t *testing.T) {
	now := time.Now()
	session := testManifest("generic-session", manifest.StateWorking, now.Add(-20*time.Minute))
	session.Context.Tags = []string{"worker", "team-backend"}

	if !isWorkerSession(session) {
		t.Error("Session with 'worker' tag should be identified as worker")
	}
}

func TestIntegration_SkippedStates(t *testing.T) {
	now := time.Now()

	// These states should be skipped by error loop detection
	skippedStates := []string{
		manifest.StateOffline,
		manifest.StateReady,
		manifest.StateDone,
	}

	detector := NewStallDetector(&OpContext{})

	for _, state := range skippedStates {
		t.Run(state, func(t *testing.T) {
			session := testManifest("test-"+state, state, now.Add(-10*time.Minute))
			event := detector.detectErrorLoopStall(session)
			if event != nil {
				t.Errorf("State %q should be skipped for error loop detection", state)
			}
		})
	}
}

func TestIntegration_ZeroStateUpdatedAt(t *testing.T) {
	detector := NewStallDetector(&OpContext{})
	now := time.Now()

	// Zero StateUpdatedAt should not trigger permission stall
	session := testManifest("test", manifest.StatePermissionPrompt, time.Time{})
	event := detector.detectPermissionPromptStall(session, now)
	if event != nil {
		t.Error("Zero StateUpdatedAt should not trigger permission stall")
	}

	// Zero StateUpdatedAt should not trigger no-commit stall
	session2 := testManifest("worker-test", manifest.StateWorking, time.Time{})
	event2 := detector.detectNoCommitStall(session2, now)
	if event2 != nil {
		t.Error("Zero StateUpdatedAt should not trigger no-commit stall")
	}
}

func TestIntegration_MixedErrorPatterns(t *testing.T) {
	output := `error: permission denied
error: timeout waiting for response
error: permission denied
error: timeout waiting for response
error: permission denied
info: processing completed`

	patterns := extractErrorPatterns(output)

	// Should have at least 2 distinct error patterns
	if len(patterns) < 2 {
		t.Errorf("Expected at least 2 error patterns, got %d", len(patterns))
	}

	// "permission denied" normalized should appear 3 times
	found := false
	for pattern, count := range patterns {
		if count >= 3 {
			found = true
			_ = pattern
		}
	}
	if !found {
		t.Error("Expected at least one pattern with count >= 3")
	}
}
