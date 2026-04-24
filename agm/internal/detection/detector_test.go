package detection

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestNewDetector(t *testing.T) {
	t.Run("default window", func(t *testing.T) {
		d := NewDetector("", 0, nil)
		if d.detectionWindow != 5*time.Minute {
			t.Errorf("expected 5 minute window, got %v", d.detectionWindow)
		}
	})

	t.Run("custom window", func(t *testing.T) {
		customWindow := 10 * time.Minute
		d := NewDetector("", customWindow, nil)
		if d.detectionWindow != customWindow {
			t.Errorf("expected %v window, got %v", customWindow, d.detectionWindow)
		}
	})
}

func TestDetectUUID(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "test-project")
	os.MkdirAll(projectDir, 0755)

	// Create history file
	historyFile := createTestHistory(t, tmpDir, []string{
		// Recent entry (within 5 min window)
		createHistoryEntry("uuid-recent", projectDir, time.Now().Add(-2*time.Minute)),
	})

	d := NewDetector(historyFile, 5*time.Minute, nil)

	t.Run("manual UUID takes precedence", func(t *testing.T) {
		m := &manifest.Manifest{
			Context: manifest.Context{Project: projectDir},
			Claude:  manifest.Claude{UUID: "manual-uuid-123"},
		}

		result, err := d.DetectUUID(m)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Source != "manual" {
			t.Errorf("expected source 'manual', got '%s'", result.Source)
		}
		if result.UUID != "manual-uuid-123" {
			t.Errorf("expected manual UUID, got '%s'", result.UUID)
		}
		if result.Confidence != "high" {
			t.Errorf("expected high confidence, got '%s'", result.Confidence)
		}
	})

	t.Run("detects from recent history", func(t *testing.T) {
		m := &manifest.Manifest{
			Context: manifest.Context{Project: projectDir},
			Claude:  manifest.Claude{},
		}

		result, err := d.DetectUUID(m)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Source != "history" {
			t.Errorf("expected source 'history', got '%s'", result.Source)
		}
		if result.UUID != "uuid-recent" {
			t.Errorf("expected 'uuid-recent', got '%s'", result.UUID)
		}
		if result.Confidence != "high" {
			t.Errorf("expected high confidence for recent match, got '%s'", result.Confidence)
		}
	})

	t.Run("medium confidence for older entry", func(t *testing.T) {
		// Create history with entry from 3 minutes ago (middle of 5-min window)
		historyFile := createTestHistory(t, tmpDir, []string{
			createHistoryEntry("uuid-medium", projectDir, time.Now().Add(-3*time.Minute)),
		})

		d := NewDetector(historyFile, 5*time.Minute, nil)
		m := &manifest.Manifest{
			Context: manifest.Context{Project: projectDir},
			Claude:  manifest.Claude{},
		}

		result, err := d.DetectUUID(m)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Confidence != "medium" {
			t.Errorf("expected medium confidence, got '%s'", result.Confidence)
		}
	})

	t.Run("low confidence for old entry", func(t *testing.T) {
		// Create history with entry from 10 minutes ago (outside 5-min window)
		historyFile := createTestHistory(t, tmpDir, []string{
			createHistoryEntry("uuid-old", projectDir, time.Now().Add(-10*time.Minute)),
		})

		d := NewDetector(historyFile, 5*time.Minute, nil)
		m := &manifest.Manifest{
			Context: manifest.Context{Project: projectDir},
			Claude:  manifest.Claude{},
		}

		result, err := d.DetectUUID(m)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Confidence != "low" {
			t.Errorf("expected low confidence for old entry, got '%s'", result.Confidence)
		}
	})

	t.Run("no match in history", func(t *testing.T) {
		historyFile := createTestHistory(t, tmpDir, []string{
			createHistoryEntry("uuid-other", "/tmp/other-project", time.Now()),
		})

		d := NewDetector(historyFile, 5*time.Minute, nil)
		m := &manifest.Manifest{
			Context: manifest.Context{Project: projectDir},
			Claude:  manifest.Claude{},
		}

		result, err := d.DetectUUID(m)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Source != "none" {
			t.Errorf("expected source 'none', got '%s'", result.Source)
		}
		if result.Confidence != "low" {
			t.Errorf("expected low confidence, got '%s'", result.Confidence)
		}
	})
}

func TestDetectAndAssociate(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	projectDir := filepath.Join(tmpDir, "test-project")
	os.MkdirAll(projectDir, 0755)
	os.MkdirAll(sessionsDir, 0755)

	// Create history with recent entry
	historyFile := createTestHistory(t, tmpDir, []string{
		createHistoryEntry("uuid-auto-detect", projectDir, time.Now().Add(-1*time.Minute)),
	})

	d := NewDetector(historyFile, 5*time.Minute, nil)

	// Phase 6: "auto-apply with high confidence" sub-test deleted -
	// it relied on manifest.Read/Write which are now stubs.

	t.Run("no auto-apply without flag", func(t *testing.T) {
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

		// Detect without auto-apply
		associated, err := d.DetectAndAssociate(m, manifestPath, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if associated {
			t.Error("expected UUID not to be auto-applied")
		}

		if m.Claude.UUID != "" {
			t.Errorf("expected UUID to remain empty, got '%s'", m.Claude.UUID)
		}
	})

	t.Run("no auto-apply with low confidence", func(t *testing.T) {
		// Create history with old entry (low confidence)
		historyFile := createTestHistory(t, tmpDir, []string{
			createHistoryEntry("uuid-old", projectDir, time.Now().Add(-10*time.Minute)),
		})

		d := NewDetector(historyFile, 5*time.Minute, nil)

		m := &manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     "test-session-3",
			Name:          "test-session-3",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Context:       manifest.Context{Project: projectDir},
			Claude:        manifest.Claude{},
			Tmux:          manifest.Tmux{SessionName: "test-tmux-3"},
		}

		sessionDir := filepath.Join(sessionsDir, m.SessionID)
		os.MkdirAll(sessionDir, 0755)
		manifestPath := filepath.Join(sessionDir, "manifest.yaml")

		// Try to detect and associate (should fail due to low confidence)
		associated, err := d.DetectAndAssociate(m, manifestPath, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if associated {
			t.Error("expected low confidence UUID not to be auto-applied")
		}
	})
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
