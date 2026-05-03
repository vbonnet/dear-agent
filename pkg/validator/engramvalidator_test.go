package validator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test fixture paths
const testdataDir = "testdata"

func getFixturePath(name string) string {
	return filepath.Join(testdataDir, name)
}

// TestFrontmatterValidation tests frontmatter validation rules.
func TestFrontmatterValidation(t *testing.T) {
	tests := []struct {
		name      string
		fixture   string
		wantError bool
		errorType string
	}{
		{
			name:      "valid frontmatter",
			fixture:   "valid_engram.ai.md",
			wantError: false,
		},
		{
			name:      "missing frontmatter",
			fixture:   "missing_frontmatter.ai.md",
			wantError: true,
			errorType: ErrorTypeMissingFrontmatter,
		},
		{
			name:      "invalid type",
			fixture:   "invalid_frontmatter_type.ai.md",
			wantError: true,
			errorType: ErrorTypeInvalidType,
		},
		{
			name:      "description too long",
			fixture:   "description_too_long.ai.md",
			wantError: true,
			errorType: ErrorTypeDescriptionTooLong,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors, err := ValidateFile(getFixturePath(tt.fixture))
			require.NoError(t, err)

			if tt.wantError {
				found := false
				for _, e := range errors {
					if e.ErrorType == tt.errorType {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected error type %s not found", tt.errorType)
			} else {
				// Valid frontmatter should not have frontmatter-related errors
				for _, e := range errors {
					assert.NotContains(t, []string{
						ErrorTypeMissingFrontmatter,
						ErrorTypeInvalidFrontmatter,
						ErrorTypeMissingField,
						ErrorTypeInvalidType,
						ErrorTypeInvalidTitle,
						ErrorTypeInvalidDescription,
					}, e.ErrorType)
				}
			}
		})
	}
}

// TestMissingRequiredFields tests detection of missing required fields.
func TestMissingRequiredFields(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		expectedType string
		expectedMsg  string
	}{
		{
			name:         "missing title",
			content:      "---\ntype: reference\ndescription: test\n---\n# Test",
			expectedType: ErrorTypeMissingField,
			expectedMsg:  "title",
		},
		{
			name:         "missing description",
			content:      "---\ntype: reference\ntitle: Test\n---\n# Test",
			expectedType: ErrorTypeMissingField,
			expectedMsg:  "description",
		},
		{
			name:         "missing type",
			content:      "---\ntitle: Test\ndescription: test\n---\n# Test",
			expectedType: ErrorTypeMissingField,
			expectedMsg:  "type",
		},
		{
			name:         "empty title",
			content:      "---\ntype: reference\ntitle: ''\ndescription: test\n---\n# Test",
			expectedType: ErrorTypeInvalidTitle,
			expectedMsg:  "non-empty",
		},
		{
			name:         "empty description",
			content:      "---\ntype: reference\ntitle: Test\ndescription: ''\n---\n# Test",
			expectedType: ErrorTypeInvalidDescription,
			expectedMsg:  "non-empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpfile, err := os.CreateTemp(t.TempDir(), "test-*.ai.md")
			require.NoError(t, err)
			defer os.Remove(tmpfile.Name())

			_, err = tmpfile.WriteString(tt.content)
			require.NoError(t, err)
			tmpfile.Close()

			errors, err := ValidateFile(tmpfile.Name())
			require.NoError(t, err)

			found := false
			for _, e := range errors {
				if e.ErrorType == tt.expectedType && strings.Contains(e.Message, tt.expectedMsg) {
					found = true
					break
				}
			}
			assert.True(t, found, "Expected error type %s with message containing '%s' not found", tt.expectedType, tt.expectedMsg)
		})
	}
}

