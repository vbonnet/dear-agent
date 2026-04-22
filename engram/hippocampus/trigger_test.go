package hippocampus

import (
	"path/filepath"
	"testing"
	"time"
)

func TestShouldConsolidate_BothConditionsMet(t *testing.T) {
	state := TriggerState{
		LastConsolidation: time.Now().Add(-25 * time.Hour),
		SessionCount:      5,
	}
	if !ShouldConsolidate(state, 24*time.Hour, 5) {
		t.Fatal("expected true when both conditions met")
	}
}

func TestShouldConsolidate_TimeNotMet(t *testing.T) {
	state := TriggerState{
		LastConsolidation: time.Now().Add(-1 * time.Hour),
		SessionCount:      10,
	}
	if ShouldConsolidate(state, 24*time.Hour, 5) {
		t.Fatal("expected false when time condition not met")
	}
}

func TestShouldConsolidate_SessionsNotMet(t *testing.T) {
	state := TriggerState{
		LastConsolidation: time.Now().Add(-48 * time.Hour),
		SessionCount:      2,
	}
	if ShouldConsolidate(state, 24*time.Hour, 5) {
		t.Fatal("expected false when session count not met")
	}
}

func TestShouldConsolidate_ZeroState(t *testing.T) {
	state := TriggerState{}
	if !ShouldConsolidate(state, 24*time.Hour, 0) {
		t.Fatal("expected true for zero state (first run)")
	}
}

func TestLoadTriggerState_FileNotExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")
	state, err := LoadTriggerState(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !state.LastConsolidation.IsZero() {
		t.Fatal("expected zero-value LastConsolidation")
	}
	if state.SessionCount != 0 {
		t.Fatal("expected zero SessionCount")
	}
	if state.LastSessionID != "" {
		t.Fatal("expected empty LastSessionID")
	}
}

func TestSaveTriggerState_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	original := TriggerState{
		LastConsolidation: time.Now().Truncate(time.Second),
		SessionCount:      7,
		LastSessionID:     "session-42",
	}

	if err := SaveTriggerState(path, original); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := LoadTriggerState(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if !original.LastConsolidation.Equal(loaded.LastConsolidation) {
		t.Fatalf("LastConsolidation mismatch: %v != %v", original.LastConsolidation, loaded.LastConsolidation)
	}
	if original.SessionCount != loaded.SessionCount {
		t.Fatalf("SessionCount mismatch: %d != %d", original.SessionCount, loaded.SessionCount)
	}
	if original.LastSessionID != loaded.LastSessionID {
		t.Fatalf("LastSessionID mismatch: %q != %q", original.LastSessionID, loaded.LastSessionID)
	}
}

func TestSaveTriggerState_CreatesDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deep", "state.json")
	state := TriggerState{SessionCount: 1}

	if err := SaveTriggerState(path, state); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := LoadTriggerState(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded.SessionCount != 1 {
		t.Fatalf("expected SessionCount 1, got %d", loaded.SessionCount)
	}
}

func TestIncrementSession(t *testing.T) {
	state := TriggerState{SessionCount: 3, LastSessionID: "old"}
	state.IncrementSession("new-session")

	if state.SessionCount != 4 {
		t.Fatalf("expected SessionCount 4, got %d", state.SessionCount)
	}
	if state.LastSessionID != "new-session" {
		t.Fatalf("expected LastSessionID %q, got %q", "new-session", state.LastSessionID)
	}
}

func TestOnSessionEnd(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json")

	// First session end
	if err := OnSessionEnd(stateFile, "session-1"); err != nil {
		t.Fatalf("OnSessionEnd failed: %v", err)
	}

	state, err := LoadTriggerState(stateFile)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if state.SessionCount != 1 {
		t.Fatalf("expected SessionCount 1, got %d", state.SessionCount)
	}
	if state.LastSessionID != "session-1" {
		t.Fatalf("expected LastSessionID %q, got %q", "session-1", state.LastSessionID)
	}

	// Second session end
	if err := OnSessionEnd(stateFile, "session-2"); err != nil {
		t.Fatalf("OnSessionEnd failed: %v", err)
	}

	state, err = LoadTriggerState(stateFile)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if state.SessionCount != 2 {
		t.Fatalf("expected SessionCount 2, got %d", state.SessionCount)
	}
	if state.LastSessionID != "session-2" {
		t.Fatalf("expected LastSessionID %q, got %q", "session-2", state.LastSessionID)
	}
}

func TestResetAfterConsolidation(t *testing.T) {
	state := TriggerState{
		SessionCount:      10,
		LastSessionID:     "some-session",
		LastConsolidation: time.Time{},
	}

	before := time.Now()
	state.ResetAfterConsolidation()
	after := time.Now()

	if state.SessionCount != 0 {
		t.Fatalf("expected SessionCount 0, got %d", state.SessionCount)
	}
	if state.LastConsolidation.Before(before) || state.LastConsolidation.After(after) {
		t.Fatalf("LastConsolidation %v not in range [%v, %v]", state.LastConsolidation, before, after)
	}
}
