package session

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestExtractUUID_ValidUUID(t *testing.T) {
	// Create temp history file with valid UUID
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "history.jsonl")

	validUUID := "fd8b9d80-f061-48d4-b2e0-c56421fe0b17"
	content := `{"sessionId":"` + validUUID + `","timestamp":"2026-02-02T21:00:00Z"}`

	if err := os.WriteFile(historyPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	uuid, err := ExtractUUID(historyPath)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if uuid != validUUID {
		t.Errorf("UUID = %s, want %s", uuid, validUUID)
	}
}

func TestExtractUUID_MissingFile(t *testing.T) {
	uuid, err := ExtractUUID("/nonexistent/history.jsonl")

	if err == nil {
		t.Error("Expected error for missing file")
	}

	// Should return fallback UUID
	if !isFallbackUUID(uuid) {
		t.Errorf("Expected fallback UUID, got: %s", uuid)
	}

	if uuid == "" {
		t.Error("Expected non-empty fallback UUID")
	}
}

func TestExtractUUID_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "history.jsonl")

	if err := os.WriteFile(historyPath, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	uuid, err := ExtractUUID(historyPath)

	if err == nil {
		t.Error("Expected error for empty file")
	}

	// Should return fallback UUID
	if !isFallbackUUID(uuid) {
		t.Errorf("Expected fallback UUID, got: %s", uuid)
	}
}

func TestExtractUUID_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "history.jsonl")

	content := `{invalid json}`
	if err := os.WriteFile(historyPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	uuid, err := ExtractUUID(historyPath)

	if err == nil {
		t.Error("Expected error for invalid JSON")
	}

	// Should return fallback UUID
	if !isFallbackUUID(uuid) {
		t.Errorf("Expected fallback UUID, got: %s", uuid)
	}
}

func TestExtractUUID_InvalidUUIDFormat(t *testing.T) {
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "history.jsonl")

	content := `{"sessionId":"not-a-valid-uuid","timestamp":"2026-02-02T21:00:00Z"}`
	if err := os.WriteFile(historyPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	uuid, err := ExtractUUID(historyPath)

	if err == nil {
		t.Error("Expected error for invalid UUID format")
	}

	// Should return fallback UUID
	if !isFallbackUUID(uuid) {
		t.Errorf("Expected fallback UUID, got: %s", uuid)
	}
}

func TestExtractUUID_MultipleLines(t *testing.T) {
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "history.jsonl")

	validUUID := "12345678-1234-1234-1234-123456789abc"
	content := `{"sessionId":"old-uuid-1","timestamp":"2026-02-01T10:00:00Z"}
{"sessionId":"old-uuid-2","timestamp":"2026-02-01T15:00:00Z"}
{"sessionId":"` + validUUID + `","timestamp":"2026-02-02T21:00:00Z"}`

	if err := os.WriteFile(historyPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	uuid, err := ExtractUUID(historyPath)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Should extract UUID from LAST line
	if uuid != validUUID {
		t.Errorf("UUID = %s, want %s", uuid, validUUID)
	}
}

func TestGenerateFallbackUUID_Format(t *testing.T) {
	uuid := generateFallbackUUID("test")

	// Should match auto-<timestamp>-<hex> format
	pattern := regexp.MustCompile(`^auto-\d{10}-[0-9a-f]{4}$`)
	if !pattern.MatchString(uuid) {
		t.Errorf("Fallback UUID format invalid: %s", uuid)
	}
}

func TestGenerateFallbackUUID_Uniqueness(t *testing.T) {
	// Generate multiple fallback UUIDs, verify they're different
	uuid1 := generateFallbackUUID("test1")
	uuid2 := generateFallbackUUID("test2")

	if uuid1 == uuid2 {
		t.Error("Fallback UUIDs should be unique")
	}
}

// Helper function to check if UUID matches fallback format
func isFallbackUUID(uuid string) bool {
	pattern := regexp.MustCompile(`^auto-\d{10}-[0-9a-f]{4}$`)
	return pattern.MatchString(uuid)
}