// TestContextReferenceDetection tests context reference detection.
func TestContextReferenceDetection(t *testing.T) {
	t.Run("context references detected", func(t *testing.T) {
		errors, err := ValidateFile(getFixturePath("context_references.ai.md"))
		require.NoError(t, err)

		contextErrors := filterErrorsByType(errors, ErrorTypeContextReference)
		assert.Greater(t, len(contextErrors), 0, "Should detect context references")

		// Check specific patterns
		messages := getErrorMessages(contextErrors)
		assert.True(t, containsSubstring(messages, "mentioned above"), "Should detect 'mentioned above'")
		assert.True(t, containsSubstring(messages, "discussed earlier"), "Should detect 'discussed earlier'")
	})

	t.Run("no context references in valid file", func(t *testing.T) {
		errors, err := ValidateFile(getFixturePath("valid_engram.ai.md"))
		require.NoError(t, err)

		contextErrors := filterErrorsByType(errors, ErrorTypeContextReference)
		assert.Equal(t, 0, len(contextErrors))
	})
}

// TestVagueVerbDetection tests vague verb detection.
func TestVagueVerbDetection(t *testing.T) {
	t.Run("vague verbs detected", func(t *testing.T) {
		errors, err := ValidateFile(getFixturePath("vague_verbs.ai.md"))
		require.NoError(t, err)

		vagueErrors := filterErrorsByType(errors, ErrorTypeVagueVerb)
		assert.Greater(t, len(vagueErrors), 0, "Should detect vague verbs")

		// Check for specific vague verbs
		messages := getErrorMessages(vagueErrors)
		vagueVerbs := []string{"improve", "optimize", "fix", "enhance"}
		for _, verb := range vagueVerbs {
			assert.True(t, containsSubstring(messages, verb), "Should detect '%s'", verb)
		}
	})

	t.Run("specific verbs allowed", func(t *testing.T) {
		content := `---
type: reference
title: Specific Instructions
description: Test specific instructions
---

# Specific Instructions

Optimize the database query by adding index on user_id column (target: <50ms).

Improve error handling by adding try-catch for FileNotFoundError and PermissionError.
`
		tmpfile, err := os.CreateTemp(t.TempDir(), "test-*.ai.md")
		require.NoError(t, err)
		defer os.Remove(tmpfile.Name())

		_, err = tmpfile.WriteString(content)
		require.NoError(t, err)
		tmpfile.Close()

		errors, err := ValidateFile(tmpfile.Name())
		require.NoError(t, err)

		vagueErrors := filterErrorsByType(errors, ErrorTypeVagueVerb)
		assert.Equal(t, 0, len(vagueErrors), "Specific instructions should not trigger vague verb errors")
	})
}

// TestMissingExampleDetection tests missing example detection.
func TestMissingExampleDetection(t *testing.T) {
	t.Run("missing examples detected", func(t *testing.T) {
		errors, err := ValidateFile(getFixturePath("missing_examples.ai.md"))
		require.NoError(t, err)

		exampleErrors := filterErrorsByType(errors, ErrorTypeMissingExample)
		assert.Greater(t, len(exampleErrors), 0, "Should detect missing examples")
	})

	t.Run("examples present", func(t *testing.T) {
		errors, err := ValidateFile(getFixturePath("valid_engram.ai.md"))
		require.NoError(t, err)

		exampleErrors := filterErrorsByType(errors, ErrorTypeMissingExample)
		assert.Equal(t, 0, len(exampleErrors), "Should not flag principles with examples")
	})
}

// TestMissingConstraintDetection tests missing constraint detection.
func TestMissingConstraintDetection(t *testing.T) {
	t.Run("missing constraints detected", func(t *testing.T) {
		errors, err := ValidateFile(getFixturePath("missing_constraints.ai.md"))
		require.NoError(t, err)

		constraintErrors := filterErrorsByType(errors, ErrorTypeMissingConstraints)
		assert.Greater(t, len(constraintErrors), 0, "Should detect missing constraints")

		// Check for specific task verbs
		messages := getErrorMessages(constraintErrors)
		taskVerbs := []string{"implement", "create", "build", "generate"}
		for _, verb := range taskVerbs {
			assert.True(t, containsSubstring(messages, verb), "Should detect '%s'", verb)
		}
	})

	t.Run("constraints present", func(t *testing.T) {
		errors, err := ValidateFile(getFixturePath("valid_engram.ai.md"))
		require.NoError(t, err)

		constraintErrors := filterErrorsByType(errors, ErrorTypeMissingConstraints)
		assert.Equal(t, 0, len(constraintErrors), "Should not flag tasks with constraints")
	})
}

