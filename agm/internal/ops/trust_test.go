package ops

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTrustDir(t *testing.T) (cleanup func()) {
	t.Helper()
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	return func() {
		t.Setenv("HOME", origHome)
	}
}

func TestTrustRecord(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()

	result, err := TrustRecord(nil, &TrustRecordRequest{
		SessionName: "test-session",
		EventType:   "success",
		Detail:      "task completed correctly",
	})
	if err != nil {
		t.Fatalf("TrustRecord: %v", err)
	}

	if result.Event.EventType != TrustEventSuccess {
		t.Errorf("EventType = %q, want %q", result.Event.EventType, TrustEventSuccess)
	}
	if result.Event.SessionName != "test-session" {
		t.Errorf("SessionName = %q, want %q", result.Event.SessionName, "test-session")
	}
	if result.Event.Detail != "task completed correctly" {
		t.Errorf("Detail = %q, want %q", result.Event.Detail, "task completed correctly")
	}

	// Verify file was written
	path := trustFilePath("test-session")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("trust file not created at %s", path)
	}
}

func TestTrustRecord_InvalidEventType(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()

	_, err := TrustRecord(nil, &TrustRecordRequest{
		SessionName: "test-session",
		EventType:   "bogus",
	})
	if err == nil {
		t.Fatal("expected error for invalid event type, got nil")
	}
}

func TestTrustRecord_EmptySessionName(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()

	_, err := TrustRecord(nil, &TrustRecordRequest{
		SessionName: "",
		EventType:   "success",
	})
	if err == nil {
		t.Fatal("expected error for empty session name, got nil")
	}
}

func TestTrustScore_NoEvents(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()

	result, err := TrustScore(nil, &TrustScoreRequest{SessionName: "empty-session"})
	if err != nil {
		t.Fatalf("TrustScore: %v", err)
	}

	if result.Score != 50 {
		t.Errorf("Score = %d, want 50 (base score)", result.Score)
	}
	if result.TotalEvents != 0 {
		t.Errorf("TotalEvents = %d, want 0", result.TotalEvents)
	}
}

func TestTrustScore_MixedEvents(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()

	// Record events: 3 successes (+15), 1 false_completion (-15), 1 stall (-5)
	// Expected: 50 + 15 - 15 - 5 = 45
	events := []TrustRecordRequest{
		{SessionName: "mixed", EventType: "success", Detail: "good"},
		{SessionName: "mixed", EventType: "success", Detail: "good"},
		{SessionName: "mixed", EventType: "success", Detail: "good"},
		{SessionName: "mixed", EventType: "false_completion", Detail: "bad"},
		{SessionName: "mixed", EventType: "stall", Detail: "stuck"},
	}
	for _, req := range events {
		if _, err := TrustRecord(nil, &req); err != nil {
			t.Fatalf("TrustRecord: %v", err)
		}
	}

	result, err := TrustScore(nil, &TrustScoreRequest{SessionName: "mixed"})
	if err != nil {
		t.Fatalf("TrustScore: %v", err)
	}

	if result.Score != 45 {
		t.Errorf("Score = %d, want 45", result.Score)
	}
	if result.TotalEvents != 5 {
		t.Errorf("TotalEvents = %d, want 5", result.TotalEvents)
	}
	if len(result.Breakdown) != 3 {
		t.Errorf("Breakdown length = %d, want 3", len(result.Breakdown))
	}
}

func TestTrustScore_ClampMin(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()

	// 5 false_completions: 50 + (5 * -15) = -25, clamped to 0
	for i := 0; i < 5; i++ {
		if _, err := TrustRecord(nil, &TrustRecordRequest{
			SessionName: "bad-agent",
			EventType:   "false_completion",
		}); err != nil {
			t.Fatalf("TrustRecord: %v", err)
		}
	}

	result, err := TrustScore(nil, &TrustScoreRequest{SessionName: "bad-agent"})
	if err != nil {
		t.Fatalf("TrustScore: %v", err)
	}

	if result.Score != 0 {
		t.Errorf("Score = %d, want 0 (clamped min)", result.Score)
	}
}

