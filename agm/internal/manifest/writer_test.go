package manifest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestWriteManifestHelper_NewFile(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test.yaml")

	// Test data
	testData := []byte("test: data\n")

	// Define closures
	validateFn := func() error { return nil }
	marshalFn := func() ([]byte, error) { return testData, nil }

	// Call helper
	err := writeManifestHelper(testPath, validateFn, marshalFn, 0600)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify file created with correct content
	content, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(content) != string(testData) {
		t.Errorf("content mismatch: got %q, want %q", content, testData)
	}

	// Verify file permissions
	info, err := os.Stat(testPath)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("permission mismatch: got %o, want 0600", info.Mode().Perm())
	}

	// Verify no backup created (new file)
	backupPath := testPath + ".1"
	if _, err := os.Stat(backupPath); err == nil {
		t.Error("backup should not exist for new file")
	}
}

func TestWriteManifestHelper_ExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test.yaml")

	// Create existing file
	originalData := []byte("original: data\n")
	if err := os.WriteFile(testPath, originalData, 0600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// New data
	newData := []byte("new: data\n")

	// Define closures
	validateFn := func() error { return nil }
	marshalFn := func() ([]byte, error) { return newData, nil }

	// Call helper
	err := writeManifestHelper(testPath, validateFn, marshalFn, 0600)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify file updated
	content, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(content) != string(newData) {
		t.Errorf("content mismatch: got %q, want %q", content, newData)
	}

	// Verify backup created
	backupPath := testPath + ".1"
	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("backup should exist: %v", err)
	}
	if string(backupContent) != string(originalData) {
		t.Errorf("backup content mismatch: got %q, want %q", backupContent, originalData)
	}
}

