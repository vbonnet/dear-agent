package validator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test fixtures

const validAiMd = `---
schema: ai-content/v1
type: howto
status: active
created: 2024-01-15
tokens: 34
---

# Test Content

This is a test document with exactly 38 tokens according to tiktoken cl100k_base encoding.
We need enough content to test token counting accurately.
`

const missingFieldsAiMd = `---
schema: ai-content/v1
status: active
created: 2024-01-15
---

# Test Content

Missing required fields.
`

const tokenMismatchAiMd = `---
schema: ai-content/v1
type: howto
status: active
created: 2024-01-15
tokens: 999
---

# Test Content

Token count is wrong.
`

const coreFileValid = `---
schema: ai-content/v1
type: core
status: active
created: 2024-01-15
tokens: 100
---

# Core Guidance

## NEVER

Never do this.

## ALWAYS

Always do that.

## REMINDER

Remember this.
`

const coreFileMissingSections = `---
schema: ai-content/v1
type: core
status: active
created: 2024-01-15
tokens: 50
---

# Core Guidance

## NEVER

Never do this.

Missing ALWAYS and REMINDER sections.
`

var largeFileNoCompanion = `---
schema: ai-content/v1
type: howto
status: active
created: 2024-01-15
tokens: 1500
---

# Large File

This is a large file with many tokens. It should have companion files but doesn't.
` + strings.Repeat("More content here. ", 100)

const emptyContent = `---
schema: ai-content/v1
type: howto
status: active
created: 2024-01-15
tokens: 0
---
`

// setupTestDir creates a temporary directory with test files.
func setupTestDir(t *testing.T) string {
	tmpDir, err := os.MkdirTemp("", "validator-test-*")
	require.NoError(t, err)

	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})

	return tmpDir
}

// writeTestFile writes content to a file in the test directory.
func writeTestFile(t *testing.T, dir, filename, content string) string {
	// Create subdirectories if needed
	fullPath := filepath.Join(dir, filename)
	dirPath := filepath.Dir(fullPath)

	if err := os.MkdirAll(dirPath, 0755); err != nil {
		t.Fatalf("Failed to create directory %s: %v", dirPath, err)
	}

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file %s: %v", fullPath, err)
	}

	return fullPath
}

// TestNewContentValidator tests validator creation.
func TestNewContentValidator(t *testing.T) {
	tmpDir := setupTestDir(t)

	tests := []struct {
		name      string
		dir       string
		autoFix   bool
		wantError bool
	}{
		{
			name:      "valid directory",
			dir:       tmpDir,
			autoFix:   false,
			wantError: false,
		},
		{
			name:      "valid directory with autofix",
			dir:       tmpDir,
			autoFix:   true,
			wantError: false,
		},
		{
			name:      "nonexistent directory",
			dir:       "/nonexistent/path",
			autoFix:   false,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := NewContentValidator(tt.dir, tt.autoFix)

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, v)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, v)
				assert.Equal(t, tt.autoFix, v.autoFix)
			}
		})
	}
}

// TestFrontmatterCompleteness tests frontmatter field validation.
func TestFrontmatterCompleteness(t *testing.T) {
	tmpDir := setupTestDir(t)

	tests := []struct {
		name       string
		content    string
		wantErrors int
	}{
		{
			name:       "all fields present",
			content:    validAiMd,
			wantErrors: 0,
		},
		{
			name:       "missing required fields",
			content:    missingFieldsAiMd,
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := writeTestFile(t, tmpDir, "test.ai.md", tt.content)

			v, err := NewContentValidator(tmpDir, false)
			require.NoError(t, err)

			err = v.ValidateFile(file)
			assert.NoError(t, err)

			errors := v.GetErrors()
			assert.Len(t, errors, tt.wantErrors)

			if tt.wantErrors > 0 {
				assert.Contains(t, errors[0].Message, "Missing required fields")
				assert.Equal(t, "frontmatter", errors[0].Check)
			}
		})
	}
}

