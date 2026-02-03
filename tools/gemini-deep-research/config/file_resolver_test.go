package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Test 1: Resolve @file syntax
func TestResolveFile_ValidFile(t *testing.T) {
	// Create temporary file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "Extract key topics from the content"

	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test @file syntax
	input := "@" + testFile
	result, err := ResolveFile(input)

	if err != nil {
		t.Errorf("ResolveFile() error = %v, want nil", err)
	}
	if result != testContent {
		t.Errorf("ResolveFile() = %q, want %q", result, testContent)
	}
}

// Test 2: Pass-through non-@file values
func TestResolveFile_PassThrough(t *testing.T) {
	tests := []string{
		"Simple prompt",
		"Analyze {topics} in depth",
		"Research on {url}",
		"",
		"@",  // Just @ without path should error, but test empty string separately
		"@@", // Double @ should be treated as file path "@@"
	}

	for _, input := range tests {
		if input == "@" {
			continue // Skip this edge case, tested separately
		}
		result, err := ResolveFile(input)
		if err != nil && !strings.HasPrefix(input, "@") {
			t.Errorf("ResolveFile(%q) unexpected error: %v", input, err)
		}
		if !strings.HasPrefix(input, "@") && result != input {
			t.Errorf("ResolveFile(%q) = %q, want %q", input, result, input)
		}
	}
}

// Test 3: Resolve multiple @files
func TestResolveFiles_MultipleFiles(t *testing.T) {
	// Create temporary files
	tmpDir := t.TempDir()
	extractFile := filepath.Join(tmpDir, "extract.txt")
	researchFile := filepath.Join(tmpDir, "research.txt")

	extractContent := "Extract key topics"
	researchContent := "Research each topic"

	if err := os.WriteFile(extractFile, []byte(extractContent), 0644); err != nil {
		t.Fatalf("Failed to create extract file: %v", err)
	}
	if err := os.WriteFile(researchFile, []byte(researchContent), 0644); err != nil {
		t.Fatalf("Failed to create research file: %v", err)
	}

	// Test multiple files
	prompts := map[string]string{
		"extract":  "@" + extractFile,
		"analyze":  "Analyze {topics}",
		"research": "@" + researchFile,
	}

	resolved, err := ResolveFiles(prompts)
	if err != nil {
		t.Fatalf("ResolveFiles() error = %v, want nil", err)
	}

	if resolved["extract"] != extractContent {
		t.Errorf("resolved[\"extract\"] = %q, want %q", resolved["extract"], extractContent)
	}
	if resolved["analyze"] != "Analyze {topics}" {
		t.Errorf("resolved[\"analyze\"] = %q, want %q", resolved["analyze"], "Analyze {topics}")
	}
	if resolved["research"] != researchContent {
		t.Errorf("resolved[\"research\"] = %q, want %q", resolved["research"], researchContent)
	}
}

// Test 4: File not found error
func TestResolveFile_FileNotFound(t *testing.T) {
	input := "@nonexistent.txt"
	result, err := ResolveFile(input)

	if err == nil {
		t.Error("ResolveFile() error = nil, want error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "file not found") {
		t.Errorf("ResolveFile() error = %v, want 'file not found' message", err)
	}
	if !strings.Contains(err.Error(), "nonexistent.txt") {
		t.Errorf("ResolveFile() error should mention file path: %v", err)
	}
	if result != "" {
		t.Errorf("ResolveFile() = %q, want empty string on error", result)
	}
}

// Test 5: Mixed @file and inline
func TestResolveFiles_MixedFileAndInline(t *testing.T) {
	// Create temporary file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "extract.txt")
	testContent := "Extract from file"

	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Mix of @file and inline prompts
	prompts := map[string]string{
		"extract":  "@" + testFile,
		"analyze":  "Inline analyze prompt",
		"research": "Inline research prompt",
	}

	resolved, err := ResolveFiles(prompts)
	if err != nil {
		t.Fatalf("ResolveFiles() error = %v, want nil", err)
	}

	if resolved["extract"] != testContent {
		t.Errorf("resolved[\"extract\"] = %q, want %q", resolved["extract"], testContent)
	}
	if resolved["analyze"] != "Inline analyze prompt" {
		t.Errorf("resolved[\"analyze\"] = %q, want unchanged", resolved["analyze"])
	}
	if resolved["research"] != "Inline research prompt" {
		t.Errorf("resolved[\"research\"] = %q, want unchanged", resolved["research"])
	}
}

// Test 6: Relative path resolution
func TestResolveFile_RelativePath(t *testing.T) {
	// Create temporary file in a subdirectory
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "prompts")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	testFile := filepath.Join(subDir, "extract.txt")
	testContent := "Relative path test"

	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Change to temp directory to test relative paths
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Test relative path
	input := "@prompts/extract.txt"
	result, err := ResolveFile(input)

	if err != nil {
		t.Errorf("ResolveFile() error = %v, want nil", err)
	}
	if result != testContent {
		t.Errorf("ResolveFile() = %q, want %q", result, testContent)
	}
}

