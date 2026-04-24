package retrospective

import (
	"path/filepath"
	"testing"
)

// TestCaptureGitContext_NonGitDirectory tests error handling for non-git repos
func TestCaptureGitContext_NonGitDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// tmpDir is not a git repository
	_, err := captureGitContext(tmpDir)

	// Should return error for non-git directory
	if err == nil {
		t.Errorf("Expected error for non-git directory, got nil")
	}
}

// TestCaptureGitContext_InvalidPath tests error handling for invalid paths
func TestCaptureGitContext_InvalidPath(t *testing.T) {
	invalidPath := filepath.Join(t.TempDir(), "nonexistent", "path")

	_, err := captureGitContext(invalidPath)

	// Should return error for invalid path
	if err == nil {
		t.Errorf("Expected error for invalid path, got nil")
	}
}

// TestCaptureDeliverables_EmptyDirectory tests empty directory handling
func TestCaptureDeliverables_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	deliverables, err := captureDeliverables(tmpDir)

	// Should succeed with empty list
	if err != nil {
		t.Errorf("Expected no error for empty directory, got: %v", err)
	}

	if len(deliverables) != 0 {
		t.Errorf("Expected empty deliverables list, got %d items", len(deliverables))
	}
}

// TestCaptureDeliverables_InvalidPath tests handling of invalid paths
func TestCaptureDeliverables_InvalidPath(t *testing.T) {
	invalidPath := filepath.Join(t.TempDir(), "nonexistent")

	deliverables, err := captureDeliverables(invalidPath)

	// filepath.Glob handles non-existent paths gracefully (returns empty, no error)
	if err != nil {
		t.Errorf("Expected no error for non-existent path, got: %v", err)
	}

	// Should return empty list for non-existent directory
	if len(deliverables) != 0 {
		t.Errorf("Expected empty deliverables list, got: %v", deliverables)
	}
}
