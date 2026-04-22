package hippocampus

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrackParent(t *testing.T) {
	dir := t.TempDir()
	store := NewLineageStore(filepath.Join(dir, "lineage.json"))

	if err := store.TrackParent("child-1", "parent-1", "clear"); err != nil {
		t.Fatalf("TrackParent: %v", err)
	}

	state, err := store.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(state.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(state.Sessions))
	}

	s := state.Sessions[0]
	if s.SessionID != "child-1" {
		t.Errorf("SessionID = %q, want %q", s.SessionID, "child-1")
	}
	if s.ParentSessionID != "parent-1" {
		t.Errorf("ParentSessionID = %q, want %q", s.ParentSessionID, "parent-1")
	}
	if s.TransitionType != "clear" {
		t.Errorf("TransitionType = %q, want %q", s.TransitionType, "clear")
	}
}

func TestRegenerateSession(t *testing.T) {
	dir := t.TempDir()
	store := NewLineageStore(filepath.Join(dir, "lineage.json"))

	newID, err := store.RegenerateSession("original-session", "plan_to_impl")
	if err != nil {
		t.Fatalf("RegenerateSession: %v", err)
	}

	if newID == "" {
		t.Fatal("expected non-empty new session ID")
	}
	if newID == "original-session" {
		t.Fatal("new ID should differ from original")
	}

	// Verify lineage chain
	chain, err := store.GetLineageChain(newID)
	if err != nil {
		t.Fatalf("GetLineageChain: %v", err)
	}
	if len(chain) != 1 {
		t.Fatalf("expected chain length 1, got %d", len(chain))
	}
	if chain[0].ParentSessionID != "original-session" {
		t.Errorf("parent = %q, want %q", chain[0].ParentSessionID, "original-session")
	}
	if chain[0].TransitionType != "plan_to_impl" {
		t.Errorf("transition = %q, want %q", chain[0].TransitionType, "plan_to_impl")
	}
}

func TestGetLineageChain(t *testing.T) {
	dir := t.TempDir()
	store := NewLineageStore(filepath.Join(dir, "lineage.json"))

	// Build chain: root -> mid -> leaf
	if err := store.TrackParent("mid", "root", "clear"); err != nil {
		t.Fatal(err)
	}
	if err := store.TrackParent("leaf", "mid", "plan_to_impl"); err != nil {
		t.Fatal(err)
	}

	chain, err := store.GetLineageChain("leaf")
	if err != nil {
		t.Fatalf("GetLineageChain: %v", err)
	}

	if len(chain) != 2 {
		t.Fatalf("expected chain length 2, got %d", len(chain))
	}

	// Chain should be leaf -> mid (root not tracked as a session entry)
	if chain[0].SessionID != "leaf" {
		t.Errorf("chain[0] = %q, want %q", chain[0].SessionID, "leaf")
	}
	if chain[1].SessionID != "mid" {
		t.Errorf("chain[1] = %q, want %q", chain[1].SessionID, "mid")
	}
}

func TestGetLineageChain_CycleProtection(t *testing.T) {
	dir := t.TempDir()
	store := NewLineageStore(filepath.Join(dir, "lineage.json"))

	// Create a cycle: a -> b -> a
	if err := store.TrackParent("a", "b", "clear"); err != nil {
		t.Fatal(err)
	}
	if err := store.TrackParent("b", "a", "clear"); err != nil {
		t.Fatal(err)
	}

	chain, err := store.GetLineageChain("a")
	if err != nil {
		t.Fatalf("GetLineageChain with cycle: %v", err)
	}

	// Should not loop forever; chain length should be exactly 2
	if len(chain) != 2 {
		t.Fatalf("expected chain length 2 with cycle, got %d", len(chain))
	}
}

func TestMarkTeleported(t *testing.T) {
	dir := t.TempDir()
	store := NewLineageStore(filepath.Join(dir, "lineage.json"))

	// Track a session first
	if err := store.TrackParent("session-1", "parent-1", "clear"); err != nil {
		t.Fatal(err)
	}

	// Mark as teleported
	if err := store.MarkTeleported("session-1"); err != nil {
		t.Fatalf("MarkTeleported: %v", err)
	}

	state, err := store.load()
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, s := range state.Sessions {
		if s.SessionID == "session-1" {
			if !s.Teleported {
				t.Error("expected Teleported=true")
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("session-1 not found in state")
	}
}

func TestMarkTeleported_NewSession(t *testing.T) {
	dir := t.TempDir()
	store := NewLineageStore(filepath.Join(dir, "lineage.json"))

	// Mark teleported for a session not yet tracked
	if err := store.MarkTeleported("new-session"); err != nil {
		t.Fatalf("MarkTeleported: %v", err)
	}

	state, err := store.load()
	if err != nil {
		t.Fatal(err)
	}

	if len(state.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(state.Sessions))
	}
	if !state.Sessions[0].Teleported {
		t.Error("expected Teleported=true for new session")
	}
}

func TestLoadEmptyFile(t *testing.T) {
	dir := t.TempDir()
	store := NewLineageStore(filepath.Join(dir, "nonexistent.json"))

	state, err := store.load()
	if err != nil {
		t.Fatalf("load nonexistent: %v", err)
	}
	if len(state.Sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(state.Sessions))
	}
}

func TestGenerateSessionID(t *testing.T) {
	id1, err := generateSessionID()
	if err != nil {
		t.Fatalf("generateSessionID: %v", err)
	}
	id2, err := generateSessionID()
	if err != nil {
		t.Fatalf("generateSessionID: %v", err)
	}

	if len(id1) != 16 { // 8 bytes = 16 hex chars
		t.Errorf("expected 16 char ID, got %d", len(id1))
	}
	if id1 == id2 {
		t.Error("expected unique IDs")
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c", "lineage.json")
	store := NewLineageStore(nested)

	if err := store.TrackParent("s1", "s0", "clear"); err != nil {
		t.Fatalf("TrackParent with nested dir: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(nested); err != nil {
		t.Fatalf("state file not created: %v", err)
	}
}
