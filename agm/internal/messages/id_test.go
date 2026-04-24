package messages

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestMessageIDGenerator_NoConcurrentDeadlock verifies that calling Next()
// concurrently does not cause a deadlock (regression test for 2026-03-17 fix)
func TestMessageIDGenerator_NoConcurrentDeadlock(t *testing.T) {
	// Create temp state directory
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "state")

	// Create generator
	gen, err := NewMessageIDGenerator("test-sender", stateDir)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	// Test with timeout to catch deadlocks
	done := make(chan bool, 1)
	timeout := time.After(5 * time.Second)

	go func() {
		// Generate 10 IDs concurrently
		var wg sync.WaitGroup
		errors := make(chan error, 10)

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := gen.Next()
				if err != nil {
					errors <- err
				}
			}()
		}

		wg.Wait()
		close(errors)

		// Check for errors
		for err := range errors {
			t.Errorf("Next() failed: %v", err)
		}

		done <- true
	}()

	select {
	case <-done:
		// Success - no deadlock
	case <-timeout:
		t.Fatal("Deadlock detected: Next() calls did not complete within 5 seconds")
	}
}

// TestMessageIDGenerator_SequenceIncrement verifies sequence counter increments
func TestMessageIDGenerator_SequenceIncrement(t *testing.T) {
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "state")

	gen, err := NewMessageIDGenerator("test", stateDir)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	// Generate 3 IDs
	id1, _ := gen.Next()
	id2, _ := gen.Next()
	id3, _ := gen.Next()

	// Verify all IDs are unique
	if id1 == id2 || id2 == id3 || id1 == id3 {
		t.Errorf("IDs should be unique: %s, %s, %s", id1, id2, id3)
	}

	// Verify format (timestamp-sender-seq)
	if !ValidateMessageID(id1) {
		t.Errorf("Invalid message ID format: %s", id1)
	}
}

// TestMessageIDGenerator_PersistSequence verifies sequence persists across instances
func TestMessageIDGenerator_PersistSequence(t *testing.T) {
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "state")

	// Create first generator and generate ID
	gen1, err := NewMessageIDGenerator("test", stateDir)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	id1, _ := gen1.Next()

	// Create second generator (should load saved sequence)
	gen2, err := NewMessageIDGenerator("test", stateDir)
	if err != nil {
		t.Fatalf("Failed to create second generator: %v", err)
	}

	id2, _ := gen2.Next()

	// Verify IDs are different (sequence continued)
	if id1 == id2 {
		t.Errorf("Second generator should continue sequence, got same ID: %s", id1)
	}
}

// TestMessageIDGenerator_StateFileCreation verifies state directory is created
func TestMessageIDGenerator_StateFileCreation(t *testing.T) {
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "nonexistent", "state")

	// Should create directory if it doesn't exist
	gen, err := NewMessageIDGenerator("test", stateDir)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		t.Errorf("State directory was not created: %s", stateDir)
	}

	// Generate ID to create state file
	_, err = gen.Next()
	if err != nil {
		t.Fatalf("Failed to generate ID: %v", err)
	}

	// Verify state file was created
	stateFile := filepath.Join(stateDir, "msg_seq_test.json")
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		t.Errorf("State file was not created: %s", stateFile)
	}
}

// TestValidateMessageID tests message ID validation
func TestValidateMessageID(t *testing.T) {
	tests := []struct {
		id    string
		valid bool
	}{
		{"1738612345678-sender-001", true},
		{"1738612345678-test-042", true},
		{"1738612345678-agm-send-999", true},
		{"notanumber-sender-001", true}, // passes basic format check (has 2 dashes, min length)
		{"invalid", false},
		{"", false},
		{"123-x", false},            // too short
		{"123456789-sender", false}, // only 1 dash (missing sequence)
		{"x", false},                // way too short
	}

	for _, tt := range tests {
		result := ValidateMessageID(tt.id)
		if result != tt.valid {
			t.Errorf("ValidateMessageID(%q) = %v, want %v", tt.id, result, tt.valid)
		}
	}
}