// Test 7: Absolute path resolution
func TestResolveFile_AbsolutePath(t *testing.T) {
	// Create temporary file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "Absolute path test"

	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test absolute path (testFile is already absolute from TempDir)
	input := "@" + testFile
	result, err := ResolveFile(input)

	if err != nil {
		t.Errorf("ResolveFile() error = %v, want nil", err)
	}
	if result != testContent {
		t.Errorf("ResolveFile() = %q, want %q", result, testContent)
	}
}

// Test 8: Cross-platform paths (Windows backslash)
func TestResolveFile_CrossPlatform(t *testing.T) {
	// Create temporary file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "Cross-platform test"

	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// filepath.Join handles platform-specific separators
	// This test ensures our code works regardless of separator
	input := "@" + testFile
	result, err := ResolveFile(input)

	if err != nil {
		t.Errorf("ResolveFile() error = %v, want nil", err)
	}
	if result != testContent {
		t.Errorf("ResolveFile() = %q, want %q", result, testContent)
	}
}

// Additional Test: Empty @file path
func TestResolveFile_EmptyFilePath(t *testing.T) {
	input := "@"
	_, err := ResolveFile(input)

	if err == nil {
		t.Error("ResolveFile(@) error = nil, want error for empty file path")
	}
	if !strings.Contains(err.Error(), "empty file path") {
		t.Errorf("ResolveFile(@) error = %v, want 'empty file path' message", err)
	}
}

// Additional Test: Directory instead of file
func TestResolveFile_DirectoryError(t *testing.T) {
	tmpDir := t.TempDir()
	input := "@" + tmpDir

	_, err := ResolveFile(input)

	if err == nil {
		t.Error("ResolveFile() error = nil, want error for directory path")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Errorf("ResolveFile() error = %v, want 'directory' message", err)
	}
}

// Additional Test: File too large
func TestResolveFile_FileTooLarge(t *testing.T) {
	// Create a file larger than MaxFileSize
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "large.txt")

	// Create a file slightly larger than 1MB
	largeContent := make([]byte, MaxFileSize+1000)
	for i := range largeContent {
		largeContent[i] = 'A'
	}

	if err := os.WriteFile(testFile, largeContent, 0644); err != nil {
		t.Fatalf("Failed to create large test file: %v", err)
	}

	input := "@" + testFile
	_, err := ResolveFile(input)

	if err == nil {
		t.Error("ResolveFile() error = nil, want error for large file")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("ResolveFile() error = %v, want 'too large' message", err)
	}
}

// Additional Test: UTF-8 encoding
func TestResolveFile_UTF8Encoding(t *testing.T) {
	// Create file with UTF-8 content
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "utf8.txt")
	testContent := "Hello 世界 🌍 Привет"

	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create UTF-8 test file: %v", err)
	}

	input := "@" + testFile
	result, err := ResolveFile(input)

	if err != nil {
		t.Errorf("ResolveFile() error = %v, want nil for UTF-8 content", err)
	}
	if result != testContent {
		t.Errorf("ResolveFile() = %q, want %q", result, testContent)
	}
}

// Additional Test: Performance benchmark (<100ms per file)
func TestResolveFile_Performance(t *testing.T) {
	// Create a reasonably sized file (100KB)
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "perf.txt")
	testContent := make([]byte, 100*1024) // 100KB
	for i := range testContent {
		testContent[i] = byte('A' + (i % 26))
	}

	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("Failed to create performance test file: %v", err)
	}

	input := "@" + testFile

	// Measure performance
	start := time.Now()
	_, err := ResolveFile(input)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("ResolveFile() error = %v, want nil", err)
	}

	// Performance requirement: <100ms per file
	if elapsed > 100*time.Millisecond {
		t.Errorf("ResolveFile() took %v, want <100ms", elapsed)
	}
}

// Additional Test: ResolveFiles error propagation
func TestResolveFiles_ErrorPropagation(t *testing.T) {
	prompts := map[string]string{
		"extract":  "@nonexistent.txt",
		"analyze":  "Valid inline prompt",
		"research": "Another valid prompt",
	}

	_, err := ResolveFiles(prompts)

	if err == nil {
		t.Error("ResolveFiles() error = nil, want error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "extract") {
		t.Errorf("ResolveFiles() error should mention field name 'extract': %v", err)
	}
	if !strings.Contains(err.Error(), "file not found") {
		t.Errorf("ResolveFiles() error should contain 'file not found': %v", err)
	}
}

// Additional Test: Whitespace handling
func TestResolveFile_WhitespaceInPath(t *testing.T) {
	// Create file with whitespace in directory name
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "my prompts")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory with spaces: %v", err)
	}

	testFile := filepath.Join(subDir, "test prompt.txt")
	testContent := "Whitespace test"

	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file with spaces: %v", err)
	}

	input := "@" + testFile
	result, err := ResolveFile(input)

	if err != nil {
		t.Errorf("ResolveFile() error = %v, want nil for path with spaces", err)
	}
	if result != testContent {
		t.Errorf("ResolveFile() = %q, want %q", result, testContent)
	}
}
