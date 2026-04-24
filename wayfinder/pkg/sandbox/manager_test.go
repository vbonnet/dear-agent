package sandbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateSandbox(t *testing.T) {
	// Setup
	tempDir := t.TempDir()
	manager := NewManager(tempDir)

	// Test
	sandbox, err := manager.CreateSandbox("test-project")
	if err != nil {
		t.Fatalf("CreateSandbox failed: %v", err)
	}

	// Verify
	if sandbox.Name != "test-project" {
		t.Errorf("Expected name 'test-project', got '%s'", sandbox.Name)
	}

	if sandbox.ID == "" {
		t.Error("Sandbox ID should not be empty")
	}

	// Check directory exists
	sandboxPath := filepath.Join(tempDir, sandbox.ID)
	if _, err := os.Stat(sandboxPath); os.IsNotExist(err) {
		t.Error("Sandbox directory was not created")
	}

	// Check metadata file exists
	metadataPath := filepath.Join(sandboxPath, ".wayfinder-project")
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		t.Error("Metadata file was not created")
	}
}

func TestListSandboxes(t *testing.T) {
	// Setup
	tempDir := t.TempDir()
	manager := NewManager(tempDir)

	// Create multiple sandboxes
	_, err := manager.CreateSandbox("project-1")
	if err != nil {
		t.Fatalf("Failed to create project-1: %v", err)
	}

	_, err = manager.CreateSandbox("project-2")
	if err != nil {
		t.Fatalf("Failed to create project-2: %v", err)
	}

	// List sandboxes
	sandboxes, err := manager.ListSandboxes()
	if err != nil {
		t.Fatalf("ListSandboxes failed: %v", err)
	}

	// Verify
	if len(sandboxes) != 2 {
		t.Errorf("Expected 2 sandboxes, got %d", len(sandboxes))
	}
}

func TestCleanupSandbox(t *testing.T) {
	// Setup
	tempDir := t.TempDir()
	manager := NewManager(tempDir)

	// Create sandbox
	sandbox, err := manager.CreateSandbox("test-cleanup")
	if err != nil {
		t.Fatalf("CreateSandbox failed: %v", err)
	}

	// Verify sandbox exists
	sandboxPath := filepath.Join(tempDir, sandbox.ID)
	if _, err := os.Stat(sandboxPath); os.IsNotExist(err) {
		t.Fatal("Sandbox directory should exist before cleanup")
	}

	// Cleanup
	err = manager.CleanupSandbox(sandbox.Name)
	if err != nil {
		t.Fatalf("CleanupSandbox failed: %v", err)
	}

	// Verify sandbox removed
	if _, err := os.Stat(sandboxPath); !os.IsNotExist(err) {
		t.Error("Sandbox directory should be removed after cleanup")
	}
}

func TestPathResolver(t *testing.T) {
	// Setup
	tempDir := t.TempDir()
	manager := NewManager(tempDir)
	resolver := NewPathResolver(manager)

	// Test fallback (no active sandbox)
	path := resolver.GetSandboxPath("sessions")
	if !filepath.IsAbs(path) {
		t.Error("Path should be absolute")
	}

	// Should contain .wayfinder for fallback mode
	if !contains(path, ".wayfinder") {
		t.Error("Fallback path should contain .wayfinder")
	}
}

func contains(s, substr string) bool {
	return filepath.Base(filepath.Dir(s)) == substr || filepath.Base(filepath.Dir(filepath.Dir(s))) == substr
}
