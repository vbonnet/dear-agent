package fileutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testData := []byte("test data")

	// Write file
	if err := AtomicWrite(testFile, testData, 0600); err != nil {
		t.Fatalf("AtomicWrite failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Error("File was not created")
	}

	// Verify content
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(data) != string(testData) {
		t.Errorf("Wrong content: got %q, want %q", data, testData)
	}

	// Verify permissions
	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("Wrong permissions: got %o, want 0600", info.Mode().Perm())
	}
}

func TestAtomicWrite_Overwrite(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// Write initial content
	if err := AtomicWrite(testFile, []byte("original"), 0600); err != nil {
		t.Fatalf("First write failed: %v", err)
	}

	// Overwrite with new content
	newData := []byte("overwritten")
	if err := AtomicWrite(testFile, newData, 0600); err != nil {
		t.Fatalf("Second write failed: %v", err)
	}

	// Verify new content
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(data) != string(newData) {
		t.Errorf("Wrong content after overwrite: got %q, want %q", data, newData)
	}
}

func TestAtomicWrite_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "subdir", "nested", "test.txt")
	testData := []byte("test")

	// Directory doesn't exist yet
	if _, err := os.Stat(filepath.Dir(testFile)); !os.IsNotExist(err) {
		t.Error("Directory should not exist yet")
	}

	// Write file (should create directory)
	if err := AtomicWrite(testFile, testData, 0600); err != nil {
		t.Fatalf("AtomicWrite failed: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(filepath.Dir(testFile)); os.IsNotExist(err) {
		t.Error("Directory was not created")
	}

	// Verify file was created
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(data) != string(testData) {
		t.Errorf("Wrong content: got %q, want %q", data, testData)
	}
}

func TestAtomicWrite_NoTempFilesLeft(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// Write file
	if err := AtomicWrite(testFile, []byte("test"), 0600); err != nil {
		t.Fatalf("AtomicWrite failed: %v", err)
	}

	// Check for leftover temp files
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	for _, entry := range entries {
		if entry.Name() != "test.txt" {
			t.Errorf("Unexpected file in directory: %s", entry.Name())
		}
	}
}

func TestAtomicWrite_DifferentPermissions(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		perm os.FileMode
	}{
		{0600},
		{0644},
		{0400}, // read-only
	}

	for _, tt := range tests {
		testFile := filepath.Join(tmpDir, "test-"+string(rune(tt.perm))+".txt")
		if err := AtomicWrite(testFile, []byte("test"), tt.perm); err != nil {
			t.Fatalf("AtomicWrite with perm %o failed: %v", tt.perm, err)
		}

		info, err := os.Stat(testFile)
		if err != nil {
			t.Fatalf("Failed to stat file: %v", err)
		}

		if info.Mode().Perm() != tt.perm {
			t.Errorf("Wrong permissions: got %o, want %o", info.Mode().Perm(), tt.perm)
		}
	}
}
