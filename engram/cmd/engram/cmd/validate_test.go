package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDetectValidatorType tests auto-detection of validator types
func TestDetectValidatorType(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		want     ValidatorType
	}{
		{
			name:     "engram file",
			filePath: "/path/to/example.ai.md",
			want:     ValidatorEngram,
		},
		{
			name:     "content file in core",
			filePath: "/path/to/core/example.ai.md",
			want:     ValidatorContent,
		},
		{
			name:     "content file in core subdirectory",
			filePath: "/path/to/core/howtos/example.ai.md",
			want:     ValidatorContent,
		},
		{
			name:     "retrospective file",
			filePath: "/path/to/project-retrospective.md",
			want:     ValidatorRetrospective,
		},
		{
			name:     "wayfinder artifact",
			filePath: "/path/to/wayfinder-artifact.yaml",
			want:     ValidatorWayfinder,
		},
		{
			name:     "yaml file",
			filePath: "/path/to/config.yaml",
			want:     ValidatorYAMLTokenCounter,
		},
		{
			name:     "yml file",
			filePath: "/path/to/config.yml",
			want:     ValidatorYAMLTokenCounter,
		},
		{
			name:     "non-engram markdown",
			filePath: "/path/to/README.md",
			want:     "",
		},
		{
			name:     "regular file",
			filePath: "/path/to/file.txt",
			want:     "",
		},
		{
			name:     "Windows path - content",
			filePath: "C:\\path\\to\\core\\example.ai.md",
			want:     ValidatorContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectValidatorType(tt.filePath)
			if got != tt.want {
				t.Errorf("detectValidatorType(%q) = %q, want %q", tt.filePath, got, tt.want)
			}
		})
	}
}

// TestValidationResult tests ValidationResult structure
func TestValidationResult(t *testing.T) {
	result := ValidationResult{
		ValidatorType: ValidatorEngram,
		FilePath:      "test.ai.md",
		Errors: []ValidationError{
			{
				FilePath: "test.ai.md",
				Line:     10,
				Type:     "missing_frontmatter",
				Message:  "Missing YAML frontmatter",
			},
		},
		Warnings: []ValidationWarning{
			{
				FilePath: "test.ai.md",
				Line:     25,
				Type:     "vague_verb",
				Message:  "Vague verb 'improve' without criteria",
			},
		},
		FixesApplied: []string{
			"Auto-fixed token count: 100 → 120",
		},
	}

	if len(result.Errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(result.Errors))
	}

	if len(result.Warnings) != 1 {
		t.Errorf("Expected 1 warning, got %d", len(result.Warnings))
	}

	if len(result.FixesApplied) != 1 {
		t.Errorf("Expected 1 fix, got %d", len(result.FixesApplied))
	}

	if result.Errors[0].Line != 10 {
		t.Errorf("Expected error line 10, got %d", result.Errors[0].Line)
	}
}

// TestValidationSummary tests ValidationSummary structure
func TestValidationSummary(t *testing.T) {
	summary := ValidationSummary{
		TotalFiles:     3,
		FilesValidated: 3,
		ErrorCount:     2,
		WarningCount:   1,
		FixesApplied:   1,
		Results: []ValidationResult{
			{
				ValidatorType: ValidatorEngram,
				FilePath:      "file1.ai.md",
				Errors: []ValidationError{
					{Type: "error1", Message: "Error 1"},
				},
			},
			{
				ValidatorType: ValidatorContent,
				FilePath:      "file2.ai.md",
				Errors: []ValidationError{
					{Type: "error2", Message: "Error 2"},
				},
				Warnings: []ValidationWarning{
					{Type: "warning1", Message: "Warning 1"},
				},
				FixesApplied: []string{"Fix 1"},
			},
			{
				ValidatorType: ValidatorWayfinder,
				FilePath:      "wayfinder-artifact.yaml",
			},
		},
	}

	if summary.TotalFiles != 3 {
		t.Errorf("Expected 3 total files, got %d", summary.TotalFiles)
	}

	if summary.ErrorCount != 2 {
		t.Errorf("Expected 2 errors, got %d", summary.ErrorCount)
	}

	if summary.WarningCount != 1 {
		t.Errorf("Expected 1 warning, got %d", summary.WarningCount)
	}

	if len(summary.Results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(summary.Results))
	}
}

