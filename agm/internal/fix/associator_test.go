package fix

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/detection"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/history"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/testutil"
)

func TestGetSuggestions(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "test-project")
	os.MkdirAll(projectDir, 0755)

	// Create history with multiple entries
	historyFile := createTestHistory(t, tmpDir, []string{
		createHistoryEntry("uuid-recent", projectDir, time.Now().Add(-1*time.Minute)),
		createHistoryEntry("uuid-other-1", "/tmp/other-1", time.Now().Add(-5*time.Minute)),
		createHistoryEntry("uuid-other-2", "/tmp/other-2", time.Now().Add(-10*time.Minute)),
	})

	detector := detection.NewDetector(historyFile, 5*time.Minute, nil)
	parser := history.NewParser(historyFile)
	assoc := NewAssociator(detector, parser, nil)

	t.Run("includes auto-detected UUID", func(t *testing.T) {
		m := &manifest.Manifest{
			Context: manifest.Context{Project: projectDir},
			Claude:  manifest.Claude{},
		}

		suggestions, err := assoc.GetSuggestions(m, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(suggestions) == 0 {
			t.Fatal("expected at least one suggestion")
		}

		// First suggestion should be auto-detected
		if suggestions[0].UUID != "uuid-recent" {
			t.Errorf("expected first suggestion 'uuid-recent', got '%s'", suggestions[0].UUID)
		}
		if suggestions[0].Source != "history" {
			t.Errorf("expected source 'history', got '%s'", suggestions[0].Source)
		}
	})

	t.Run("includes recent history entries", func(t *testing.T) {
		m := &manifest.Manifest{
			Context: manifest.Context{Project: "/tmp/unrelated"},
			Claude:  manifest.Claude{},
		}

		suggestions, err := assoc.GetSuggestions(m, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should have recent entries even without auto-detection
		if len(suggestions) < 2 {
			t.Errorf("expected at least 2 suggestions from recent history, got %d", len(suggestions))
		}
	})

	t.Run("respects limit", func(t *testing.T) {
		m := &manifest.Manifest{
			Context: manifest.Context{Project: "/tmp/unrelated"},
			Claude:  manifest.Claude{},
		}

		suggestions, err := assoc.GetSuggestions(m, 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(suggestions) > 2 {
			t.Errorf("expected at most 2 suggestions, got %d", len(suggestions))
		}
	})

	t.Run("no duplicate UUIDs", func(t *testing.T) {
		m := &manifest.Manifest{
			Context: manifest.Context{Project: projectDir},
			Claude:  manifest.Claude{},
		}

		suggestions, err := assoc.GetSuggestions(m, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Check for duplicates
		seen := make(map[string]bool)
		for _, s := range suggestions {
			if seen[s.UUID] {
				t.Errorf("duplicate UUID in suggestions: %s", s.UUID)
			}
			seen[s.UUID] = true
		}
	})
}

func TestAssociate(t *testing.T) {
	adapter := dolt.NewMockAdapter()

	detector := detection.NewDetector("", 5*time.Minute, nil)
	parser := history.NewParser("")
	assoc := NewAssociator(detector, parser, adapter)

	t.Run("successfully associates UUID", func(t *testing.T) {
		sessionID := "test-assoc-" + t.Name()
		m := &manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     sessionID,
			Name:          sessionID,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Context:       manifest.Context{Project: "/tmp/test"},
			Claude:        manifest.Claude{},
			Tmux:          manifest.Tmux{SessionName: "test-tmux"},
		}

		// Create session in mock adapter first
		if err := adapter.CreateSession(m); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}

		// Associate UUID
		err := assoc.Associate(m, "", "new-uuid-123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify in-memory update
		if m.Claude.UUID != "new-uuid-123" {
			t.Errorf("expected UUID 'new-uuid-123', got '%s'", m.Claude.UUID)
		}

		// Verify persistence in Dolt
		retrieved, err := adapter.GetSession(sessionID)
		if err != nil {
			t.Fatalf("failed to get session from adapter: %v", err)
		}
		if retrieved.Claude.UUID != "new-uuid-123" {
			t.Errorf("expected persisted UUID 'new-uuid-123', got '%s'", retrieved.Claude.UUID)
		}
	})

	t.Run("rejects empty UUID", func(t *testing.T) {
		m := &manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     "test-assoc-empty",
			Name:          "test-assoc-empty",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Context:       manifest.Context{Project: "/tmp/test"},
			Claude:        manifest.Claude{},
			Tmux:          manifest.Tmux{SessionName: "test-tmux-2"},
		}

		err := assoc.Associate(m, "", "")
		if err == nil {
			t.Error("expected error for empty UUID")
		}
	})
}

func TestClear(t *testing.T) {
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "test-project")
	os.MkdirAll(projectDir, 0755)

	detector := detection.NewDetector("", 5*time.Minute, adapter)
	parser := history.NewParser("")
	assoc := NewAssociator(detector, parser, adapter)

	t.Run("clears UUID association", func(t *testing.T) {
		sid := "test-clear-" + uuid.New().String()[:8]
		m := &manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     sid,
			Name:          sid,
			Harness:       "claude-code",
			Workspace:     "oss",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Context:       manifest.Context{Project: projectDir},
			Claude:        manifest.Claude{UUID: "existing-uuid"},
			Tmux:          manifest.Tmux{SessionName: "test-tmux"},
		}

		if err := adapter.CreateSession(m); err != nil {
			t.Fatalf("failed to create session: %v", err)
		}

		// Clear UUID
		err := assoc.Clear(m, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify cleared in-memory
		if m.Claude.UUID != "" {
			t.Errorf("expected empty UUID, got '%s'", m.Claude.UUID)
		}

		// Verify persistence in Dolt
		reloaded, err := adapter.GetSession(sid)
		if err != nil {
			t.Fatalf("failed to reload from Dolt: %v", err)
		}

		if reloaded.Claude.UUID != "" {
			t.Errorf("expected persisted UUID to be empty, got '%s'", reloaded.Claude.UUID)
		}

		// Cleanup
		_ = adapter.DeleteSession(sid)
	})
}