func TestTrustScore_ClampMax(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()

	// 20 successes: 50 + (20 * 5) = 150, clamped to 100
	for i := 0; i < 20; i++ {
		if _, err := TrustRecord(nil, &TrustRecordRequest{
			SessionName: "star-agent",
			EventType:   "success",
		}); err != nil {
			t.Fatalf("TrustRecord: %v", err)
		}
	}

	result, err := TrustScore(nil, &TrustScoreRequest{SessionName: "star-agent"})
	if err != nil {
		t.Fatalf("TrustScore: %v", err)
	}

	if result.Score != 100 {
		t.Errorf("Score = %d, want 100 (clamped max)", result.Score)
	}
}

func TestTrustHistory(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()

	// Record 3 events
	for _, et := range []string{"success", "stall", "error_loop"} {
		if _, err := TrustRecord(nil, &TrustRecordRequest{
			SessionName: "hist-session",
			EventType:   et,
		}); err != nil {
			t.Fatalf("TrustRecord: %v", err)
		}
	}

	result, err := TrustHistory(nil, &TrustHistoryRequest{SessionName: "hist-session"})
	if err != nil {
		t.Fatalf("TrustHistory: %v", err)
	}

	if result.Total != 3 {
		t.Errorf("Total = %d, want 3", result.Total)
	}
	if len(result.Events) != 3 {
		t.Fatalf("Events length = %d, want 3", len(result.Events))
	}
	if result.Events[0].EventType != TrustEventSuccess {
		t.Errorf("Events[0].EventType = %q, want %q", result.Events[0].EventType, TrustEventSuccess)
	}
	if result.Events[1].EventType != TrustEventStall {
		t.Errorf("Events[1].EventType = %q, want %q", result.Events[1].EventType, TrustEventStall)
	}
	if result.Events[2].EventType != TrustEventErrorLoop {
		t.Errorf("Events[2].EventType = %q, want %q", result.Events[2].EventType, TrustEventErrorLoop)
	}
}

func TestTrustHistory_NoFile(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()

	result, err := TrustHistory(nil, &TrustHistoryRequest{SessionName: "nonexistent"})
	if err != nil {
		t.Fatalf("TrustHistory: %v", err)
	}

	if result.Total != 0 {
		t.Errorf("Total = %d, want 0", result.Total)
	}
}

func TestTrustLeaderboard(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()

	// Create 3 sessions with different scores
	// session-a: 2 successes = 50 + 10 = 60
	// session-b: 1 false_completion = 50 - 15 = 35
	// session-c: 3 successes = 50 + 15 = 65

	for i := 0; i < 2; i++ {
		if _, err := TrustRecord(nil, &TrustRecordRequest{
			SessionName: "session-a", EventType: "success",
		}); err != nil {
			t.Fatalf("TrustRecord: %v", err)
		}
	}

	if _, err := TrustRecord(nil, &TrustRecordRequest{
		SessionName: "session-b", EventType: "false_completion",
	}); err != nil {
		t.Fatalf("TrustRecord: %v", err)
	}

	for i := 0; i < 3; i++ {
		if _, err := TrustRecord(nil, &TrustRecordRequest{
			SessionName: "session-c", EventType: "success",
		}); err != nil {
			t.Fatalf("TrustRecord: %v", err)
		}
	}

	result, err := TrustLeaderboard(nil)
	if err != nil {
		t.Fatalf("TrustLeaderboard: %v", err)
	}

	if len(result.Entries) != 3 {
		t.Fatalf("Entries length = %d, want 3", len(result.Entries))
	}

	// Should be sorted by score descending: session-c(65), session-a(60), session-b(35)
	if result.Entries[0].SessionName != "session-c" {
		t.Errorf("Entries[0].SessionName = %q, want %q", result.Entries[0].SessionName, "session-c")
	}
	if result.Entries[0].Score != 65 {
		t.Errorf("Entries[0].Score = %d, want 65", result.Entries[0].Score)
	}
	if result.Entries[1].SessionName != "session-a" {
		t.Errorf("Entries[1].SessionName = %q, want %q", result.Entries[1].SessionName, "session-a")
	}
	if result.Entries[1].Score != 60 {
		t.Errorf("Entries[1].Score = %d, want 60", result.Entries[1].Score)
	}
	if result.Entries[2].SessionName != "session-b" {
		t.Errorf("Entries[2].SessionName = %q, want %q", result.Entries[2].SessionName, "session-b")
	}
	if result.Entries[2].Score != 35 {
		t.Errorf("Entries[2].Score = %d, want 35", result.Entries[2].Score)
	}
}

