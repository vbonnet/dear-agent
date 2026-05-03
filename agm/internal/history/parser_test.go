package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewParser(t *testing.T) {
	t.Run("default path", func(t *testing.T) {
		p := NewParser("")
		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, ".claude", "history.jsonl")
		if p.historyPath != expected {
			t.Errorf("expected path %s, got %s", expected, p.historyPath)
		}
	})

	t.Run("custom path", func(t *testing.T) {
		customPath := "/tmp/custom-history.jsonl"
		p := NewParser(customPath)
		if p.historyPath != customPath {
			t.Errorf("expected path %s, got %s", customPath, p.historyPath)
		}
	})
}

func TestReadAll(t *testing.T) {
	t.Run("nonexistent file", func(t *testing.T) {
		p := NewParser("/tmp/nonexistent-history-file-12345.jsonl")
		entries, err := p.ReadAll()
		if err != nil {
			t.Errorf("expected no error for missing file, got: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("expected empty list, got %d entries", len(entries))
		}
	})

	t.Run("valid entries", func(t *testing.T) {
		tmpFile := createTestHistoryFile(t, []string{
			`{"uuid":"uuid-1","directory":"/tmp/project-1","timestamp":"2024-01-01T10:00:00Z"}`,
			`{"uuid":"uuid-2","directory":"/tmp/project-2","timestamp":"2024-01-02T10:00:00Z"}`,
			`{"uuid":"uuid-3","directory":"/tmp/project-3","timestamp":"2024-01-03T10:00:00Z","name":"my-session"}`,
		})
		defer os.Remove(tmpFile)

		p := NewParser(tmpFile)
		entries, err := p.ReadAll()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(entries) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(entries))
		}

		if entries[0].UUID != "uuid-1" {
			t.Errorf("expected uuid-1, got %s", entries[0].UUID)
		}
		if entries[2].Name != "my-session" {
			t.Errorf("expected name 'my-session', got '%s'", entries[2].Name)
		}
	})

	t.Run("malformed lines skipped", func(t *testing.T) {
		tmpFile := createTestHistoryFile(t, []string{
			`{"uuid":"uuid-1","directory":"/tmp/project-1","timestamp":"2024-01-01T10:00:00Z"}`,
			`this is not valid json`,
			`{"uuid":"uuid-2","directory":"/tmp/project-2","timestamp":"2024-01-02T10:00:00Z"}`,
		})
		defer os.Remove(tmpFile)

		p := NewParser(tmpFile)
		entries, err := p.ReadAll()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should get 2 valid entries, skipping the malformed one
		if len(entries) != 2 {
			t.Fatalf("expected 2 entries (skipping malformed), got %d", len(entries))
		}
	})

	t.Run("empty lines skipped", func(t *testing.T) {
		tmpFile := createTestHistoryFile(t, []string{
			`{"uuid":"uuid-1","directory":"/tmp/project-1","timestamp":"2024-01-01T10:00:00Z"}`,
			``,
			`{"uuid":"uuid-2","directory":"/tmp/project-2","timestamp":"2024-01-02T10:00:00Z"}`,
			``,
		})
		defer os.Remove(tmpFile)

		p := NewParser(tmpFile)
		entries, err := p.ReadAll()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(entries))
		}
	})
}

func TestFindByDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	project1 := filepath.Join(tmpDir, "project-1")
	project2 := filepath.Join(tmpDir, "project-2")

	tmpFile := createTestHistoryFile(t, []string{
		`{"uuid":"uuid-1-old","directory":"` + project1 + `","timestamp":"2024-01-01T10:00:00Z"}`,
		`{"uuid":"uuid-2","directory":"` + project2 + `","timestamp":"2024-01-02T10:00:00Z"}`,
		`{"uuid":"uuid-1-new","directory":"` + project1 + `","timestamp":"2024-01-03T10:00:00Z"}`,
	})
	defer os.Remove(tmpFile)

	p := NewParser(tmpFile)

	t.Run("finds most recent", func(t *testing.T) {
		entry, err := p.FindByDirectory(project1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if entry.UUID != "uuid-1-new" {
			t.Errorf("expected most recent UUID 'uuid-1-new', got '%s'", entry.UUID)
		}
	})

	t.Run("directory not found", func(t *testing.T) {
		_, err := p.FindByDirectory("/tmp/nonexistent-dir")
		if err == nil {
			t.Error("expected error for nonexistent directory")
		}
	})
}