// TestTokenAccuracy tests token count validation.
func TestTokenAccuracy(t *testing.T) {
	tmpDir := setupTestDir(t)

	tests := []struct {
		name       string
		content    string
		autoFix    bool
		wantErrors int
		wantFixes  int
	}{
		{
			name:       "exact match",
			content:    validAiMd,
			autoFix:    false,
			wantErrors: 0,
			wantFixes:  0,
		},
		{
			name:       "mismatch without auto-fix",
			content:    tokenMismatchAiMd,
			autoFix:    false,
			wantErrors: 1,
			wantFixes:  0,
		},
		{
			name:       "mismatch with auto-fix",
			content:    tokenMismatchAiMd,
			autoFix:    true,
			wantErrors: 0,
			wantFixes:  1,
		},
		{
			name:       "empty content",
			content:    emptyContent,
			autoFix:    false,
			wantErrors: 0, // Empty content has 0 tokens which matches declaration
			wantFixes:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := writeTestFile(t, tmpDir, "test.ai.md", tt.content)

			v, err := NewContentValidator(tmpDir, tt.autoFix)
			require.NoError(t, err)

			err = v.ValidateFile(file)
			assert.NoError(t, err)

			errors := v.GetErrors()
			assert.Len(t, errors, tt.wantErrors)

			fixes := v.GetFixesApplied()
			assert.Len(t, fixes, tt.wantFixes)

			if tt.wantErrors > 0 && !tt.autoFix && len(errors) > 0 {
				assert.Contains(t, errors[0].Message, "Token count mismatch")
				assert.Equal(t, "tokens", errors[0].Check)
			}

			if tt.wantFixes > 0 && len(fixes) > 0 {
				assert.Contains(t, fixes[0], "Auto-updated")
				assert.Contains(t, fixes[0], "tokens")
			}
		})
	}
}

// TestTokenAccuracyAutoFix verifies auto-fix updates files correctly.
func TestTokenAccuracyAutoFix(t *testing.T) {
	tmpDir := setupTestDir(t)
	file := writeTestFile(t, tmpDir, "test.ai.md", tokenMismatchAiMd)

	v, err := NewContentValidator(tmpDir, true)
	require.NoError(t, err)

	err = v.ValidateFile(file)
	assert.NoError(t, err)

	// Should have applied a fix
	fixes := v.GetFixesApplied()
	assert.Len(t, fixes, 1)

	// Read file and verify token count was updated
	content, err := os.ReadFile(file)
	require.NoError(t, err)

	assert.NotContains(t, string(content), "tokens: 999")
	assert.Contains(t, string(content), "tokens:")
}

// TestCoreStructure tests core file structure validation.
func TestCoreStructure(t *testing.T) {
	tmpDir := setupTestDir(t)

	// Create core-files.txt
	validationDir := filepath.Join(filepath.Dir(tmpDir), "validation")
	require.NoError(t, os.MkdirAll(validationDir, 0755))

	coreFilesList := filepath.Join(validationDir, "core-files.txt")
	require.NoError(t, os.WriteFile(coreFilesList, []byte("core.ai.md\n"), 0644))

	tests := []struct {
		name       string
		filename   string
		content    string
		wantErrors int
	}{
		{
			name:       "all sections present",
			filename:   "core.ai.md",
			content:    coreFileValid,
			wantErrors: 0,
		},
		{
			name:       "missing sections",
			filename:   "core.ai.md",
			content:    coreFileMissingSections,
			wantErrors: 1,
		},
		{
			name:       "non-core file skipped",
			filename:   "regular.ai.md",
			content:    coreFileMissingSections,
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := writeTestFile(t, tmpDir, tt.filename, tt.content)

			v, err := NewContentValidator(tmpDir, false)
			require.NoError(t, err)

			err = v.ValidateFile(file)
			assert.NoError(t, err)

			errors := v.GetErrors()
			structureErrors := 0
			for _, e := range errors {
				if e.Check == "structure" {
					structureErrors++
				}
			}

			assert.Equal(t, tt.wantErrors, structureErrors)

			if tt.wantErrors > 0 {
				found := false
				for _, e := range errors {
					if e.Check == "structure" {
						assert.Contains(t, e.Message, "missing required sections")
						found = true
					}
				}
				assert.True(t, found)
			}
		})
	}
}

// TestProgressiveDisclosure tests companion file validation.
func TestProgressiveDisclosure(t *testing.T) {
	tmpDir := setupTestDir(t)

	tests := []struct {
		name         string
		content      string
		createMD     bool
		createWhy    bool
		referenceMD  bool
		wantWarnings int
	}{
		{
			name:         "small file - no warning",
			content:      validAiMd, // 50 tokens
			createMD:     false,
			createWhy:    false,
			wantWarnings: 0,
		},
		{
			name:         "large file - no companions",
			content:      largeFileNoCompanion,
			createMD:     false,
			createWhy:    false,
			wantWarnings: 1,
		},
		{
			name:         "large file - has .md companion",
			content:      largeFileNoCompanion + "\nSee test.md for details.",
			createMD:     true,
			createWhy:    false,
			referenceMD:  true,
			wantWarnings: 0,
		},
		{
			name:         "large file - companion not referenced",
			content:      largeFileNoCompanion,
			createMD:     true,
			createWhy:    false,
			referenceMD:  false,
			wantWarnings: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := writeTestFile(t, tmpDir, "test.ai.md", tt.content)

			if tt.createMD {
				writeTestFile(t, tmpDir, "test.md", "# Companion file")
			}

			if tt.createWhy {
				writeTestFile(t, tmpDir, "test.why.md", "# Why file")
			}

			v, err := NewContentValidator(tmpDir, false)
			require.NoError(t, err)

			err = v.ValidateFile(file)
			assert.NoError(t, err)

			warnings := v.GetWarnings()
			disclosureWarnings := 0
			for _, w := range warnings {
				if w.Check == "progressive-disclosure" {
					disclosureWarnings++
				}
			}

			assert.Equal(t, tt.wantWarnings, disclosureWarnings)
		})
	}
}