// TestFileValidation tests file-level validation.
func TestFileValidation(t *testing.T) {
	t.Run("valid file returns no critical errors", func(t *testing.T) {
		errors, err := ValidateFile(getFixturePath("valid_engram.ai.md"))
		require.NoError(t, err)

		criticalErrors := []string{
			ErrorTypeMissingFrontmatter,
			ErrorTypeInvalidFrontmatter,
			ErrorTypeFileError,
		}

		for _, e := range errors {
			assert.NotContains(t, criticalErrors, e.ErrorType)
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		errors, err := ValidateFile("/nonexistent/file.ai.md")
		require.NoError(t, err)

		found := false
		for _, e := range errors {
			if e.ErrorType == ErrorTypeFileError {
				found = true
				break
			}
		}
		assert.True(t, found, "Should return file_error for nonexistent file")
	})

	t.Run("line numbers reported", func(t *testing.T) {
		errors, err := ValidateFile(getFixturePath("context_references.ai.md"))
		require.NoError(t, err)

		contextErrors := filterErrorsByType(errors, ErrorTypeContextReference)
		assert.Greater(t, len(contextErrors), 0)

		for _, e := range contextErrors {
			assert.Greater(t, e.Line, 0, "Should report line numbers")
		}
	})
}

// TestDirectoryValidation tests directory-level validation.
func TestDirectoryValidation(t *testing.T) {
	t.Run("validate directory", func(t *testing.T) {
		results, err := ValidateDirectory(testdataDir)
		require.NoError(t, err)

		assert.Greater(t, len(results), 0, "Should find .ai.md files")

		for path := range results {
			assert.True(t, strings.HasSuffix(path, ".ai.md"), "Should only validate .ai.md files")
		}
	})

	t.Run("directory results structure", func(t *testing.T) {
		results, err := ValidateDirectory(testdataDir)
		require.NoError(t, err)

		for filePath, errors := range results {
			assert.NotEmpty(t, filePath)
			assert.NotNil(t, errors)

			for _, e := range errors {
				assert.NotEmpty(t, e.ErrorType)
			}
		}
	})
}

// TestValidatorClass tests EngramValidator class.
func TestValidatorClass(t *testing.T) {
	t.Run("validator initialization", func(t *testing.T) {
		validator := New(getFixturePath("valid_engram.ai.md"))
		assert.NotNil(t, validator)
		assert.Equal(t, getFixturePath("valid_engram.ai.md"), validator.filePath)
		assert.Equal(t, 0, len(validator.errors))
	})

	t.Run("validator validate method", func(t *testing.T) {
		validator := New(getFixturePath("valid_engram.ai.md"))
		errors := validator.Validate()
		assert.NotNil(t, errors)
	})

	t.Run("multiple validations", func(t *testing.T) {
		validator := New(getFixturePath("context_references.ai.md"))
		errors1 := validator.Validate()
		errors2 := validator.Validate()
		// Errors should be reset between runs
		assert.Equal(t, len(errors1), len(errors2))
	})
}

// TestEdgeCases tests edge cases and boundary conditions.
func TestEdgeCases(t *testing.T) {
	t.Run("empty file", func(t *testing.T) {
		tmpfile, err := os.CreateTemp(t.TempDir(), "test-*.ai.md")
		require.NoError(t, err)
		defer os.Remove(tmpfile.Name())

		tmpfile.Close()

		errors, err := ValidateFile(tmpfile.Name())
		require.NoError(t, err)

		found := false
		for _, e := range errors {
			if e.ErrorType == ErrorTypeMissingFrontmatter {
				found = true
				break
			}
		}
		assert.True(t, found, "Empty file should fail validation")
	})

	t.Run("malformed yaml", func(t *testing.T) {
		content := "---\ntype: reference\ntitle: [unclosed\n---\n# Test"
		tmpfile, err := os.CreateTemp(t.TempDir(), "test-*.ai.md")
		require.NoError(t, err)
		defer os.Remove(tmpfile.Name())

		_, err = tmpfile.WriteString(content)
		require.NoError(t, err)
		tmpfile.Close()

		errors, err := ValidateFile(tmpfile.Name())
		require.NoError(t, err)

		found := false
		for _, e := range errors {
			if e.ErrorType == ErrorTypeInvalidFrontmatter {
				found = true
				break
			}
		}
		assert.True(t, found, "Malformed YAML should be detected")
	})

	t.Run("unicode content", func(t *testing.T) {
		content := "---\ntype: reference\ntitle: Unicode Test 你好\ndescription: Test\n---\n# Test 🎉"
		tmpfile, err := os.CreateTemp(t.TempDir(), "test-*.ai.md")
		require.NoError(t, err)
		defer os.Remove(tmpfile.Name())

		_, err = tmpfile.WriteString(content)
		require.NoError(t, err)
		tmpfile.Close()

		errors, err := ValidateFile(tmpfile.Name())
		require.NoError(t, err)

		// Should not crash, may or may not have errors
		assert.NotNil(t, errors)
	})
}

// TestAllValidTypes tests all valid frontmatter types.
func TestAllValidTypes(t *testing.T) {
	types := []string{"reference", "template", "workflow", "guide"}

	for _, validType := range types {
		t.Run(validType, func(t *testing.T) {
			content := "---\ntype: " + validType + "\ntitle: Test\ndescription: Test\n---\n# Test"
			tmpfile, err := os.CreateTemp(t.TempDir(), "test-*.ai.md")
			require.NoError(t, err)
			defer os.Remove(tmpfile.Name())

			_, err = tmpfile.WriteString(content)
			require.NoError(t, err)
			tmpfile.Close()

			errors, err := ValidateFile(tmpfile.Name())
			require.NoError(t, err)

			// Should not have type errors
			for _, e := range errors {
				assert.NotEqual(t, ErrorTypeInvalidType, e.ErrorType)
			}
		})
	}
}

// TestContextReferencePatterns tests all context reference patterns.
func TestContextReferencePatterns(t *testing.T) {
	patterns := []string{
		"see above for more details",
		"as mentioned earlier in the document",
		"discussed previously in this guide",
		"refer to the section above",
		"previous section covers this",
		"earlier example shows how",
		"as discussed in the introduction",
		"the pattern mentioned above is useful",
	}

	for _, pattern := range patterns {
		t.Run(pattern, func(t *testing.T) {
			content := "---\ntype: reference\ntitle: Test\ndescription: Test\n---\n# Test\n\n" + pattern
			tmpfile, err := os.CreateTemp(t.TempDir(), "test-*.ai.md")
			require.NoError(t, err)
			defer os.Remove(tmpfile.Name())

			_, err = tmpfile.WriteString(content)
			require.NoError(t, err)
			tmpfile.Close()

			errors, err := ValidateFile(tmpfile.Name())
			require.NoError(t, err)

			found := false
			for _, e := range errors {
				if e.ErrorType == ErrorTypeContextReference {
					found = true
					break
				}
			}
			assert.True(t, found, "Should detect context reference pattern: %s", pattern)
		})
	}
}

// TestVagueVerbsWithCriteria tests vague verbs with measurable criteria.
func TestVagueVerbsWithCriteria(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		hasError bool
	}{
		{
			name:     "optimize with percentage",
			content:  "Optimize the query to achieve 80% performance improvement",
			hasError: false,
		},
		{
			name:     "improve with time bound",
			content:  "Improve response time to <100ms",
			hasError: false,
		},
		{
			name:     "fix with specific action",
			content:  "Fix error handling by adding try-catch",
			hasError: false,
		},
		{
			name:     "enhance with target",
			content:  "Enhance logging with stack traces",
			hasError: false,
		},
		{
			name:     "update with must constraint",
			content:  "Update validation must include email format check",
			hasError: false,
		},
		{
			name:     "refactor with should constraint",
			content:  "Refactor code should be backward compatible",
			hasError: false,
		},
		{
			name:     "optimize without criteria",
			content:  "Optimize the database",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := "---\ntype: reference\ntitle: Test\ndescription: Test\n---\n# Test\n\n" + tt.content
			tmpfile, err := os.CreateTemp(t.TempDir(), "test-*.ai.md")
			require.NoError(t, err)
			defer os.Remove(tmpfile.Name())

			_, err = tmpfile.WriteString(content)
			require.NoError(t, err)
			tmpfile.Close()

			errors, err := ValidateFile(tmpfile.Name())
			require.NoError(t, err)

			vagueErrors := filterErrorsByType(errors, ErrorTypeVagueVerb)
			if tt.hasError {
				assert.Greater(t, len(vagueErrors), 0, "Should detect vague verb without criteria")
			} else {
				assert.Equal(t, 0, len(vagueErrors), "Should not flag verb with criteria")
			}
		})
	}
}

