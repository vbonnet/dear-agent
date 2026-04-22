package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckExitGate_NoMarker(t *testing.T) {
	// Use a session name that won't have a marker file
	err := CheckExitGate("nonexistent-test-session-xyz")
	if err == nil {
		t.Fatal("expected error when no exit marker exists")
	}
}

func TestWriteExitMarker_AndCheckExitGate(t *testing.T) {
	sessionName := "test-exit-gate-session"

	// Clean up after test
	homeDir, _ := os.UserHomeDir()
	markerPath := filepath.Join(homeDir, ".agm", "exit-markers", sessionName+".exit")
	t.Cleanup(func() { os.Remove(markerPath) })

	// Before writing, gate should fail
	err := CheckExitGate(sessionName)
	if err == nil {
		t.Fatal("expected error before exit marker is written")
	}

	// Write the marker
	if err := WriteExitMarker(sessionName); err != nil {
		t.Fatalf("WriteExitMarker failed: %v", err)
	}

	// After writing, gate should pass
	if err := CheckExitGate(sessionName); err != nil {
		t.Fatalf("CheckExitGate failed after writing marker: %v", err)
	}

	// Verify marker file contents
	data, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("failed to read marker file: %v", err)
	}
	if string(data) != "exited\n" {
		t.Errorf("unexpected marker content: %q", string(data))
	}
}
