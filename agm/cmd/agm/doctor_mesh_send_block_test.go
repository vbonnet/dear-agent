package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

// ---------------------------------------------------------------------------
// parseSendBlockGuard
// ---------------------------------------------------------------------------

func TestParseSendBlockGuard_HumanTyping(t *testing.T) {
	// Mirrors a historical pre-operation safety-guard audit entry.
	errMsg := "safety guard blocked send on session 'worker-1':\n\n  human_typing: Unsent text detected in prompt: \"hello\"\n  -> Wait for the human to finish typing before sending.\n\n"
	got := parseSendBlockGuard(errMsg)
	if got != "human_typing" {
		t.Errorf("expected 'human_typing', got %q", got)
	}
}

func TestParseSendBlockGuard_SessionUninitialized(t *testing.T) {
	errMsg := "safety guard blocked send on session 's2':\n\n  session_uninitialized: Claude process is not running.\n  -> Wait for Claude to start.\n\n"
	got := parseSendBlockGuard(errMsg)
	if got != "session_uninitialized" {
		t.Errorf("expected 'session_uninitialized', got %q", got)
	}
}

func TestParseSendBlockGuard_ClaudeMidResponse(t *testing.T) {
	errMsg := "safety guard blocked send on session 'sess':\n\n  claude_mid_response: Spinner detected.\n  -> Wait for response.\n\n"
	got := parseSendBlockGuard(errMsg)
	if got != "claude_mid_response" {
		t.Errorf("expected 'claude_mid_response', got %q", got)
	}
}

func TestParseSendBlockGuard_NotAGuardError(t *testing.T) {
	errMsg := "session 'foo' does not exist in tmux"
	got := parseSendBlockGuard(errMsg)
	if got != "" {
		t.Errorf("expected empty string for non-guard error, got %q", got)
	}
}

func TestParseSendBlockGuard_EmptyString(t *testing.T) {
	if parseSendBlockGuard("") != "" {
		t.Error("expected empty string for empty input")
	}
}

func TestParseSendBlockGuard_GuardBlockedButNoViolationLine(t *testing.T) {
	// Malformed error: "safety guard blocked" present but no violation line.
	errMsg := "safety guard blocked send on session 'x':\n\n"
	got := parseSendBlockGuard(errMsg)
	if got != "" {
		t.Errorf("expected empty string when no violation line, got %q", got)
	}
}

func TestParseSendBlockGuard_SharedOperationReadiness(t *testing.T) {
	errMsg := "shared CLI send: [AGM-016] Session is not ready for input: Session \"worker-1\" cannot safely receive input (readiness: WRONG_HARNESS). No input was sent."
	if got := parseSendBlockGuard(errMsg); got != "readiness_wrong_harness" {
		t.Fatalf("parseSendBlockGuard() = %q, want readiness_wrong_harness", got)
	}
}