// TestPrincipleWithExamples tests principle detection with various example formats.
func TestPrincipleWithExamples(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		hasError bool
	}{
		{
			name:     "principle with code block",
			content:  "## Principle: Test\n\nSome text\n\n```\ncode example\n```",
			hasError: false,
		},
		{
			name:     "principle with example section",
			content:  "## Guideline: Test\n\nSome text\n\nExample:\nThis is an example",
			hasError: false,
		},
		{
			name:     "principle with good example",
			content:  "## Pattern: Test\n\nSome text\n\nGood example: use this",
			hasError: false,
		},
		{
			name:     "principle with bad example",
			content:  "## Rule: Test\n\nSome text\n\nBad example: avoid this",
			hasError: false,
		},
		{
			name:     "principle without example",
			content:  "## Best Practice: Test\n\nSome text without any examples at all",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := "---\ntype: reference\ntitle: Test\ndescription: Test\n---\n\n" + tt.content
			tmpfile, err := os.CreateTemp(t.TempDir(), "test-*.ai.md")
			require.NoError(t, err)
			defer os.Remove(tmpfile.Name())

			_, err = tmpfile.WriteString(content)
			require.NoError(t, err)
			tmpfile.Close()

			errors, err := ValidateFile(tmpfile.Name())
			require.NoError(t, err)

			exampleErrors := filterErrorsByType(errors, ErrorTypeMissingExample)
			if tt.hasError {
				assert.Greater(t, len(exampleErrors), 0, "Should detect missing example")
			} else {
				assert.Equal(t, 0, len(exampleErrors), "Should not flag principle with example")
			}
		})
	}
}

