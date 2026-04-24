package compaction

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSessionState_SpecificFile(t *testing.T) {
	dir := t.TempDir()
	stateJSON := `{
		"orchestrator_session": "orchestrator",
		"managed_sessions": {
			"worker-1": {"status": "ACTIVE", "notes": "doing stuff"},
			"worker-2": {"status": "LAUNCHED", "notes": "starting"}
		},
		"completed_this_session_count": 5,
		"policy": {"no_impl": "NEVER do implementation work."},
		"queued": ["task-a", "task-b"],
		"scan_loop": {"interval": "2 minutes", "cron_id": "abc123"}
	}`
	os.WriteFile(filepath.Join(dir, "my-session-state.json"), []byte(stateJSON), 0o644)

	state, path, err := LoadSessionState(dir, "my-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(path, "my-session-state.json") {
		t.Errorf("path = %q, want suffix my-session-state.json", path)
	}
	if state.OrchestratorSession != "orchestrator" {
		t.Errorf("OrchestratorSession = %q, want orchestrator", state.OrchestratorSession)
	}
	if len(state.ManagedSessions) != 2 {
		t.Errorf("ManagedSessions count = %d, want 2", len(state.ManagedSessions))
	}
	if state.CompletedCount != 5 {
		t.Errorf("CompletedCount = %d, want 5", state.CompletedCount)
	}
	if len(state.Queued) != 2 {
		t.Errorf("Queued count = %d, want 2", len(state.Queued))
	}
	if state.ScanLoop == nil || state.ScanLoop.Interval != "2 minutes" {
		t.Error("ScanLoop not parsed correctly")
	}
}

func TestLoadSessionState_OrchestratorDirect(t *testing.T) {
	dir := t.TempDir()
	stateJSON := `{"orchestrator_session": "orchestrator", "completed_this_session_count": 10}`
	os.WriteFile(filepath.Join(dir, "orchestrator-state.json"), []byte(stateJSON), 0o644)

	// Session "orchestrator" should find orchestrator-state.json directly
	state, path, err := LoadSessionState(dir, "orchestrator")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(path, "orchestrator-state.json") {
		t.Errorf("path = %q, want suffix orchestrator-state.json", path)
	}
	if state.CompletedCount != 10 {
		t.Errorf("CompletedCount = %d, want 10", state.CompletedCount)
	}
}

func TestLoadSessionState_NoFallbackToOrchestrator(t *testing.T) {
	dir := t.TempDir()
	// Only orchestrator-state.json exists — other sessions must NOT fall back to it
	stateJSON := `{"orchestrator_session": "orchestrator", "completed_this_session_count": 10}`
	os.WriteFile(filepath.Join(dir, "orchestrator-state.json"), []byte(stateJSON), 0o644)

	_, _, err := LoadSessionState(dir, "some-worker")
	if err == nil {
		t.Fatal("expected error: should not fall back to orchestrator-state.json for non-orchestrator session")
	}
	if !strings.Contains(err.Error(), "no state file found") {
		t.Errorf("error = %q, want 'no state file found'", err.Error())
	}
}

func TestLoadSessionState_NoStateFile(t *testing.T) {
	dir := t.TempDir()
	_, _, err := LoadSessionState(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing state file")
	}
	if !strings.Contains(err.Error(), "no state file found") {
		t.Errorf("error = %q, want 'no state file found'", err.Error())
	}
}

func TestGeneratePreservePrompt_Full(t *testing.T) {
	state := &SessionState{
		OrchestratorSession: "orchestrator",
		ManagedSessions: map[string]ManagedSessionInfo{
			"worker-1": {Status: "ACTIVE", Notes: "doing stuff"},
			"worker-2": {Status: "LAUNCHED", Notes: "starting"},
		},
		CompletedCount: 49,
		Policy: map[string]string{
			"no_impl":     "NEVER do implementation work.",
			"anti_bypass": "NEVER accept --force/--skip.",
		},
		Queued: []string{"task-a", "task-b", "task-c"},
		ScanLoop: &ScanLoopInfo{
			Interval: "2 minutes",
			CronID:   "abc123",
		},
	}

	result := GeneratePreservePrompt(state, "~/.agm/orchestrator-state.json", "", "orchestrator")

	// Must start with /compact
	if !strings.HasPrefix(result, "/compact PRESERVE THROUGH COMPACTION:") {
		t.Errorf("should start with /compact PRESERVE THROUGH COMPACTION:, got prefix: %q", result[:60])
	}
	// Must contain identity from targetSessionName, not state file
	if !strings.Contains(result, "I am the orchestrator") {
		t.Error("should contain identity")
	}
	// Must contain state file path
	if !strings.Contains(result, "~/.agm/orchestrator-state.json") {
		t.Error("should contain state file path")
	}
	// Must contain managed sessions
	if !strings.Contains(result, "Managing 2 workers") {
		t.Error("should contain managed sessions count")
	}
	// Must contain completed count
	if !strings.Contains(result, "49 completed sessions") {
		t.Error("should contain completed count")
	}
	// Must contain queue size
	if !strings.Contains(result, "Queue has 3 items") {
		t.Error("should contain queue size")
	}
	// Must contain scan loop
	if !strings.Contains(result, "Resume scan loop via /loop 2 minutes") {
		t.Error("should contain scan loop instruction")
	}
	// Must contain policy rules
	if !strings.Contains(result, "NEVER do implementation work.") {
		t.Error("should contain policy rules")
	}
}

func TestGeneratePreservePrompt_WithFocus(t *testing.T) {
	state := &SessionState{
		OrchestratorSession: "orchestrator",
	}
	result := GeneratePreservePrompt(state, "/tmp/state.json", "preserve auth context", "orchestrator")
	if !strings.Contains(result, "Additional focus: preserve auth context.") {
		t.Error("should contain focus text")
	}
}

func TestGeneratePreservePrompt_MinimalState(t *testing.T) {
	state := &SessionState{}
	result := GeneratePreservePrompt(state, "/tmp/state.json", "", "")

	if !strings.HasPrefix(result, "/compact PRESERVE THROUGH COMPACTION:") {
		t.Error("should start with /compact PRESERVE")
	}
	// With empty targetSessionName, identity should be "worker"
	if !strings.Contains(result, "I am the worker") {
		t.Error("should default identity to worker")
	}
}

func TestGeneratePreservePrompt_UsesTargetIdentity(t *testing.T) {
	// State file has sender's identity ("orchestrator"), but we're compacting
	// the target session "meta-orchestrator". The prompt must use the target's
	// identity, not the sender's.
	state := &SessionState{
		OrchestratorSession: "orchestrator", // sender identity — must be ignored
		CompletedCount:      5,
	}
	result := GeneratePreservePrompt(state, "/tmp/state.json", "", "meta-orchestrator")

	if !strings.Contains(result, "I am the meta-orchestrator") {
		t.Errorf("should use target identity 'meta-orchestrator', got: %s", result)
	}
	if strings.Contains(result, "I am the orchestrator.") {
		t.Error("must NOT use sender identity from state file")
	}
}

func TestLoadSessionState_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad-state.json"), []byte("not json"), 0o644)
	_, _, err := LoadSessionState(dir, "bad")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parse state file") {
		t.Errorf("error = %q, want 'parse state file'", err.Error())
	}
}
