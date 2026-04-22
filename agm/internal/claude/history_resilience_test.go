package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseHistory_NullBytes tests that ParseHistory handles null bytes gracefully
func TestParseHistory_NullBytes(t *testing.T) {
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "history.jsonl")

	// Create history file with null byte in a line
	content := strings.Join([]string{
		`{"sessionId":"abc-123","project":"~/src","timestamp":1708387200000}`,
		`{"sessionId":"def\x00-456","project":"~/src","timestamp":1708387300000}`, // Null byte in sessionId
		`{"sessionId":"ghi-789","project":"~/src","timestamp":1708387400000}`,
	}, "\n")

	if err := os.WriteFile(historyPath, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test history file: %v", err)
	}

	// Parse history - should succeed and skip malformed line
	entries, stats, err := ParseHistory(historyPath)

	// Should NOT return error
	if err != nil {
		t.Errorf("ParseHistory returned error for file with null bytes: %v", err)
	}

	// Should have parsed 2 valid entries (skipped the one with null byte)
	if len(entries) != 2 {
		t.Errorf("Expected 2 valid entries, got %d", len(entries))
	}

	// Stats should show 1 skipped error
	if stats.SkippedErrors != 1 {
		t.Errorf("Expected 1 skipped error, got %d", stats.SkippedErrors)
	}

	// Verify the valid entries were parsed correctly
	if len(entries) >= 2 {
		if entries[0].SessionID != "abc-123" {
			t.Errorf("First entry sessionId = %q, want %q", entries[0].SessionID, "abc-123")
		}
		if entries[1].SessionID != "ghi-789" {
			t.Errorf("Second entry sessionId = %q, want %q", entries[1].SessionID, "ghi-789")
		}
	}
}

// TestParseHistory_MultipleCorruptedLines tests handling of multiple corrupted lines
func TestParseHistory_MultipleCorruptedLines(t *testing.T) {
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "history.jsonl")

	// Create history with multiple types of corruption
	content := strings.Join([]string{
		`{"sessionId":"valid-1","project":"~/src","timestamp":1708387200000}`,
		`{invalid json}`, // Invalid JSON
		`{"sessionId":"valid-2","project":"~/src","timestamp":1708387300000}`,
		``, // Empty line
		`{"sessionId":"valid-3","project":"~/src","timestamp":1708387400000}`,
		`{"sessionId":"test\x00\x00\x00","project":"/tmp","timestamp":1708387500000}`, // Multiple null bytes
		`{"sessionId":"valid-4","project":"~/src","timestamp":1708387600000}`,
	}, "\n")

	if err := os.WriteFile(historyPath, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test history file: %v", err)
	}

	// Parse history
	entries, stats, err := ParseHistory(historyPath)

	// Should NOT fail
	if err != nil {
		t.Fatalf("ParseHistory failed: %v", err)
	}

	// Should have 4 valid entries
	if len(entries) != 4 {
		t.Errorf("Expected 4 valid entries, got %d", len(entries))
	}

	// Should have skipped 2 errors (invalid JSON + null bytes) and 1 empty line
	if stats.SkippedErrors != 2 {
		t.Errorf("Expected 2 skipped errors, got %d", stats.SkippedErrors)
	}
	if stats.SkippedEmpty != 1 {
		t.Errorf("Expected 1 skipped empty line, got %d", stats.SkippedEmpty)
	}

	// Verify total lines processed
	expectedTotal := 7
	if stats.TotalLines != expectedTotal {
		t.Errorf("Expected %d total lines, got %d", expectedTotal, stats.TotalLines)
	}
}

// TestParseHistory_EntirelyCorrupted tests that entirely corrupted file returns empty list, not error
func TestParseHistory_EntirelyCorrupted(t *testing.T) {
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "history.jsonl")

	// Create history file with only corrupted entries
	content := strings.Join([]string{
		`{invalid}`,
		`not json at all`,
		`{"missing":"sessionId"}`,
		`\x00\x00\x00\x00`,
	}, "\n")

	if err := os.WriteFile(historyPath, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test history file: %v", err)
	}

	// Parse history
	entries, stats, err := ParseHistory(historyPath)

	// Should NOT fail - just return empty list
	if err != nil {
		t.Errorf("ParseHistory should not fail on entirely corrupted file, got error: %v", err)
	}

	// Should have no valid entries
	if len(entries) != 0 {
		t.Errorf("Expected 0 valid entries from corrupted file, got %d", len(entries))
	}

	// Should have tracked all lines as errors or skipped
	if stats.TotalLines != 4 {
		t.Errorf("Expected 4 total lines, got %d", stats.TotalLines)
	}
}

// TestParseHistory_LargeFileWithCorruption tests handling of large files with scattered corruption
func TestParseHistory_LargeFileWithCorruption(t *testing.T) {
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "history.jsonl")

	var lines []string
	validCount := 0

	// Create 1000 lines with corruption every 100th line
	for i := 0; i < 1000; i++ {
		if i%100 == 0 {
			// Corrupted line with null byte
			lines = append(lines, `{"sessionId":"bad\x00","project":"/tmp","timestamp":1708387200000}`)
		} else {
			// Valid line
			lines = append(lines, `{"sessionId":"valid-`+string(rune('0'+i%10))+`","project":"~/src","timestamp":1708387200000}`)
			validCount++
		}
	}

	content := strings.Join(lines, "\n")
	if err := os.WriteFile(historyPath, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test history file: %v", err)
	}

	// Parse history
	entries, stats, err := ParseHistory(historyPath)

	// Should NOT fail
	if err != nil {
		t.Fatalf("ParseHistory failed on large file: %v", err)
	}

	// Should have parsed all valid entries
	if len(entries) != validCount {
		t.Errorf("Expected %d valid entries, got %d", validCount, len(entries))
	}

	// Should have skipped 10 corrupted lines
	if stats.SkippedErrors != 10 {
		t.Errorf("Expected 10 skipped errors, got %d", stats.SkippedErrors)
	}
}