func TestTrustLeaderboard_EmptyDir(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()

	result, err := TrustLeaderboard(nil)
	if err != nil {
		t.Fatalf("TrustLeaderboard: %v", err)
	}

	if len(result.Entries) != 0 {
		t.Errorf("Entries length = %d, want 0", len(result.Entries))
	}
}

func TestIsValidTrustEventType(t *testing.T) {
	for _, et := range ValidTrustEventTypes() {
		if !IsValidTrustEventType(string(et)) {
			t.Errorf("IsValidTrustEventType(%q) = false, want true", et)
		}
	}

	if IsValidTrustEventType("bogus") {
		t.Error("IsValidTrustEventType(bogus) = true, want false")
	}
}

func TestTrustFilePath(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := trustFilePath("my-session")
	want := filepath.Join(home, ".agm", "trust", "my-session.jsonl")
	if got != want {
		t.Errorf("trustFilePath(my-session) = %q, want %q", got, want)
	}
}

func TestTrustScore_AllEventTypes(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()

	// One of each: 50 + 5 - 15 - 5 - 3 - 1 - 10 + 0(gc_archived) = 21
	for _, et := range ValidTrustEventTypes() {
		if _, err := TrustRecord(nil, &TrustRecordRequest{
			SessionName: "all-types",
			EventType:   string(et),
		}); err != nil {
			t.Fatalf("TrustRecord(%s): %v", et, err)
		}
	}

	result, err := TrustScore(nil, &TrustScoreRequest{SessionName: "all-types"})
	if err != nil {
		t.Fatalf("TrustScore: %v", err)
	}

	if result.Score != 21 {
		t.Errorf("Score = %d, want 21", result.Score)
	}
	if result.TotalEvents != 7 {
		t.Errorf("TotalEvents = %d, want 7", result.TotalEvents)
	}
	if len(result.Breakdown) != 7 {
		t.Errorf("Breakdown length = %d, want 7", len(result.Breakdown))
	}
}

func TestRecordTrustEventForSession(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()

	err := RecordTrustEventForSession("my-session", TrustEventSuccess, "test detail")
	if err != nil {
		t.Fatalf("RecordTrustEventForSession: %v", err)
	}

	// Verify event was recorded
	events, err := readTrustEvents("my-session")
	if err != nil {
		t.Fatalf("readTrustEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != TrustEventSuccess {
		t.Errorf("EventType = %q, want %q", events[0].EventType, TrustEventSuccess)
	}
	if events[0].Detail != "test detail" {
		t.Errorf("Detail = %q, want %q", events[0].Detail, "test detail")
	}
	if events[0].SessionName != "my-session" {
		t.Errorf("SessionName = %q, want %q", events[0].SessionName, "my-session")
	}
}

func TestTrustEventGCArchived_ZeroDelta(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()

	// Record one success (+5) and one gc_archived (+0): score = 50 + 5 = 55
	for _, req := range []TrustRecordRequest{
		{SessionName: "gc-test", EventType: "success"},
		{SessionName: "gc-test", EventType: "gc_archived"},
	} {
		if _, err := TrustRecord(nil, &req); err != nil {
			t.Fatalf("TrustRecord(%s): %v", req.EventType, err)
		}
	}

	result, err := TrustScore(nil, &TrustScoreRequest{SessionName: "gc-test"})
	if err != nil {
		t.Fatalf("TrustScore: %v", err)
	}

	if result.Score != 55 {
		t.Errorf("Score = %d, want 55 (gc_archived has no score impact)", result.Score)
	}
	if result.TotalEvents != 2 {
		t.Errorf("TotalEvents = %d, want 2", result.TotalEvents)
	}
}
