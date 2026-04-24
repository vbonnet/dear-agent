package validator

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDetectLanguage tests language detection from file extensions
func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		name      string
		files     []string
		wantLang  string
		wantError bool
	}{
		{
			name:      "Go project",
			files:     []string{"/tmp/main.go", "/tmp/utils.go"},
			wantLang:  "go",
			wantError: false,
		},
		{
			name:      "Python project",
			files:     []string{"/tmp/main.py", "/tmp/utils.py"},
			wantLang:  "python",
			wantError: false,
		},
		{
			name:      "JavaScript project",
			files:     []string{"/tmp/index.js", "/tmp/utils.js"},
			wantLang:  "javascript",
			wantError: false,
		},
		{
			name:      "TypeScript project",
			files:     []string{"/tmp/index.ts", "/tmp/utils.ts"},
			wantLang:  "javascript",
			wantError: false,
		},
		{
			name:      "Rust project",
			files:     []string{"/tmp/main.rs", "/tmp/lib.rs"},
			wantLang:  "rust",
			wantError: false,
		},
		{
			name:      "Mixed Go and Python (Go majority)",
			files:     []string{"/tmp/main.go", "/tmp/utils.go", "/tmp/test.py"},
			wantLang:  "go",
			wantError: false,
		},
		{
			name:      "No code files",
			files:     []string{"/tmp/README.md", "/tmp/LICENSE"},
			wantLang:  "",
			wantError: true,
		},
		{
			name:      "Empty file list",
			files:     []string{},
			wantLang:  "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := detectLanguage(tt.files)
			if tt.wantError {
				if err == nil {
					t.Errorf("detectLanguage() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("detectLanguage() unexpected error: %v", err)
				return
			}
			if got != tt.wantLang {
				t.Errorf("detectLanguage() = %q, want %q", got, tt.wantLang)
			}
		})
	}
}

// TestValidatePath tests path traversal protection
func TestValidatePath(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantError bool
		errMsg    string
	}{
		{
			name:      "Valid absolute path",
			path:      "/tmp/test/project/file.go",
			wantError: false,
		},
		{
			name:      "Valid relative path",
			path:      "src/main.go",
			wantError: false,
		},
		{
			name:      "Path traversal with ../",
			path:      "../../../etc/passwd",
			wantError: true,
			errMsg:    "path traversal detected",
		},
		{
			name:      "Path with .. in middle",
			path:      "/tmp/test/../root/file",
			wantError: true,
			errMsg:    "path traversal detected",
		},
		{
			name:      "Valid path with dots in filename",
			path:      "/tmp/test/file.test.go",
			wantError: false,
		},
		{
			name:      "Empty path",
			path:      "",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePath("/tmp/project", tt.path)
			if tt.wantError {
				if err == nil {
					t.Errorf("validatePath() expected error, got nil")
					return
				}
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("validatePath() error = %q, want substring %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validatePath() unexpected error: %v", err)
				}
			}
		})
	}
}

// TestValidateFilesExist tests file existence validation
func TestValidateFilesExist(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	if err := os.WriteFile(filepath.Join(tmpDir, "small.go"), []byte("package main"), 0600); err != nil {
		t.Fatalf("Failed to create small file: %v", err)
	}

	// Create large file (> 10MB limit)
	largeContent := make([]byte, 11*1024*1024) // 11 MB
	if err := os.WriteFile(filepath.Join(tmpDir, "large.go"), largeContent, 0600); err != nil {
		t.Fatalf("Failed to create large file: %v", err)
	}

	tests := []struct {
		name      string
		files     []string
		wantError bool
		errMsg    string
	}{
		{
			name:      "All files exist",
			files:     []string{"small.go"}, // Relative path
			wantError: false,
		},
		{
			name:      "File doesn't exist",
			files:     []string{"missing.go"},
			wantError: true,
			errMsg:    "missing.go",
		},
		{
			name:      "File too large",
			files:     []string{"large.go"},
			wantError: true,
			errMsg:    "file too large",
		},
		{
			name:      "Empty file list",
			files:     []string{},
			wantError: false,
		},
		{
			name:      "Path traversal attempt",
			files:     []string{"../../../etc/passwd"},
			wantError: true,
			errMsg:    "path traversal detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFilesExist(tmpDir, tt.files)
			if tt.wantError {
				if err == nil {
					t.Errorf("validateFilesExist() expected error, got nil")
					return
				}
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("validateFilesExist() error = %q, want substring %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateFilesExist() unexpected error: %v", err)
				}
			}
		})
	}
}

