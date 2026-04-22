package beads

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	client := NewClient()
	if client == nil {
		t.Error("Expected non-nil client")
	}
}

func TestIsAvailable(t *testing.T) {
	client := NewClient()
	// This test depends on bd being installed
	// Just verify it doesn't crash
	_ = client.IsAvailable()
}

func TestGetBeadByUUID_NoBdCLI(t *testing.T) {
	client := &Client{bdPath: ""}

	_, err := client.GetBeadByUUID("test-uuid")
	if err == nil {
		t.Error("Expected error when bd CLI not available")
	}
}

func TestGetBeadTitle_NoBdCLI(t *testing.T) {
	client := &Client{bdPath: "/nonexistent/bd"}

	_, err := client.GetBeadTitle("test-id")
	if err == nil {
		t.Error("Expected error when bd CLI fails")
	}
}