func TestScanUnassociated(t *testing.T) {
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	os.MkdirAll(sessionsDir, 0755)

	// Create test sessions directly in Dolt
	prefix := "test-unassoc-" + uuid.New().String()[:6]
	sid1 := prefix + "-1"
	sid2 := prefix + "-2"
	sid3 := prefix + "-3"

	for _, tc := range []struct {
		sid  string
		uuid string
	}{
		{sid1, ""},         // No UUID
		{sid2, "uuid-123"}, // Has UUID
		{sid3, ""},         // No UUID
	} {
		m := &manifest.Manifest{
			SchemaVersion: "2.0", SessionID: tc.sid, Name: tc.sid,
			Harness: "claude-code", Workspace: "oss",
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
			Context: manifest.Context{Project: "/tmp/" + tc.sid},
			Tmux:    manifest.Tmux{SessionName: "tmux-" + tc.sid},
			Claude:  manifest.Claude{UUID: tc.uuid},
		}
		if err := adapter.CreateSession(m); err != nil {
			t.Fatalf("failed to create session %s: %v", tc.sid, err)
		}
	}

	detector := detection.NewDetector("", 5*time.Minute, adapter)
	parser := history.NewParser("")
	assoc := NewAssociator(detector, parser, adapter)

	unassociated, err := assoc.ScanUnassociated(sessionsDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Filter to only our test sessions (Dolt may have other sessions)
	names := make(map[string]bool)
	for _, m := range unassociated {
		if m.SessionID == sid1 || m.SessionID == sid3 {
			names[m.SessionID] = true
		}
	}

	if len(names) != 2 {
		t.Errorf("expected 2 unassociated test sessions, got %d", len(names))
	}

	// Cleanup
	_ = adapter.DeleteSession(sid1)
	_ = adapter.DeleteSession(sid2)
	_ = adapter.DeleteSession(sid3)
}

func TestScanBroken(t *testing.T) {
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	os.MkdirAll(sessionsDir, 0755)

	validUUID := "valid-" + uuid.New().String()[:8]
	invalidUUID := "invalid-" + uuid.New().String()[:8]

	// Create history with one UUID
	historyFile := createTestHistory(t, tmpDir, []string{
		createHistoryEntry(validUUID, "/tmp/project", time.Now()),
	})

	// Create test sessions directly in Dolt
	prefix := "test-broken-" + uuid.New().String()[:6]
	sid1 := prefix + "-1"
	sid2 := prefix + "-2"
	sid3 := prefix + "-3"

	for _, tc := range []struct {
		sid  string
		uuid string
	}{
		{sid1, validUUID},   // In history
		{sid2, invalidUUID}, // Not in history
		{sid3, ""},          // No UUID
	} {
		m := &manifest.Manifest{
			SchemaVersion: "2.0", SessionID: tc.sid, Name: tc.sid,
			Harness: "claude-code", Workspace: "oss",
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
			Context: manifest.Context{Project: "/tmp/" + tc.sid},
			Tmux:    manifest.Tmux{SessionName: "tmux-" + tc.sid},
			Claude:  manifest.Claude{UUID: tc.uuid},
		}
		if err := adapter.CreateSession(m); err != nil {
			t.Fatalf("failed to create session %s: %v", tc.sid, err)
		}
	}

	detector := detection.NewDetector(historyFile, 5*time.Minute, adapter)
	parser := history.NewParser(historyFile)
	assoc := NewAssociator(detector, parser, adapter)

	broken, err := assoc.ScanBroken(sessionsDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Filter to our test sessions only
	var testBroken []*manifest.Manifest
	for _, m := range broken {
		if m.SessionID == sid2 {
			testBroken = append(testBroken, m)
		}
	}

	if len(testBroken) != 1 {
		t.Errorf("expected 1 broken test session, got %d", len(testBroken))
	}

	// Cleanup
	_ = adapter.DeleteSession(sid1)
	_ = adapter.DeleteSession(sid2)
	_ = adapter.DeleteSession(sid3)
}

// Helper functions

func createTestHistory(t *testing.T, dir string, entries []string) string {
	t.Helper()

	historyFile := filepath.Join(dir, "history.jsonl")
	f, err := os.Create(historyFile)
	if err != nil {
		t.Fatalf("failed to create history file: %v", err)
	}
	defer f.Close()

	for _, entry := range entries {
		f.WriteString(entry + "\n")
	}

	return historyFile
}

func createHistoryEntry(uuid, directory string, timestamp time.Time) string {
	return `{"uuid":"` + uuid + `","directory":"` + directory + `","timestamp":"` + timestamp.Format(time.RFC3339) + `"}`
}