func TestParseSendBlockGuard_IgnoresOtherOperationErrors(t *testing.T) {
	errMsg := "[AGM-001] Session not found: No session matches identifier \"missing\"."
	if got := parseSendBlockGuard(errMsg); got != "" {
		t.Fatalf("parseSendBlockGuard() = %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// analyzeMeshWideSendBlock
// ---------------------------------------------------------------------------

// writeAuditLog writes a slice of AuditEvents to a temporary JSONL file.
func writeAuditLog(t *testing.T, events []ops.AuditEvent) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create audit log: %v", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, ev := range events {
		if err := enc.Encode(ev); err != nil {
			t.Fatalf("encode event: %v", err)
		}
	}
	return logPath
}

func blockErr(session string) string {
	return "safety guard blocked send on session '" + session + "':\n\n  human_typing: Unsent text detected.\n  -> Wait.\n\n"
}

func TestAnalyzeMeshWideSendBlock_NoEvents(t *testing.T) {
	logPath := writeAuditLog(t, nil)
	result, err := analyzeMeshWideSendBlock(logPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.GuardToSessions) != 0 {
		t.Errorf("expected no guard entries, got %v", result.GuardToSessions)
	}
}

func TestAnalyzeMeshWideSendBlock_SingleSessionNotIncident(t *testing.T) {
	now := time.Now()
	events := []ops.AuditEvent{
		{Timestamp: now, Command: "send.msg", Session: "worker-1", Result: "error", Error: blockErr("worker-1")},
	}
	logPath := writeAuditLog(t, events)
	result, err := analyzeMeshWideSendBlock(logPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sessions := result.GuardToSessions["human_typing"]
	if len(sessions) != 1 {
		t.Errorf("expected 1 session entry, got %d", len(sessions))
	}
}

func TestAnalyzeMeshWideSendBlock_TwoSessionsIsIncident(t *testing.T) {
	now := time.Now()
	events := []ops.AuditEvent{
		{Timestamp: now, Command: "send.msg", Session: "worker-1", Result: "error", Error: blockErr("worker-1")},
		{Timestamp: now, Command: "send.msg", Session: "worker-2", Result: "error", Error: blockErr("worker-2")},
	}
	logPath := writeAuditLog(t, events)
	result, err := analyzeMeshWideSendBlock(logPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sessions := result.GuardToSessions["human_typing"]
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d: %v", len(sessions), sessions)
	}
}

func TestAnalyzeMeshWideSendBlock_DeduplicatesSameSession(t *testing.T) {
	// Multiple failures from the same session count as one.
	now := time.Now()
	events := []ops.AuditEvent{
		{Timestamp: now, Command: "send.msg", Session: "worker-1", Result: "error", Error: blockErr("worker-1")},
		{Timestamp: now.Add(time.Minute), Command: "send.msg", Session: "worker-1", Result: "error", Error: blockErr("worker-1")},
		{Timestamp: now, Command: "send.msg", Session: "worker-2", Result: "error", Error: blockErr("worker-2")},
	}
	logPath := writeAuditLog(t, events)
	result, err := analyzeMeshWideSendBlock(logPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sessions := result.GuardToSessions["human_typing"]
	if len(sessions) != 2 {
		t.Errorf("expected 2 distinct sessions after dedup, got %d: %v", len(sessions), sessions)
	}
}

func TestAnalyzeMeshWideSendBlock_IgnoresOldEvents(t *testing.T) {
	old := time.Now().Add(-2 * time.Hour)
	events := []ops.AuditEvent{
		{Timestamp: old, Command: "send.msg", Session: "worker-1", Result: "error", Error: blockErr("worker-1")},
		{Timestamp: old, Command: "send.msg", Session: "worker-2", Result: "error", Error: blockErr("worker-2")},
	}
	logPath := writeAuditLog(t, events)
	result, err := analyzeMeshWideSendBlock(logPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.GuardToSessions) != 0 {
		t.Errorf("expected no entries for old events, got %v", result.GuardToSessions)
	}
}

func TestAnalyzeMeshWideSendBlock_IgnoresNonSendMsg(t *testing.T) {
	now := time.Now()
	events := []ops.AuditEvent{
		{Timestamp: now, Command: "session.kill", Session: "worker-1", Result: "error", Error: blockErr("worker-1")},
		{Timestamp: now, Command: "send.enter", Session: "worker-2", Result: "error", Error: blockErr("worker-2")},
	}
	logPath := writeAuditLog(t, events)
	result, err := analyzeMeshWideSendBlock(logPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.GuardToSessions) != 0 {
		t.Errorf("expected no entries for non-send.msg commands, got %v", result.GuardToSessions)
	}
}

func TestAnalyzeMeshWideSendBlock_IgnoresSuccessfulSends(t *testing.T) {
	now := time.Now()
	events := []ops.AuditEvent{
		{Timestamp: now, Command: "send.msg", Session: "worker-1", Result: "success"},
		{Timestamp: now, Command: "send.msg", Session: "worker-2", Result: "success"},
	}
	logPath := writeAuditLog(t, events)
	result, err := analyzeMeshWideSendBlock(logPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.GuardToSessions) != 0 {
		t.Errorf("expected no entries for successful sends, got %v", result.GuardToSessions)
	}
}

func TestAnalyzeMeshWideSendBlock_IgnoresNonGuardErrors(t *testing.T) {
	now := time.Now()
	events := []ops.AuditEvent{
		{Timestamp: now, Command: "send.msg", Session: "worker-1", Result: "error", Error: "session 'worker-1' does not exist in tmux"},
		{Timestamp: now, Command: "send.msg", Session: "worker-2", Result: "error", Error: "rate limit exceeded"},
	}
	logPath := writeAuditLog(t, events)
	result, err := analyzeMeshWideSendBlock(logPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.GuardToSessions) != 0 {
		t.Errorf("expected no entries for non-guard errors, got %v", result.GuardToSessions)
	}
}

func TestAnalyzeMeshWideSendBlock_GroupsByGuard(t *testing.T) {
	now := time.Now()
	humanTypingErr := "safety guard blocked send on session 'w1':\n\n  human_typing: Unsent text.\n  -> Wait.\n\n"
	uninitErr := "safety guard blocked send on session 'w2':\n\n  session_uninitialized: Claude not running.\n  -> Wait.\n\n"
	events := []ops.AuditEvent{
		{Timestamp: now, Command: "send.msg", Session: "w1", Result: "error", Error: humanTypingErr},
		{Timestamp: now, Command: "send.msg", Session: "w2", Result: "error", Error: uninitErr},
		{Timestamp: now, Command: "send.msg", Session: "w3", Result: "error", Error: humanTypingErr},
	}
	logPath := writeAuditLog(t, events)
	result, err := analyzeMeshWideSendBlock(logPath, 30*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.GuardToSessions["human_typing"]) != 2 {
		t.Errorf("expected 2 human_typing sessions, got %d", len(result.GuardToSessions["human_typing"]))
	}
	if len(result.GuardToSessions["session_uninitialized"]) != 1 {
		t.Errorf("expected 1 session_uninitialized session, got %d", len(result.GuardToSessions["session_uninitialized"]))
	}
}

func TestAnalyzeMeshWideSendBlock_MissingLogFile(t *testing.T) {
	result, err := analyzeMeshWideSendBlock("/nonexistent/path/audit.jsonl", 30*time.Minute)
	if err != nil {
		t.Fatalf("expected nil error for missing log file, got %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for missing log file")
	}
	if len(result.GuardToSessions) != 0 {
		t.Errorf("expected empty result for missing log file, got %v", result.GuardToSessions)
	}
}

// ---------------------------------------------------------------------------
// checkMeshWideSendBlock (integration-style, uses temp HOME)
// ---------------------------------------------------------------------------

func TestCheckMeshWideSendBlock_HealthyWhenNoBlock(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// No audit log at all — should return healthy.
	healthy := checkMeshWideSendBlock()
	if !healthy {
		t.Error("expected healthy when no audit log exists")
	}
}

func TestCheckMeshWideSendBlock_DetectsIncident(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	logDir := filepath.Join(tmpHome, ".agm", "logs")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	now := time.Now()
	events := []ops.AuditEvent{
		{Timestamp: now, Command: "send.msg", Session: "alpha", Result: "error", Error: blockErr("alpha")},
		{Timestamp: now, Command: "send.msg", Session: "beta", Result: "error", Error: blockErr("beta")},
	}
	logPath := filepath.Join(logDir, "audit.jsonl")
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create log: %v", err)
	}
	enc := json.NewEncoder(f)
	for _, ev := range events {
		_ = enc.Encode(ev)
	}
	f.Close()

	healthy := checkMeshWideSendBlock()
	if healthy {
		t.Error("expected unhealthy when 2+ sessions share a guard block")
	}
}
