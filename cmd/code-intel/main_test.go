package main

import (
	"os"
	"testing"
)

func TestDetectLanguages_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// An empty directory should return without error; no languages detected.
	if err := detectLanguages(dir); err != nil {
		t.Errorf("detectLanguages(empty dir): %v", err)
	}
}

func TestDetectLanguages_NonExistentDir(t *testing.T) {
	t.Parallel()
	// codeintel.NewRegistry handles missing dirs gracefully (no languages detected).
	if err := detectLanguages("/nonexistent-path-that-does-not-exist-XYZ"); err != nil {
		t.Errorf("detectLanguages(nonexistent): unexpected error: %v", err)
	}
}

func TestDetectLanguages_GoProject(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Create a minimal go.mod to trigger Go language detection.
	if err := os.WriteFile(dir+"/go.mod", []byte("module example.com/test\n\ngo 1.21\n"), 0o600); err != nil {
		t.Fatalf("WriteFile go.mod: %v", err)
	}
	if err := detectLanguages(dir); err != nil {
		t.Errorf("detectLanguages(go project): %v", err)
	}
}