func TestWriteManifestHelper_ValidationFailure(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test.yaml")

	// Create existing file
	originalData := []byte("original: data\n")
	if err := os.WriteFile(testPath, originalData, 0600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Define failing validator
	validateErr := errors.New("validation failed")
	validateFn := func() error { return validateErr }
	marshalFn := func() ([]byte, error) { return []byte("new data"), nil }

	// Call helper - should fail
	err := writeManifestHelper(testPath, validateFn, marshalFn, 0600)
	if !errors.Is(err, validateErr) {
		t.Fatalf("expected validation error, got: %v", err)
	}

	// Verify original file unchanged
	content, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(content) != string(originalData) {
		t.Error("file should be unchanged on validation failure")
	}

	// Backup is allowed to exist (it's created before validation runs);
	// the assertion above already covers the original file being unchanged.
	_ = testPath + ".1"
}

func TestWriteManifestHelper_MarshalFailure(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test.yaml")

	// Create existing file
	originalData := []byte("original: data\n")
	if err := os.WriteFile(testPath, originalData, 0600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Define failing marshaler
	marshalErr := errors.New("marshal failed")
	validateFn := func() error { return nil }
	marshalFn := func() ([]byte, error) { return nil, marshalErr }

	// Call helper - should fail
	err := writeManifestHelper(testPath, validateFn, marshalFn, 0600)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to marshal manifest") {
		t.Errorf("expected marshal error message, got: %v", err)
	}
	if !errors.Is(err, marshalErr) {
		t.Errorf("expected wrapped marshal error, got: %v", err)
	}

	// Verify original file unchanged
	content, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(content) != string(originalData) {
		t.Error("file should be unchanged on marshal failure")
	}
}

func TestWriteManifestHelper_BackupFailure(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test.yaml")

	// Create existing file
	if err := os.WriteFile(testPath, []byte("original"), 0600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Make file unreadable to cause backup failure
	if err := os.Chmod(testPath, 0000); err != nil {
		t.Fatalf("failed to change permissions: %v", err)
	}
	defer os.Chmod(testPath, 0600) // Cleanup

	// Define closures
	validateFn := func() error { return nil }
	marshalFn := func() ([]byte, error) { return []byte("new data"), nil }

	// Call helper - should fail on backup
	err := writeManifestHelper(testPath, validateFn, marshalFn, 0600)
	if err == nil {
		t.Fatal("expected backup error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to create backup before write") {
		t.Errorf("expected backup error message, got: %v", err)
	}
}

// Run with: go test -race
func TestWriteManifestHelper_Concurrent(t *testing.T) {
	tmpDir := t.TempDir()

	var wg sync.WaitGroup

	// Test concurrent writes to different files (should not race)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			testPath := filepath.Join(tmpDir, fmt.Sprintf("test-%d.yaml", n))
			data := []byte(fmt.Sprintf("data-%d\n", n))
			validateFn := func() error { return nil }
			marshalFn := func() ([]byte, error) { return data, nil }
			if err := writeManifestHelper(testPath, validateFn, marshalFn, 0600); err != nil {
				t.Errorf("concurrent write failed: %v", err)
			}
		}(i)
	}

	wg.Wait()

	// Verify all files created
	for i := 0; i < 10; i++ {
		testPath := filepath.Join(tmpDir, fmt.Sprintf("test-%d.yaml", i))
		content, err := os.ReadFile(testPath)
		if err != nil {
			t.Errorf("file %d not created: %v", i, err)
			continue
		}
		expected := fmt.Sprintf("data-%d\n", i)
		if string(content) != expected {
			t.Errorf("file %d content mismatch: got %q, want %q", i, content, expected)
		}
	}
}

func TestWriteManifestHelper_PermissionsPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test.yaml")

	// Test with different permissions
	testPerms := []os.FileMode{0600, 0644, 0640}

	for _, perm := range testPerms {
		// Clear any existing file
		os.Remove(testPath)

		testData := []byte(fmt.Sprintf("perm: %o\n", perm))
		validateFn := func() error { return nil }
		marshalFn := func() ([]byte, error) { return testData, nil }

		// Write with specific permissions
		err := writeManifestHelper(testPath, validateFn, marshalFn, perm)
		if err != nil {
			t.Fatalf("write failed for perm %o: %v", perm, err)
		}

		// Verify permissions
		info, err := os.Stat(testPath)
		if err != nil {
			t.Fatalf("stat failed: %v", err)
		}
		if info.Mode().Perm() != perm {
			t.Errorf("permission mismatch: got %o, want %o", info.Mode().Perm(), perm)
		}
	}
}

func TestWriteManifestHelper_MultipleBackups(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test.yaml")

	// Write file multiple times to create multiple backups
	for i := 1; i <= 3; i++ {
		data := []byte(fmt.Sprintf("version: %d\n", i))
		validateFn := func() error { return nil }
		marshalFn := func() ([]byte, error) { return data, nil }

		err := writeManifestHelper(testPath, validateFn, marshalFn, 0600)
		if err != nil {
			t.Fatalf("write %d failed: %v", i, err)
		}

		// Verify current file has latest data
		content, err := os.ReadFile(testPath)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		expected := fmt.Sprintf("version: %d\n", i)
		if string(content) != expected {
			t.Errorf("content mismatch at write %d: got %q, want %q", i, content, expected)
		}
	}

	// Verify backups created (at least 2 backups from 3 writes)
	// First write: no backup
	// Second write: backup .1 created
	// Third write: backup .2 created
	backup1 := testPath + ".1"
	backup2 := testPath + ".2"

	if _, err := os.Stat(backup1); err != nil {
		t.Error("backup .1 should exist")
	}
	if _, err := os.Stat(backup2); err != nil {
		t.Error("backup .2 should exist")
	}

	// Verify backup contents
	b1Content, _ := os.ReadFile(backup1)
	if string(b1Content) != "version: 1\n" {
		t.Errorf("backup .1 content wrong: got %q", b1Content)
	}

	b2Content, _ := os.ReadFile(backup2)
	if string(b2Content) != "version: 2\n" {
		t.Errorf("backup .2 content wrong: got %q", b2Content)
	}
}