// TestTasksWithConstraints tests task detection with various constraint formats.
func TestTasksWithConstraints(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		hasError bool
	}{
		{
			name:     "task with token budget",
			content:  "Implement authentication\n\nToken budget: 2000 tokens",
			hasError: false,
		},
		{
			name:     "task with file limit",
			content:  "Create API\n\nFile limit: 3 files",
			hasError: false,
		},
		{
			name:     "task with scope",
			content:  "Build dashboard\n\nScope: frontend only",
			hasError: false,
		},
		{
			name:     "task with time bound",
			content:  "Generate tests\n\nTime bound: complete in single response",
			hasError: false,
		},
		{
			name:     "task with constraints section",
			content:  "Write documentation\n\nConstraints:\n- Max 5 files\n- Under 3000 tokens",
			hasError: false,
		},
		{
			name:     "task without constraints",
			content:  "Implement authentication system",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := "---\ntype: reference\ntitle: Test\ndescription: Test\n---\n\n" + tt.content
			tmpfile, err := os.CreateTemp(t.TempDir(), "test-*.ai.md")
			require.NoError(t, err)
			defer os.Remove(tmpfile.Name())

			_, err = tmpfile.WriteString(content)
			require.NoError(t, err)
			tmpfile.Close()

			errors, err := ValidateFile(tmpfile.Name())
			require.NoError(t, err)

			constraintErrors := filterErrorsByType(errors, ErrorTypeMissingConstraints)
			if tt.hasError {
				assert.Greater(t, len(constraintErrors), 0, "Should detect missing constraints")
			} else {
				assert.Equal(t, 0, len(constraintErrors), "Should not flag task with constraints")
			}
		})
	}
}

// Helper functions

func filterErrorsByType(errors []EngramValidationError, errorType string) []EngramValidationError {
	var filtered []EngramValidationError
	for _, e := range errors {
		if e.ErrorType == errorType {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

func getErrorMessages(errors []EngramValidationError) []string {
	var messages []string
	for _, e := range errors {
		messages = append(messages, strings.ToLower(e.Message))
	}
	return messages
}

func containsSubstring(messages []string, substr string) bool {
	for _, msg := range messages {
		if strings.Contains(msg, strings.ToLower(substr)) {
			return true
		}
	}
	return false
}
