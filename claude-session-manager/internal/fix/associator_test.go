package fix

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/detection"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/history"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
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

	detector := detection.NewDetector(historyFile, 5*time.Minute)
	parser := history.NewParser(historyFile)
	assoc := NewAssociator(detector, parser)

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
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	projectDir := filepath.Join(tmpDir, "test-project")
	os.MkdirAll(projectDir, 0755)
	os.MkdirAll(sessionsDir, 0755)

	detector := detection.NewDetector("", 5*time.Minute)
	parser := history.NewParser("")
	assoc := NewAssociator(detector, parser)

	t.Run("successfully associates UUID", func(t *testing.T) {
		m := &manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     "test-session",
			Name:          "test-session",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Context:       manifest.Context{Project: projectDir},
			Claude:        manifest.Claude{},
			Tmux:          manifest.Tmux{SessionName: "test-tmux"},
		}

		sessionDir := filepath.Join(sessionsDir, m.SessionID)
		os.MkdirAll(sessionDir, 0755)
		manifestPath := filepath.Join(sessionDir, "manifest.yaml")
		manifest.Write(manifestPath, m)

		// Associate UUID
		err := assoc.Associate(m, manifestPath, "new-uuid-123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify in-memory update
		if m.Claude.UUID != "new-uuid-123" {
			t.Errorf("expected UUID 'new-uuid-123', got '%s'", m.Claude.UUID)
		}

		// Verify persistence
		reloaded, err := manifest.Read(manifestPath)
		if err != nil {
			t.Fatalf("failed to reload manifest: %v", err)
		}

		if reloaded.Claude.UUID != "new-uuid-123" {
			t.Errorf("expected persisted UUID 'new-uuid-123', got '%s'", reloaded.Claude.UUID)
		}
	})

	t.Run("rejects empty UUID", func(t *testing.T) {
		m := &manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     "test-session-2",
			Name:          "test-session-2",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Context:       manifest.Context{Project: projectDir},
			Claude:        manifest.Claude{},
			Tmux:          manifest.Tmux{SessionName: "test-tmux-2"},
		}

		sessionDir := filepath.Join(sessionsDir, m.SessionID)
		os.MkdirAll(sessionDir, 0755)
		manifestPath := filepath.Join(sessionDir, "manifest.yaml")

		err := assoc.Associate(m, manifestPath, "")
		if err == nil {
			t.Error("expected error for empty UUID")
		}
	})
}

func TestClear(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	projectDir := filepath.Join(tmpDir, "test-project")
	os.MkdirAll(projectDir, 0755)
	os.MkdirAll(sessionsDir, 0755)

	detector := detection.NewDetector("", 5*time.Minute)
	parser := history.NewParser("")
	assoc := NewAssociator(detector, parser)

	t.Run("clears UUID association", func(t *testing.T) {
		m := &manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     "test-session",
			Name:          "test-session",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Context:       manifest.Context{Project: projectDir},
			Claude:        manifest.Claude{UUID: "existing-uuid"},
			Tmux:          manifest.Tmux{SessionName: "test-tmux"},
		}

		sessionDir := filepath.Join(sessionsDir, m.SessionID)
		os.MkdirAll(sessionDir, 0755)
		manifestPath := filepath.Join(sessionDir, "manifest.yaml")
		manifest.Write(manifestPath, m)

		// Clear UUID
		err := assoc.Clear(m, manifestPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify cleared
		if m.Claude.UUID != "" {
			t.Errorf("expected empty UUID, got '%s'", m.Claude.UUID)
		}

		// Verify persistence
		reloaded, err := manifest.Read(manifestPath)
		if err != nil {
			t.Fatalf("failed to reload manifest: %v", err)
		}

		if reloaded.Claude.UUID != "" {
			t.Errorf("expected persisted UUID to be empty, got '%s'", reloaded.Claude.UUID)
		}
	})
}

func TestScanUnassociated(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	os.MkdirAll(sessionsDir, 0755)

	// Create manifests with and without UUIDs
	createTestManifest(t, sessionsDir, "session-1", "")         // No UUID
	createTestManifest(t, sessionsDir, "session-2", "uuid-123") // Has UUID
	createTestManifest(t, sessionsDir, "session-3", "")         // No UUID

	detector := detection.NewDetector("", 5*time.Minute)
	parser := history.NewParser("")
	assoc := NewAssociator(detector, parser)

	unassociated, err := assoc.ScanUnassociated(sessionsDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(unassociated) != 2 {
		t.Errorf("expected 2 unassociated sessions, got %d", len(unassociated))
	}

	// Verify correct sessions identified
	names := make(map[string]bool)
	for _, m := range unassociated {
		names[m.Name] = true
	}

	if !names["session-1"] || !names["session-3"] {
		t.Error("expected session-1 and session-3 to be unassociated")
	}
}

func TestScanBroken(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	os.MkdirAll(sessionsDir, 0755)

	// Create history with one UUID
	historyFile := createTestHistory(t, tmpDir, []string{
		createHistoryEntry("valid-uuid", "/tmp/project", time.Now()),
	})

	// Create manifests with different UUIDs
	createTestManifest(t, sessionsDir, "session-1", "valid-uuid")   // In history
	createTestManifest(t, sessionsDir, "session-2", "invalid-uuid") // Not in history
	createTestManifest(t, sessionsDir, "session-3", "")             // No UUID

	detector := detection.NewDetector(historyFile, 5*time.Minute)
	parser := history.NewParser(historyFile)
	assoc := NewAssociator(detector, parser)

	broken, err := assoc.ScanBroken(sessionsDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(broken) != 1 {
		t.Errorf("expected 1 broken session, got %d", len(broken))
	}

	if len(broken) > 0 && broken[0].Name != "session-2" {
		t.Errorf("expected session-2 to be broken, got %s", broken[0].Name)
	}
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

func createTestManifest(t *testing.T, sessionsDir, name, uuid string) {
	t.Helper()

	m := &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     name,
		Name:          name,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context:       manifest.Context{Project: "/tmp/" + name},
		Claude:        manifest.Claude{UUID: uuid},
		Tmux:          manifest.Tmux{SessionName: "tmux-" + name},
	}

	sessionDir := filepath.Join(sessionsDir, name)
	os.MkdirAll(sessionDir, 0755)
	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	manifest.Write(manifestPath, m)
}