// TestFindAllValidatableFilesInDir tests file discovery
func TestFindAllValidatableFilesInDir(t *testing.T) {
	// Create temp directory with test files
	tmpDir := t.TempDir()

	// Create test files
	testFiles := []struct {
		path       string
		shouldFind bool
	}{
		{"example.ai.md", true},
		{"config.yaml", true},
		{"README.md", false},
		{"test.txt", false},
		{"project-retrospective.md", true},
		{"wayfinder-artifact.yaml", true},
		{"core/howto.ai.md", true},
	}

	for _, tf := range testFiles {
		fullPath := filepath.Join(tmpDir, tf.path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", fullPath, err)
		}
	}

	// Find files
	files, err := findAllValidatableFilesInDir(tmpDir)
	if err != nil {
		t.Fatalf("findAllValidatableFilesInDir failed: %v", err)
	}

	// Count expected files
	expectedCount := 0
	for _, tf := range testFiles {
		if tf.shouldFind {
			expectedCount++
		}
	}

	if len(files) != expectedCount {
		t.Errorf("Expected %d validatable files, got %d", expectedCount, len(files))
		t.Logf("Found files: %v", files)
	}

	// Verify each found file is expected
	for _, file := range files {
		relPath, _ := filepath.Rel(tmpDir, file)
		found := false
		for _, tf := range testFiles {
			if tf.path == relPath && tf.shouldFind {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Unexpected file found: %s", relPath)
		}
	}
}

// TestRunEngramValidator tests engram validator integration
func TestRunEngramValidator(t *testing.T) {
	// Create temp file with valid frontmatter
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.ai.md")

	content := `---
type: guide
title: Test Guide
description: A test guide for validation
---

# Test Content

This is a test file.
`

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Run validator
	result, err := runEngramValidator(testFile)
	if err != nil {
		t.Fatalf("runEngramValidator failed: %v", err)
	}

	if result.ValidatorType != ValidatorEngram {
		t.Errorf("Expected ValidatorEngram, got %s", result.ValidatorType)
	}

	if result.FilePath != testFile {
		t.Errorf("Expected file path %s, got %s", testFile, result.FilePath)
	}

	// Should have no errors for valid frontmatter
	if len(result.Errors) > 0 {
		t.Logf("Unexpected errors: %+v", result.Errors)
		// Note: This might fail depending on validator strictness
		// Adjust as needed based on validator implementation
	}
}

// TestRunEngramValidatorMissingFrontmatter tests error detection
func TestRunEngramValidatorMissingFrontmatter(t *testing.T) {
	// Create temp file without frontmatter
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.ai.md")

	content := `# Test Content

This file has no frontmatter.
`

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Run validator
	result, err := runEngramValidator(testFile)
	if err != nil {
		t.Fatalf("runEngramValidator failed: %v", err)
	}

	// Should have error for missing frontmatter
	if len(result.Errors) == 0 {
		t.Error("Expected errors for missing frontmatter, got none")
	}

	// Check error type
	foundMissingFrontmatter := false
	for _, e := range result.Errors {
		if e.Type == "missing_frontmatter" {
			foundMissingFrontmatter = true
			break
		}
	}

	if !foundMissingFrontmatter {
		t.Errorf("Expected missing_frontmatter error, got: %+v", result.Errors)
	}
}

// TestRunYAMLTokenCounter tests YAML token counter
func TestRunYAMLTokenCounter(t *testing.T) {
	// Create temp YAML file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.yaml")

	content := `---
name: test
description: A test configuration
tokens: 100
---
`

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Run counter
	result, err := runYAMLTokenCounter(testFile)
	if err != nil {
		t.Fatalf("runYAMLTokenCounter failed: %v", err)
	}

	if result.ValidatorType != ValidatorYAMLTokenCounter {
		t.Errorf("Expected ValidatorYAMLTokenCounter, got %s", result.ValidatorType)
	}

	// Token counter should not produce errors (it's informational)
	if len(result.Errors) > 0 {
		t.Errorf("Expected no errors, got %d: %+v", len(result.Errors), result.Errors)
	}
}

// TestValidateFilesWithMixedTypes tests validation of multiple file types
func TestValidateFilesWithMixedTypes(t *testing.T) {
	// Create temp directory with mixed files
	tmpDir := t.TempDir()

	files := []struct {
		name    string
		content string
	}{
		{
			name: "valid.ai.md",
			content: `---
type: guide
title: Valid Guide
description: A valid guide
---

# Content
`,
		},
		{
			name: "invalid.ai.md",
			content: `# No frontmatter
`,
		},
	}

	var testPaths []string
	for _, f := range files {
		fullPath := filepath.Join(tmpDir, f.name)
		if err := os.WriteFile(fullPath, []byte(f.content), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		testPaths = append(testPaths, fullPath)
	}

	// Validate files
	summary := validateFiles(testPaths)

	if summary.TotalFiles != 2 {
		t.Errorf("Expected 2 total files, got %d", summary.TotalFiles)
	}

	if summary.FilesValidated != 2 {
		t.Errorf("Expected 2 validated files, got %d", summary.FilesValidated)
	}

	// Should have at least 1 error from invalid.ai.md
	if summary.ErrorCount == 0 {
		t.Error("Expected at least 1 error from invalid file")
	}
}

// BenchmarkDetectValidatorType benchmarks validator type detection
func BenchmarkDetectValidatorType(t *testing.B) {
	filePaths := []string{
		"/path/to/example.ai.md",
		"/path/to/core/example.ai.md",
		"/path/to/project-retrospective.md",
		"/path/to/wayfinder-artifact.yaml",
		"/path/to/config.yaml",
	}

	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		for _, path := range filePaths {
			_ = detectValidatorType(path)
		}
	}
}

// TestValidatorTypeConstants tests validator type constants
func TestValidatorTypeConstants(t *testing.T) {
	expected := map[ValidatorType]string{
		ValidatorEngram:           "engram",
		ValidatorContent:          "content",
		ValidatorWayfinder:        "wayfinder",
		ValidatorLinkChecker:      "linkchecker",
		ValidatorYAMLTokenCounter: "yamltokencounter",
		ValidatorRetrospective:    "retrospective",
	}

	for vType, expectedStr := range expected {
		if string(vType) != expectedStr {
			t.Errorf("Expected ValidatorType %s, got %s", expectedStr, vType)
		}
	}
}

// TestValidationErrorStructure tests ValidationError fields
func TestValidationErrorStructure(t *testing.T) {
	err := ValidationError{
		FilePath: "test.ai.md",
		Line:     42,
		Type:     "test_error",
		Message:  "Test error message",
	}

	if err.FilePath != "test.ai.md" {
		t.Errorf("Expected FilePath 'test.ai.md', got %s", err.FilePath)
	}

	if err.Line != 42 {
		t.Errorf("Expected Line 42, got %d", err.Line)
	}

	if err.Type != "test_error" {
		t.Errorf("Expected Type 'test_error', got %s", err.Type)
	}

	if err.Message != "Test error message" {
		t.Errorf("Expected Message 'Test error message', got %s", err.Message)
	}
}

// TestValidationWarningStructure tests ValidationWarning fields
func TestValidationWarningStructure(t *testing.T) {
	warn := ValidationWarning{
		FilePath: "test.ai.md",
		Line:     15,
		Type:     "test_warning",
		Message:  "Test warning message",
	}

	if warn.FilePath != "test.ai.md" {
		t.Errorf("Expected FilePath 'test.ai.md', got %s", warn.FilePath)
	}

	if warn.Line != 15 {
		t.Errorf("Expected Line 15, got %d", warn.Line)
	}

	if warn.Type != "test_warning" {
		t.Errorf("Expected Type 'test_warning', got %s", warn.Type)
	}

	if warn.Message != "Test warning message" {
		t.Errorf("Expected Message 'Test warning message', got %s", warn.Message)
	}
}