// TestTokenBudget tests token budget enforcement.
func TestTokenBudget(t *testing.T) {
	tmpDir := setupTestDir(t)

	// Create token-budgets.json
	validationDir := filepath.Join(filepath.Dir(tmpDir), "validation")
	require.NoError(t, os.MkdirAll(validationDir, 0755))

	budgetsFile := filepath.Join(validationDir, "token-budgets.json")
	budgetsJSON := `{
  "budgets": {
    "howto": 100,
    "reference": 500
  }
}`
	require.NoError(t, os.WriteFile(budgetsFile, []byte(budgetsJSON), 0644))

	tests := []struct {
		name         string
		content      string
		wantWarnings int
	}{
		{
			name:         "within budget",
			content:      validAiMd, // 50 tokens, budget 100
			wantWarnings: 0,
		},
		{
			name: "over budget",
			content: `---
schema: ai-content/v1
type: howto
status: active
created: 2024-01-15
tokens: 150
---

# Large Content
` + strings.Repeat("More content. ", 50),
			wantWarnings: 1,
		},
		{
			name: "no budget for type",
			content: `---
schema: ai-content/v1
type: tutorial
status: active
created: 2024-01-15
tokens: 1000
---

# Tutorial
`,
			wantWarnings: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := writeTestFile(t, tmpDir, "test.ai.md", tt.content)

			v, err := NewContentValidator(tmpDir, false)
			require.NoError(t, err)

			err = v.ValidateFile(file)
			assert.NoError(t, err)

			warnings := v.GetWarnings()
			budgetWarnings := 0
			for _, w := range warnings {
				if w.Check == "budget" {
					budgetWarnings++
				}
			}

			assert.Equal(t, tt.wantWarnings, budgetWarnings)

			if tt.wantWarnings > 0 {
				found := false
				for _, w := range warnings {
					if w.Check == "budget" {
						assert.Contains(t, w.Message, "Exceeds")
						assert.Contains(t, w.Message, "budget")
						found = true
					}
				}
				assert.True(t, found)
			}
		})
	}
}

// TestValidateAll tests batch validation.
func TestValidateAll(t *testing.T) {
	tmpDir := setupTestDir(t)

	// Create multiple test files
	writeTestFile(t, tmpDir, "valid.ai.md", validAiMd)
	writeTestFile(t, tmpDir, "invalid.ai.md", missingFieldsAiMd)
	writeTestFile(t, tmpDir, "mismatch.ai.md", tokenMismatchAiMd)

	v, err := NewContentValidator(tmpDir, false)
	require.NoError(t, err)

	errorCount, warningCount, err := v.ValidateAll(nil)
	assert.NoError(t, err)

	// Should find errors in invalid.ai.md and mismatch.ai.md
	assert.Greater(t, errorCount, 0)
	assert.GreaterOrEqual(t, errorCount+warningCount, 2)

	errors := v.GetErrors()
	assert.Greater(t, len(errors), 0)
}

// TestValidateSpecificFiles tests validating specific files.
func TestValidateSpecificFiles(t *testing.T) {
	tmpDir := setupTestDir(t)

	validFile := writeTestFile(t, tmpDir, "valid.ai.md", validAiMd)
	invalidFile := writeTestFile(t, tmpDir, "invalid.ai.md", missingFieldsAiMd)

	v, err := NewContentValidator(tmpDir, false)
	require.NoError(t, err)

	// Validate only the valid file
	errorCount, _, err := v.ValidateAll([]string{validFile})
	assert.NoError(t, err)
	assert.Equal(t, 0, errorCount)

	// Reset validator
	v, err = NewContentValidator(tmpDir, false)
	require.NoError(t, err)

	// Validate only the invalid file
	errorCount, _, err = v.ValidateAll([]string{invalidFile})
	assert.NoError(t, err)
	assert.Greater(t, errorCount, 0)
}

