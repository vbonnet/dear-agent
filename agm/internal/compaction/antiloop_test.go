package compaction

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadState_FileNotExist(t *testing.T) {
	dir := t.TempDir()
	state, err := LoadState(dir, "test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.SessionName != "test-session" {
		t.Errorf("SessionName = %q, want %q", state.SessionName, "test-session")
	}
	if state.CompactionCount != 0 {
		t.Errorf("CompactionCount = %d, want 0", state.CompactionCount)
	}
	if !state.LastCompaction.IsZero() {
		t.Errorf("LastCompaction should be zero, got %v", state.LastCompaction)
	}
}

func TestSaveAndLoadState(t *testing.T) {
	dir := t.TempDir()
	original := &CompactionState{
		SessionName:     "my-session",
		LastCompaction:  time.Now().Add(-1 * time.Hour),
		CompactionCount: 2,
		History: []CompactionRecord{
			{Timestamp: time.Now().Add(-2 * time.Hour), PromptFile: "/tmp/p1.md", Forced: false},
			{Timestamp: time.Now().Add(-1 * time.Hour), PromptFile: "/tmp/p2.md", Forced: true},
		},
	}

	if err := SaveState(dir, original); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "compaction-state", "my-session.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state file not created: %v", err)
	}

	loaded, err := LoadState(dir, "my-session")
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if loaded.CompactionCount != 2 {
		t.Errorf("CompactionCount = %d, want 2", loaded.CompactionCount)
	}
	if len(loaded.History) != 2 {
		t.Errorf("History length = %d, want 2", len(loaded.History))
	}
}

func TestCheckAntiLoop_FirstCompaction(t *testing.T) {
	state := &CompactionState{SessionName: "new-session"}
	if err := CheckAntiLoop(state, false); err != nil {
		t.Errorf("first compaction should be allowed: %v", err)
	}
}

func TestCheckAntiLoop_WithinCooldown(t *testing.T) {
	state := &CompactionState{
		SessionName:     "busy-session",
		LastCompaction:  time.Now().Add(-30 * time.Minute),
		CompactionCount: 1,
	}
	err := CheckAntiLoop(state, false)
	if err == nil {
		t.Error("should block compaction within cooldown")
	}
}

func TestCheckAntiLoop_BeyondCooldown(t *testing.T) {
	state := &CompactionState{
		SessionName:     "ready-session",
		LastCompaction:  time.Now().Add(-3 * time.Hour),
		CompactionCount: 1,
	}
	if err := CheckAntiLoop(state, false); err != nil {
		t.Errorf("should allow compaction beyond cooldown: %v", err)
	}
}

func TestCheckAntiLoop_AtMaxCount(t *testing.T) {
	now := time.Now()
	state := &CompactionState{
		SessionName:     "exhausted-session",
		LastCompaction:  now.Add(-3 * time.Hour),
		CompactionCount: 3,
		History: []CompactionRecord{
			{Timestamp: now.Add(-10 * time.Hour)},
			{Timestamp: now.Add(-6 * time.Hour)},
			{Timestamp: now.Add(-3 * time.Hour)},
		},
	}
	err := CheckAntiLoop(state, false)
	if err == nil {
		t.Error("should block compaction at max count within window")
	}
}

func TestCheckAntiLoop_ExpiredCompactionsAllowMore(t *testing.T) {
	now := time.Now()
	state := &CompactionState{
		SessionName:     "long-session",
		LastCompaction:  now.Add(-3 * time.Hour),
		CompactionCount: 5,
		History: []CompactionRecord{
			{Timestamp: now.Add(-72 * time.Hour)},
			{Timestamp: now.Add(-48 * time.Hour)},
			{Timestamp: now.Add(-30 * time.Hour)},
			{Timestamp: now.Add(-25 * time.Hour)},
			{Timestamp: now.Add(-3 * time.Hour)},
		},
	}
	if err := CheckAntiLoop(state, false); err != nil {
		t.Errorf("should allow compaction when old entries expired: %v", err)
	}
}

func TestCheckAntiLoop_ForceBypassesLimit(t *testing.T) {
	now := time.Now()
	state := &CompactionState{
		SessionName:     "exhausted-session",
		LastCompaction:  now.Add(-3 * time.Hour),
		CompactionCount: 3,
		History: []CompactionRecord{
			{Timestamp: now.Add(-10 * time.Hour)},
			{Timestamp: now.Add(-6 * time.Hour)},
			{Timestamp: now.Add(-3 * time.Hour)},
		},
	}
	err := CheckAntiLoop(state, true)
	if err != nil {
		t.Errorf("--force should bypass max count: %v", err)
	}
}

func TestCheckAntiLoop_ForceBypassesCooldown(t *testing.T) {
	state := &CompactionState{
		SessionName:     "busy-session",
		LastCompaction:  time.Now().Add(-30 * time.Minute),
		CompactionCount: 1,
	}
	err := CheckAntiLoop(state, true)
	if err != nil {
		t.Errorf("--force should bypass cooldown: %v", err)
	}
}

func TestRecordCompaction(t *testing.T) {
	state := &CompactionState{
		SessionName:     "test-session",
		CompactionCount: 1,
	}
	result := RecordCompaction(state, "/tmp/prompt.md", false)
	if result.CompactionCount != 2 {
		t.Errorf("CompactionCount = %d, want 2", result.CompactionCount)
	}
	if result.LastCompaction.IsZero() {
		t.Error("LastCompaction should be set")
	}
	if len(result.History) != 1 {
		t.Fatalf("History length = %d, want 1", len(result.History))
	}
	if result.History[0].PromptFile != "/tmp/prompt.md" {
		t.Errorf("PromptFile = %q, want %q", result.History[0].PromptFile, "/tmp/prompt.md")
	}
	if result.History[0].Forced {
		t.Error("Forced should be false")
	}
}