func TestFindByUUID(t *testing.T) {
	tmpFile := createTestHistoryFile(t, []string{
		`{"uuid":"uuid-1","directory":"/tmp/project-1","timestamp":"2024-01-01T10:00:00Z"}`,
		`{"uuid":"uuid-2","directory":"/tmp/project-2","timestamp":"2024-01-02T10:00:00Z"}`,
		`{"uuid":"uuid-1","directory":"/tmp/project-3","timestamp":"2024-01-03T10:00:00Z"}`,
	})
	defer os.Remove(tmpFile)

	p := NewParser(tmpFile)

	t.Run("finds multiple entries", func(t *testing.T) {
		entries, err := p.FindByUUID("uuid-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(entries))
		}
	})

	t.Run("uuid not found", func(t *testing.T) {
		entries, err := p.FindByUUID("nonexistent-uuid")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(entries) != 0 {
			t.Errorf("expected 0 entries, got %d", len(entries))
		}
	})
}

func TestGetRecentEntries(t *testing.T) {
	tmpFile := createTestHistoryFile(t, []string{
		`{"uuid":"uuid-1","directory":"/tmp/project-1","timestamp":"2024-01-01T10:00:00Z"}`,
		`{"uuid":"uuid-2","directory":"/tmp/project-2","timestamp":"2024-01-03T10:00:00Z"}`,
		`{"uuid":"uuid-3","directory":"/tmp/project-3","timestamp":"2024-01-02T10:00:00Z"}`,
	})
	defer os.Remove(tmpFile)

	p := NewParser(tmpFile)

	t.Run("limit results", func(t *testing.T) {
		entries, err := p.GetRecentEntries(2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(entries))
		}

		// Should be sorted by timestamp descending
		if entries[0].UUID != "uuid-2" {
			t.Errorf("expected most recent 'uuid-2', got '%s'", entries[0].UUID)
		}
		if entries[1].UUID != "uuid-3" {
			t.Errorf("expected second most recent 'uuid-3', got '%s'", entries[1].UUID)
		}
	})

	t.Run("no limit", func(t *testing.T) {
		entries, err := p.GetRecentEntries(0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(entries) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(entries))
		}
	})

	t.Run("limit exceeds count", func(t *testing.T) {
		entries, err := p.GetRecentEntries(10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(entries) != 3 {
			t.Fatalf("expected 3 entries (all available), got %d", len(entries))
		}
	})
}

// Helper function to create a test history file
func TestReadAll_MixedFormats(t *testing.T) {
	// Test that parser handles both old format (RFC 3339 timestamps) and
	// new format (Unix millisecond timestamps) without emitting warnings
	tmpFile := createTestHistoryFile(t, []string{
		// New format entries (integer timestamps) - should be silently skipped
		`{"display":"Test conversation","pastedContents":{},"timestamp":1704067200000,"project":"/home/user","sessionId":"abc123"}`,
		`{"display":"Another test","pastedContents":{},"timestamp":1704153600000,"project":"/tmp/test"}`,
		// Old format entries (RFC 3339 timestamps) - should be parsed successfully
		`{"uuid":"uuid-1","directory":"/tmp/project-1","timestamp":"2024-01-01T10:00:00Z"}`,
		`{"uuid":"uuid-2","directory":"/tmp/project-2","timestamp":"2024-01-02T10:00:00Z","name":"old-session"}`,
	})
	defer os.Remove(tmpFile)

	p := NewParser(tmpFile)
	entries, err := p.ReadAll()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only get old format entries (2), new format entries are skipped
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Verify the old format entries were parsed correctly
	if entries[0].UUID != "uuid-1" {
		t.Errorf("expected UUID uuid-1, got %s", entries[0].UUID)
	}
	if entries[1].UUID != "uuid-2" {
		t.Errorf("expected UUID uuid-2, got %s", entries[1].UUID)
	}
	if entries[1].Name != "old-session" {
		t.Errorf("expected name old-session, got %s", entries[1].Name)
	}
}

func createTestHistoryFile(t *testing.T, lines []string) string {
	t.Helper()

	tmpFile, err := os.CreateTemp(t.TempDir(), "history-test-*.jsonl")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	for _, line := range lines {
		if _, err := tmpFile.WriteString(line + "\n"); err != nil {
			t.Fatalf("failed to write to temp file: %v", err)
		}
	}

	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	return tmpFile.Name()
}

func TestEntry_Timestamps(t *testing.T) {
	// Verify timestamp parsing works correctly
	tmpFile := createTestHistoryFile(t, []string{
		`{"uuid":"uuid-1","directory":"/tmp/test","timestamp":"2024-01-15T14:30:00Z"}`,
	})
	defer os.Remove(tmpFile)

	p := NewParser(tmpFile)
	entries, err := p.ReadAll()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	expectedTime := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)
	if !entries[0].Timestamp.Equal(expectedTime) {
		t.Errorf("expected timestamp %v, got %v", expectedTime, entries[0].Timestamp)
	}
}
