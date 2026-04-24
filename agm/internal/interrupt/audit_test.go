package interrupt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLogInterrupt(t *testing.T) {
	// Use temp dir for test isolation
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	entry := &AuditEntry{
		Sender:    "orchestrator",
		Recipient: "test-session",
		FlagUsed:  "emergency-interrupt",
	}

	if err := LogInterrupt(entry); err != nil {
		t.Fatalf("LogInterrupt failed: %v", err)
	}

	// Verify file exists and contains valid JSONL
	logPath := filepath.Join(tmpDir, ".agm", "logs", "interrupt-audit.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log: %v", err)
	}

	var parsed AuditEntry
	if err := json.Unmarshal(data[:len(data)-1], &parsed); err != nil {
		t.Fatalf("Failed to parse JSONL: %v", err)
	}

	if parsed.Sender != "orchestrator" {
		t.Errorf("Expected sender 'orchestrator', got '%s'", parsed.Sender)
	}
	if parsed.InterruptNum != 1 {
		t.Errorf("Expected interrupt_num 1, got %d", parsed.InterruptNum)
	}
}

func TestGetInterruptCount(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Log 3 interrupts for same session
	for i := 0; i < 3; i++ {
		entry := &AuditEntry{
			Sender:    "orchestrator",
			Recipient: "counted-session",
			FlagUsed:  "emergency-interrupt",
		}
		if err := LogInterrupt(entry); err != nil {
			t.Fatalf("LogInterrupt %d failed: %v", i, err)
		}
	}

	// Log 1 interrupt for different session
	entry := &AuditEntry{
		Sender:    "orchestrator",
		Recipient: "other-session",
		FlagUsed:  "emergency-interrupt",
	}
	if err := LogInterrupt(entry); err != nil {
		t.Fatalf("LogInterrupt for other failed: %v", err)
	}

	count := GetInterruptCount("counted-session")
	if count != 3 {
		t.Errorf("Expected 3 interrupts for counted-session, got %d", count)
	}

	otherCount := GetInterruptCount("other-session")
	if otherCount != 1 {
		t.Errorf("Expected 1 interrupt for other-session, got %d", otherCount)
	}

	noCount := GetInterruptCount("nonexistent")
	if noCount != 0 {
		t.Errorf("Expected 0 interrupts for nonexistent, got %d", noCount)
	}
}