// TestInvalidFrontmatter tests handling of invalid frontmatter.
func TestInvalidFrontmatter(t *testing.T) {
	tmpDir := setupTestDir(t)

	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "no frontmatter",
			content: "# Just Content\n\nNo frontmatter here.",
		},
		{
			name: "invalid YAML",
			content: `---
invalid: [yaml syntax
---

# Content
`,
		},
		{
			name: "missing closing delimiter",
			content: `---
schema: ai-content/v1
type: howto

# Content without closing ---
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := writeTestFile(t, tmpDir, "test.ai.md", tt.content)

			v, err := NewContentValidator(tmpDir, false)
			require.NoError(t, err)

			err = v.ValidateFile(file)
			assert.NoError(t, err) // Validate doesn't return errors, stores them

			errors := v.GetErrors()
			assert.Greater(t, len(errors), 0)
			assert.Equal(t, "parsing", errors[0].Check)
		})
	}
}

// TestUnicodeContent tests token counting with unicode.
func TestUnicodeContent(t *testing.T) {
	tmpDir := setupTestDir(t)

	unicodeContent := `---
schema: ai-content/v1
type: howto
status: active
created: 2024-01-15
tokens: 20
---

# Unicode Test

こんにちは世界 Hello 你好 مرحبا
`

	file := writeTestFile(t, tmpDir, "unicode.ai.md", unicodeContent)

	v, err := NewContentValidator(tmpDir, false)
	require.NoError(t, err)

	err = v.ValidateFile(file)
	assert.NoError(t, err)

	// Should detect token mismatch (unicode counts differently)
	errors := v.GetErrors()
	assert.Greater(t, len(errors), 0)
}

// TestContentValidationErrorString tests error formatting.
func TestContentValidationErrorString(t *testing.T) {
	tests := []struct {
		name     string
		err      ContentValidationError
		contains []string
	}{
		{
			name: "error severity",
			err: ContentValidationError{
				Filepath: "test.ai.md",
				Check:    "tokens",
				Severity: "error",
				Message:  "Token count mismatch",
			},
			contains: []string{"❌", "[tokens]", "test.ai.md", "Token count mismatch"},
		},
		{
			name: "warning severity",
			err: ContentValidationError{
				Filepath: "test.ai.md",
				Check:    "bloat",
				Severity: "warning",
				Message:  "Token count increased",
			},
			contains: []string{"⚠️", "[bloat]", "test.ai.md", "Token count increased"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			str := tt.err.String()
			for _, expected := range tt.contains {
				assert.Contains(t, str, expected)
			}
		})
	}
}

// TestGetRelativePath tests path resolution.
func TestGetRelativePath(t *testing.T) {
	tmpDir := setupTestDir(t)

	v, err := NewContentValidator(tmpDir, false)
	require.NoError(t, err)

	tests := []struct {
		name     string
		absPath  string
		expected string
	}{
		{
			name:     "file in root",
			absPath:  filepath.Join(tmpDir, "test.ai.md"),
			expected: "test.ai.md",
		},
		{
			name:     "file in subdirectory",
			absPath:  filepath.Join(tmpDir, "subdir", "test.ai.md"),
			expected: filepath.Join("subdir", "test.ai.md"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			relPath, err := v.getRelativePath(tt.absPath)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, relPath)
		})
	}
}

// TestEmptyFileList tests validation with no files.
func TestEmptyFileList(t *testing.T) {
	tmpDir := setupTestDir(t)

	v, err := NewContentValidator(tmpDir, false)
	require.NoError(t, err)

	// Empty directory - no files to validate
	errorCount, warningCount, err := v.ValidateAll(nil)
	assert.NoError(t, err)
	assert.Equal(t, 0, errorCount)
	assert.Equal(t, 0, warningCount)
}

// TestInvalidTokenType tests handling of non-integer token values.
func TestInvalidTokenType(t *testing.T) {
	tmpDir := setupTestDir(t)

	invalidTokenType := `---
schema: ai-content/v1
type: howto
status: active
created: 2024-01-15
tokens: "not a number"
---

# Test
`

	file := writeTestFile(t, tmpDir, "test.ai.md", invalidTokenType)

	v, err := NewContentValidator(tmpDir, false)
	require.NoError(t, err)

	err = v.ValidateFile(file)
	assert.NoError(t, err)

	errors := v.GetErrors()
	assert.Greater(t, len(errors), 0)
	foundTokenError := false
	for _, e := range errors {
		if e.Check == "tokens" && strings.Contains(e.Message, "Invalid token count type") {
			foundTokenError = true
		}
	}
	assert.True(t, foundTokenError)
}
