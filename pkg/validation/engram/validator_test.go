package engram

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Test fixtures
const (
	validEngram = `---
type: reference
title: Valid Engram Test
description: A valid engram file for testing
tags: [test]
---

# Valid Engram

This is a valid engram file with proper structure.

## Principle: Context Embedding

Always embed full context in prompts without references to previous sections.

**Good Example**:
` + "```" + `
Use the Repository Pattern to implement the user service: separate data access logic
into repositories with interfaces (IUserRepository) and concrete implementations
(UserRepository that extends IUserRepository).
` + "```" + `

## Task: Create Authentication

Create JWT authentication with the following constraints:

**Constraints**:
- Scope: Only modify auth.service.ts (max 1 file)
- Token budget: <2000 tokens
- Time bound: Complete in single response
- Dependencies: Use existing jsonwebtoken library

**Success Criteria**:
- [ ] Tests pass with 80%+ coverage
- [ ] Tokens expire after 24 hours
- [ ] Response time <100ms

This engram demonstrates proper use of constraints and examples.
`

	missingFrontmatter = `# No Frontmatter

This file has no frontmatter.
`

	invalidType = `---
type: invalid_type
title: Test
description: Testing invalid type
---

# Test
`

	contextReferences = `---
type: reference
title: Context Reference Test
description: Testing context reference detection
---

# Context References

## Bad Pattern

Use the pattern mentioned above to implement the service.

As discussed earlier, the Repository Pattern is useful.

Refer to the previous section for implementation details.

See the example described earlier for more information.
`

	vagueVerbsFixture = `---
type: reference
title: Vague Verbs Test
description: Testing vague verb detection
---

# Vague Verbs

We need to improve the performance of the database.

Please optimize the code for better speed.

Fix the authentication bug.

Enhance the user experience.

Update the configuration.

Refactor the codebase.
`

	missingExamples = `---
type: reference
title: Missing Examples Test
description: Testing missing example detection
---

# Missing Examples

## Principle: DRY Principle

Don't Repeat Yourself - avoid code duplication.

## Pattern: Repository Pattern

Use repositories to separate data access logic.

## Guideline: Error Handling

Always handle errors properly.
`

	missingConstraints = `---
type: reference
title: Missing Constraints Test
description: Testing missing constraint detection
---

# Missing Constraints

Implement user authentication.

Create a new dashboard component.

Build a REST API for user management.

Generate test data for the database.

Write integration tests.

Develop a configuration system.
`

	descriptionTooLong = `---
type: reference
title: Description Too Long Test
description: This is a very long description that exceeds the 200 character limit. It contains way too much text and should trigger a validation error because descriptions must be concise and under 200 characters in total length.
---

# Test
`

	specificVerbs = `---
type: reference
title: Specific Instructions
description: Test specific instructions
---

# Specific Instructions

Optimize the database query by adding index on user_id column (target: <50ms).

Improve error handling by adding try-catch for FileNotFoundError and PermissionError.
`

	malformedYAML = `---
type: reference
title: [unclosed
---

# Test
`

	emptyFile = ``

	unicodeContent = `---
type: reference
title: Unicode Test 你好
description: Test
---

# Test 🎉
`
)

func TestValidateFrontmatter(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		wantErrorType string
	}{
		{
			name:          "valid frontmatter",
			content:       validEngram,
			wantErrorType: "",
		},
		{
			name:          "missing frontmatter",
			content:       missingFrontmatter,
			wantErrorType: "missing_frontmatter",
		},
		{
			name:          "invalid type",
			content:       invalidType,
			wantErrorType: "invalid_type",
		},
		{
			name:          "description too long",
			content:       descriptionTooLong,
			wantErrorType: "description_too_long",
		},
		{
			name:          "malformed YAML",
			content:       malformedYAML,
			wantErrorType: "invalid_frontmatter",
		},
		{
			name:          "empty file",
			content:       emptyFile,
			wantErrorType: "missing_frontmatter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile := createTempFile(t, tt.content)
			defer os.Remove(tmpFile) //nolint:errcheck // Cleanup in tests //nolint:errcheck // Cleanup in tests

			errors, err := ValidateFile(tmpFile)
			if err != nil {
				t.Fatalf("ValidateFile() error = %v", err)
			}

			if tt.wantErrorType == "" {
				// Should have no frontmatter errors
				for _, e := range errors {
					if strings.Contains(e.ErrorType, "frontmatter") ||
						strings.Contains(e.ErrorType, "field") ||
						strings.Contains(e.ErrorType, "type") ||
						strings.Contains(e.ErrorType, "title") ||
						strings.Contains(e.ErrorType, "description") {
						t.Errorf("Expected no frontmatter errors, got %s: %s", e.ErrorType, e.Message)
					}
				}
			} else {
				// Should have specific error
				found := false
				for _, e := range errors {
					if e.ErrorType == tt.wantErrorType {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error type %s, got errors: %+v", tt.wantErrorType, errors)
				}
			}
		})
	}
}