// TestFindCodeFiles tests code file discovery
func TestFindCodeFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory structure with code files
	codeFiles := []string{
		"main.go",
		"src/utils.go",
		"pkg/helper.py",
		"tests/test_main.py",
	}

	nonCodeFiles := []string{
		"README.md",
		"LICENSE",
		"docs/guide.md",
	}

	for _, file := range append(codeFiles, nonCodeFiles...) {
		path := filepath.Join(tmpDir, file)
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		if err := os.WriteFile(path, []byte("content"), 0600); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}

	found, err := findCodeFiles(tmpDir)
	if err != nil {
		t.Fatalf("findCodeFiles() unexpected error: %v", err)
	}

	// Should find all code files, not documentation files
	wantCount := len(codeFiles)
	if len(found) != wantCount {
		t.Errorf("findCodeFiles() found %d files, want %d", len(found), wantCount)
	}

	// Verify each code file was found
	foundMap := make(map[string]bool)
	for _, f := range found {
		rel, _ := filepath.Rel(tmpDir, f)
		foundMap[rel] = true
	}

	for _, file := range codeFiles {
		if !foundMap[file] {
			t.Errorf("findCodeFiles() missing expected file: %s", file)
		}
	}

	// Verify non-code files were not found
	for _, file := range nonCodeFiles {
		if foundMap[file] {
			t.Errorf("findCodeFiles() incorrectly included non-code file: %s", file)
		}
	}
}

// TestCalculateFilesHash tests SHA-256 hash calculation
func TestCalculateFilesHash(t *testing.T) {
	tmpDir := t.TempDir()

	file1 := filepath.Join(tmpDir, "file1.go")
	file2 := filepath.Join(tmpDir, "file2.go")

	if err := os.WriteFile(file1, []byte("content1"), 0600); err != nil {
		t.Fatalf("Failed to create file1: %v", err)
	}
	if err := os.WriteFile(file2, []byte("content2"), 0600); err != nil {
		t.Fatalf("Failed to create file2: %v", err)
	}

	tests := []struct {
		name      string
		files     []string
		wantError bool
	}{
		{
			name:      "Single file",
			files:     []string{file1},
			wantError: false,
		},
		{
			name:      "Multiple files",
			files:     []string{file1, file2},
			wantError: false,
		},
		{
			name:      "Empty file list",
			files:     []string{},
			wantError: false,
		},
		{
			name:      "Nonexistent file",
			files:     []string{filepath.Join(tmpDir, "missing.go")},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := calculateFilesHash(tt.files)
			if tt.wantError {
				if err == nil {
					t.Errorf("calculateFilesHash() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("calculateFilesHash() unexpected error: %v", err)
				return
			}
			if hash == "" {
				t.Errorf("calculateFilesHash() returned empty hash")
			}
			// Verify hash format (SHA-256 is 64 hex characters)
			if len(hash) != 64 {
				t.Errorf("calculateFilesHash() hash length = %d, want 64", len(hash))
			}
		})
	}

	// Test hash consistency (same files = same hash)
	hash1, _ := calculateFilesHash([]string{file1, file2})
	hash2, _ := calculateFilesHash([]string{file1, file2})
	if hash1 != hash2 {
		t.Errorf("calculateFilesHash() not consistent: %s != %s", hash1, hash2)
	}

	// Test hash changes when content changes
	if err := os.WriteFile(file1, []byte("modified content"), 0600); err != nil {
		t.Fatalf("Failed to modify file: %v", err)
	}
	hash3, _ := calculateFilesHash([]string{file1, file2})
	if hash1 == hash3 {
		t.Errorf("calculateFilesHash() did not change after content modification")
	}
}

// TestTestHygieneRemediation tests the remediation message generation
func TestTestHygieneRemediation(t *testing.T) {
	msg := testHygieneRemediation("go test ./...")

	// Verify message contains key elements
	expected := []string{
		"Fix code bugs",
		"Fix test bugs",
		"Rewrite tests",
		"Delete obsolete tests",
		"zero tolerance",
		"go test ./...",
	}

	for _, exp := range expected {
		if !contains(msg, exp) {
			t.Errorf("testHygieneRemediation() missing expected text: %q", exp)
		}
	}
}

// TestValidateCodeDeliverables_GracefulDegradation tests graceful handling of edge cases
func TestValidateCodeDeliverables_GracefulDegradation(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(t *testing.T) string // Returns project directory
		phase     string
		wantError bool
		errMsg    string
	}{
		{
			name: "No code files (documentation project)",
			setupFunc: func(t *testing.T) string {
				dir := t.TempDir()
				// Create only documentation files
				os.WriteFile(filepath.Join(dir, "README.md"), []byte("docs"), 0600)
				os.WriteFile(filepath.Join(dir, "SPEC.md"), []byte("spec"), 0600)
				return dir
			},
			phase:     "S8",
			wantError: false, // Should skip gracefully
		},
		{
			name: "Unsupported language",
			setupFunc: func(t *testing.T) string {
				dir := t.TempDir()
				// Create files in unsupported language
				os.WriteFile(filepath.Join(dir, "script.sh"), []byte("#!/bin/bash"), 0600)
				return dir
			},
			phase:     "S8",
			wantError: false, // Should skip gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectDir := tt.setupFunc(t)
			err := validateCodeDeliverables(tt.phase, projectDir)

			if tt.wantError {
				if err == nil {
					t.Errorf("validateCodeDeliverables() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("validateCodeDeliverables() error = %q, want substring %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateCodeDeliverables() unexpected error: %v", err)
				}
			}
		})
	}
}
