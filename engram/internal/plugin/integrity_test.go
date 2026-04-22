package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyIntegrity_NoIntegritySection(t *testing.T) {
	// Empty integrity section should pass (backwards compatibility)
	integrity := Integrity{}
	err := VerifyIntegrity("/tmp", integrity)
	if err != nil {
		t.Errorf("VerifyIntegrity() with empty integrity should pass, got error: %v", err)
	}
}

func TestVerifyIntegrity_UnsupportedAlgorithm(t *testing.T) {
	integrity := Integrity{
		Algorithm: "md5",
		Files: map[string]string{
			"test.txt": "abc123",
		},
	}

	err := VerifyIntegrity("/tmp", integrity)
	if err == nil {
		t.Error("VerifyIntegrity() should fail for unsupported algorithm")
	}
	if err.Error() != "unsupported hash algorithm: md5 (only sha256 is supported)" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestVerifyIntegrity_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	integrity := Integrity{
		Algorithm: "sha256",
		Files: map[string]string{
			"nonexistent.txt": "abc123",
		},
	}

	err := VerifyIntegrity(tmpDir, integrity)
	if err == nil {
		t.Error("VerifyIntegrity() should fail when file not found")
	}
}

func TestVerifyIntegrity_ValidHash(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("hello world")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Compute expected hash
	expectedHash, err := computeSHA256(testFile)
	if err != nil {
		t.Fatalf("Failed to compute hash: %v", err)
	}

	// Verify with correct hash
	integrity := Integrity{
		Algorithm: "sha256",
		Files: map[string]string{
			"test.txt": expectedHash,
		},
	}

	err = VerifyIntegrity(tmpDir, integrity)
	if err != nil {
		t.Errorf("VerifyIntegrity() should pass with valid hash, got error: %v", err)
	}
}

func TestVerifyIntegrity_ModifiedFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	originalContent := []byte("hello world")
	if err := os.WriteFile(testFile, originalContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Compute hash of original content
	originalHash, err := computeSHA256(testFile)
	if err != nil {
		t.Fatalf("Failed to compute hash: %v", err)
	}

	// Modify file
	modifiedContent := []byte("hello world modified")
	if err := os.WriteFile(testFile, modifiedContent, 0644); err != nil {
		t.Fatalf("Failed to modify test file: %v", err)
	}

	// Verify with original hash (should fail)
	integrity := Integrity{
		Algorithm: "sha256",
		Files: map[string]string{
			"test.txt": originalHash,
		},
	}

	err = VerifyIntegrity(tmpDir, integrity)
	if err == nil {
		t.Error("VerifyIntegrity() should fail when file has been modified")
	}
}

func TestVerifyIntegrity_MultipleFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple test files
	files := map[string]string{
		"file1.txt": "content 1",
		"file2.txt": "content 2",
		"file3.txt": "content 3",
	}

	hashes := make(map[string]string)
	for filename, content := range files {
		filePath := filepath.Join(tmpDir, filename)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", filename, err)
		}

		hash, err := computeSHA256(filePath)
		if err != nil {
			t.Fatalf("Failed to compute hash for %s: %v", filename, err)
		}
		hashes[filename] = hash
	}

	// Verify all files
	integrity := Integrity{
		Algorithm: "sha256",
		Files:     hashes,
	}

	err := VerifyIntegrity(tmpDir, integrity)
	if err != nil {
		t.Errorf("VerifyIntegrity() should pass with all valid hashes, got error: %v", err)
	}
}

func TestVerifyIntegrity_NestedDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested directory structure
	nestedDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("Failed to create nested directory: %v", err)
	}

	// Create file in nested directory
	testFile := filepath.Join(nestedDir, "tool")
	content := []byte("#!/bin/bash\necho 'hello'")
	if err := os.WriteFile(testFile, content, 0755); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Compute hash
	expectedHash, err := computeSHA256(testFile)
	if err != nil {
		t.Fatalf("Failed to compute hash: %v", err)
	}

	// Verify with relative path
	integrity := Integrity{
		Algorithm: "sha256",
		Files: map[string]string{
			"bin/tool": expectedHash,
		},
	}

	err = VerifyIntegrity(tmpDir, integrity)
	if err != nil {
		t.Errorf("VerifyIntegrity() should handle nested directories, got error: %v", err)
	}
}

func TestComputeSHA256(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file with known content
	testFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("hello world")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Compute hash
	hash, err := computeSHA256(testFile)
	if err != nil {
		t.Errorf("computeSHA256() failed: %v", err)
	}

	// Expected SHA-256 hash of "hello world"
	expectedHash := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	if hash != expectedHash {
		t.Errorf("computeSHA256() = %q, want %q", hash, expectedHash)
	}
}

func TestComputeSHA256_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create empty file
	testFile := filepath.Join(tmpDir, "empty.txt")
	if err := os.WriteFile(testFile, []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Compute hash
	hash, err := computeSHA256(testFile)
	if err != nil {
		t.Errorf("computeSHA256() failed: %v", err)
	}

	// Expected SHA-256 hash of empty file
	expectedHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if hash != expectedHash {
		t.Errorf("computeSHA256() for empty file = %q, want %q", hash, expectedHash)
	}
}

func TestComputeSHA256_FileNotFound(t *testing.T) {
	_, err := computeSHA256("/tmp/nonexistent-file-12345.txt")
	if err == nil {
		t.Error("computeSHA256() should fail when file doesn't exist")
	}
}

func TestGenerateIntegrity(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	files := []string{"file1.txt", "file2.txt"}
	for _, filename := range files {
		filePath := filepath.Join(tmpDir, filename)
		if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", filename, err)
		}
	}

	// Generate integrity
	integrity, err := GenerateIntegrity(tmpDir, files)
	if err != nil {
		t.Fatalf("GenerateIntegrity() failed: %v", err)
	}

	// Verify structure
	if integrity.Algorithm != "sha256" {
		t.Errorf("GenerateIntegrity() algorithm = %q, want %q", integrity.Algorithm, "sha256")
	}

	if len(integrity.Files) != 2 {
		t.Errorf("GenerateIntegrity() files count = %d, want 2", len(integrity.Files))
	}

	for _, filename := range files {
		if _, exists := integrity.Files[filename]; !exists {
			t.Errorf("GenerateIntegrity() missing hash for %q", filename)
		}
	}
}

func TestGenerateIntegrity_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	// Try to generate integrity for nonexistent file
	_, err := GenerateIntegrity(tmpDir, []string{"nonexistent.txt"})
	if err == nil {
		t.Error("GenerateIntegrity() should fail when file doesn't exist")
	}
}

func TestGenerateIntegrity_EmptyFileList(t *testing.T) {
	tmpDir := t.TempDir()

	// Generate integrity with empty file list
	integrity, err := GenerateIntegrity(tmpDir, []string{})
	if err != nil {
		t.Fatalf("GenerateIntegrity() with empty list failed: %v", err)
	}

	if len(integrity.Files) != 0 {
		t.Errorf("GenerateIntegrity() with empty list should have 0 files, got %d", len(integrity.Files))
	}
}