func TestDetectContextReferences(t *testing.T) {
	tmpFile := createTempFile(t, contextReferences)
	defer os.Remove(tmpFile) //nolint:errcheck // Cleanup in tests //nolint:errcheck // Cleanup in tests

	errors, err := ValidateFile(tmpFile)
	if err != nil {
		t.Fatalf("ValidateFile() error = %v", err)
	}

	contextErrors := filterErrorsByType(errors, "context_reference")
	if len(contextErrors) == 0 {
		t.Error("Expected context reference errors, got none")
	}

	// Check for specific patterns
	messages := getErrorMessages(contextErrors)
	expectedPatterns := []string{"mentioned above", "discussed earlier"}
	for _, pattern := range expectedPatterns {
		found := false
		for _, msg := range messages {
			if strings.Contains(strings.ToLower(msg), pattern) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected to find pattern '%s' in messages: %v", pattern, messages)
		}
	}
}

func TestDetectVagueVerbs(t *testing.T) {
	tmpFile := createTempFile(t, vagueVerbsFixture)
	defer os.Remove(tmpFile) //nolint:errcheck // Cleanup in tests

	errors, err := ValidateFile(tmpFile)
	if err != nil {
		t.Fatalf("ValidateFile() error = %v", err)
	}

	vagueErrors := filterErrorsByType(errors, "vague_verb")
	if len(vagueErrors) == 0 {
		t.Error("Expected vague verb errors, got none")
	}

	// Check for specific vague verbs
	messages := getErrorMessages(vagueErrors)
	expectedVerbs := []string{"improve", "optimize", "fix", "enhance"}
	for _, verb := range expectedVerbs {
		found := false
		for _, msg := range messages {
			if strings.Contains(strings.ToLower(msg), verb) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected to detect verb '%s', got messages: %v", verb, messages)
		}
	}
}

func TestSpecificVerbsAllowed(t *testing.T) {
	tmpFile := createTempFile(t, specificVerbs)
	defer os.Remove(tmpFile) //nolint:errcheck // Cleanup in tests

	errors, err := ValidateFile(tmpFile)
	if err != nil {
		t.Fatalf("ValidateFile() error = %v", err)
	}

	vagueErrors := filterErrorsByType(errors, "vague_verb")
	if len(vagueErrors) > 0 {
		t.Errorf("Expected no vague verb errors for specific instructions, got: %+v", vagueErrors)
	}
}

func TestDetectMissingExamples(t *testing.T) {
	tmpFile := createTempFile(t, missingExamples)
	defer os.Remove(tmpFile) //nolint:errcheck // Cleanup in tests

	errors, err := ValidateFile(tmpFile)
	if err != nil {
		t.Fatalf("ValidateFile() error = %v", err)
	}

	exampleErrors := filterErrorsByType(errors, "missing_example")
	if len(exampleErrors) == 0 {
		t.Error("Expected missing example errors, got none")
	}
}

func TestExamplesPresent(t *testing.T) {
	tmpFile := createTempFile(t, validEngram)
	defer os.Remove(tmpFile) //nolint:errcheck // Cleanup in tests

	errors, err := ValidateFile(tmpFile)
	if err != nil {
		t.Fatalf("ValidateFile() error = %v", err)
	}

	exampleErrors := filterErrorsByType(errors, "missing_example")
	if len(exampleErrors) > 0 {
		t.Errorf("Expected no missing example errors, got: %+v", exampleErrors)
	}
}

func TestDetectMissingConstraints(t *testing.T) {
	tmpFile := createTempFile(t, missingConstraints)
	defer os.Remove(tmpFile) //nolint:errcheck // Cleanup in tests

	errors, err := ValidateFile(tmpFile)
	if err != nil {
		t.Fatalf("ValidateFile() error = %v", err)
	}

	constraintErrors := filterErrorsByType(errors, "missing_constraints")
	if len(constraintErrors) == 0 {
		t.Error("Expected missing constraint errors, got none")
	}

	// Check for specific task verbs
	messages := getErrorMessages(constraintErrors)
	expectedVerbs := []string{"implement", "create", "build", "generate"}
	for _, verb := range expectedVerbs {
		found := false
		for _, msg := range messages {
			if strings.Contains(strings.ToLower(msg), verb) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected to detect task verb '%s', got messages: %v", verb, messages)
		}
	}
}

