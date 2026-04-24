package beads

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractFileRefs(t *testing.T) {
	description := "Modified `internal/a2a/protocol/status.go` and `internal/a2a/config/config.go` for the migration."
	refs := ExtractFileRefs(description)
	assert.Contains(t, refs, "internal/a2a/protocol/status.go")
	assert.Contains(t, refs, "internal/a2a/config/config.go")
}

func TestExtractFileRefs_SkipsURLs(t *testing.T) {
	description := "See https://example.com/path/file.go for details"
	refs := ExtractFileRefs(description)
	assert.Empty(t, refs)
}

func TestExtractFileRefs_SkipsExcludedExtensions(t *testing.T) {
	description := "Visit `example.com` and `foo.org`"
	refs := ExtractFileRefs(description)
	assert.Empty(t, refs)
}

func TestDescriptionValidator_ValidateDescription(t *testing.T) {
	dir := t.TempDir()

	// Create a test file
	err := os.MkdirAll(filepath.Join(dir, "internal", "a2a"), 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "internal", "a2a", "cards.go"), []byte("package a2a"), 0644)
	require.NoError(t, err)

	v := NewDescriptionValidator(dir)

	// Valid: references existing file
	result := v.ValidateDescription("Updated `internal/a2a/cards.go` for card generation")
	assert.True(t, result.Valid)
	assert.Empty(t, result.InvalidFileRefs)

	// Invalid: references non-existent file
	result = v.ValidateDescription("Updated `internal/a2a/nonexistent.go` for something")
	assert.False(t, result.Valid)
	assert.Len(t, result.InvalidFileRefs, 1)
	assert.Equal(t, "internal/a2a/nonexistent.go", result.InvalidFileRefs[0].Path)
}

func TestFormatValidationReport(t *testing.T) {
	// Pass
	result := ValidationResult{Valid: true}
	report := FormatValidationReport(result)
	assert.Contains(t, report, "passed")

	// Fail
	result = ValidationResult{
		Valid: false,
		InvalidFileRefs: []InvalidFileRef{
			{Path: "foo.go", Reason: "file not found in repository"},
		},
		Warnings: []string{"1 of 1 file references could not be verified"},
	}
	report = FormatValidationReport(result)
	assert.Contains(t, report, "failed")
	assert.Contains(t, report, "foo.go")
}