func TestConstraintsPresent(t *testing.T) {
	tmpFile := createTempFile(t, validEngram)
	defer os.Remove(tmpFile) //nolint:errcheck // Cleanup in tests

	errors, err := ValidateFile(tmpFile)
	if err != nil {
		t.Fatalf("ValidateFile() error = %v", err)
	}

	constraintErrors := filterErrorsByType(errors, "missing_constraints")
	if len(constraintErrors) > 0 {
		t.Errorf("Expected no missing constraint errors, got: %+v", constraintErrors)
	}
}

func TestValidFileReturnsNoErrors(t *testing.T) {
	tmpFile := createTempFile(t, validEngram)
	defer os.Remove(tmpFile) //nolint:errcheck // Cleanup in tests

	errors, err := ValidateFile(tmpFile)
	if err != nil {
		t.Fatalf("ValidateFile() error = %v", err)
	}

	// May have some warnings, but no critical errors
	criticalTypes := []string{"missing_frontmatter", "invalid_frontmatter", "file_error"}
	for _, e := range errors {
		for _, ct := range criticalTypes {
			if e.ErrorType == ct {
				t.Errorf("Expected no critical errors, got %s: %s", e.ErrorType, e.Message)
			}
		}
	}
}

func TestNonexistentFile(t *testing.T) {
	errors, err := ValidateFile("/nonexistent/file.ai.md")
	if err != nil {
		t.Fatalf("ValidateFile() error = %v", err)
	}

	found := false
	for _, e := range errors {
		if e.ErrorType == "file_error" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected file_error for nonexistent file")
	}
}

func TestLineNumbersReported(t *testing.T) {
	tmpFile := createTempFile(t, contextReferences)
	defer os.Remove(tmpFile) //nolint:errcheck // Cleanup in tests

	errors, err := ValidateFile(tmpFile)
	if err != nil {
		t.Fatalf("ValidateFile() error = %v", err)
	}

	contextErrors := filterErrorsByType(errors, "context_reference")
	if len(contextErrors) == 0 {
		t.Fatal("Expected context reference errors for line number test")
	}

	for _, e := range contextErrors {
		if e.Line == nil {
			t.Error("Expected line numbers to be reported")
		}
	}
}

func TestValidateDirectory(t *testing.T) {
	// Create temporary directory with test files
	tmpDir := t.TempDir()

	// Create test files
	files := map[string]string{
		"valid.ai.md":   validEngram,
		"invalid.ai.md": missingFrontmatter,
		"other.txt":     "not an engram file",
	}

	for name, content := range files {
		path := filepath.Join(tmpDir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	results, err := ValidateDirectory(tmpDir)
	if err != nil {
		t.Fatalf("ValidateDirectory() error = %v", err)
	}

	// Should find exactly 2 .ai.md files
	if len(results) != 2 {
		t.Errorf("Expected 2 .ai.md files, got %d", len(results))
	}

	// All results should be for .ai.md files
	for path := range results {
		if !strings.HasSuffix(path, ".ai.md") {
			t.Errorf("Expected only .ai.md files, got %s", path)
		}
	}
}

func TestUnicodeContent(t *testing.T) {
	tmpFile := createTempFile(t, unicodeContent)
	defer os.Remove(tmpFile) //nolint:errcheck // Cleanup in tests

	errors, err := ValidateFile(tmpFile)
	if err != nil {
		t.Fatalf("ValidateFile() error = %v", err)
	}

	// Should not crash, errors should be a list
	if errors == nil {
		t.Error("Expected errors list, got nil")
	}
}

func TestNewValidator(t *testing.T) {
	validator := NewValidator("/test/path.ai.md")
	if validator.filePath != "/test/path.ai.md" {
		t.Errorf("Expected filePath /test/path.ai.md, got %s", validator.filePath)
	}
	if len(validator.errors) != 0 {
		t.Errorf("Expected empty errors list, got %d errors", len(validator.errors))
	}
}

func TestMultipleValidations(t *testing.T) {
	tmpFile := createTempFile(t, contextReferences)
	defer os.Remove(tmpFile) //nolint:errcheck // Cleanup in tests

	validator := NewValidator(tmpFile)

	errors1 := validator.Validate()
	errors2 := validator.Validate()

	// Errors should be reset between runs
	if len(errors1) != len(errors2) {
		t.Errorf("Expected same number of errors in multiple runs, got %d and %d", len(errors1), len(errors2))
	}
}

// Helper functions

func createTempFile(t *testing.T, content string) string {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "engram-test-*.ai.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}
	return tmpFile.Name()
}

func filterErrorsByType(errors []ValidationError, errorType string) []ValidationError {
	var filtered []ValidationError
	for _, e := range errors {
		if e.ErrorType == errorType {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

func getErrorMessages(errors []ValidationError) []string {
	messages := make([]string, len(errors))
	for i, e := range errors {
		messages[i] = e.Message
	}
	return messages
}
