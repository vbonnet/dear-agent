package engram_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vbonnet/dear-agent/pkg/engram"
)

func TestValidateWhyFile_Valid(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create .ai.md file
	aiMdPath := filepath.Join(tmpDir, "oauth-pattern.ai.md")
	err := os.WriteFile(aiMdPath, []byte("# OAuth Pattern"), 0644)
	require.NoError(t, err)

	// Create valid .why.md file
	whyMdPath := filepath.Join(tmpDir, "oauth-pattern.why.md")
	whyContent := `---
rationale_for: "oauth-pattern"
decision_date: "2025-01-20"
decided_by: "security-team"
review_cycle: "quarterly"
status: "active"
superseded_by: ""
---

## Problem Statement
Test problem
`
	err = os.WriteFile(whyMdPath, []byte(whyContent), 0644)
	require.NoError(t, err)

	// Validate
	err = engram.ValidateWhyFile(aiMdPath)
	assert.NoError(t, err)
}

func TestValidateWhyFile_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	aiMdPath := filepath.Join(tmpDir, "oauth-pattern.ai.md")
	err := os.WriteFile(aiMdPath, []byte("# OAuth Pattern"), 0644)
	require.NoError(t, err)

	// No .why.md file created

	err = engram.ValidateWhyFile(aiMdPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing .why.md companion")
}

func TestValidateWhyFile_InvalidStatus(t *testing.T) {
	tmpDir := t.TempDir()

	aiMdPath := filepath.Join(tmpDir, "oauth-pattern.ai.md")
	err := os.WriteFile(aiMdPath, []byte("# OAuth Pattern"), 0644)
	require.NoError(t, err)

	whyMdPath := filepath.Join(tmpDir, "oauth-pattern.why.md")
	whyContent := `---
rationale_for: "oauth-pattern"
decision_date: "2025-01-20"
decided_by: "security-team"
review_cycle: "quarterly"
status: "invalid-status"
superseded_by: ""
---

## Problem Statement
Test
`
	err = os.WriteFile(whyMdPath, []byte(whyContent), 0644)
	require.NoError(t, err)

	err = engram.ValidateWhyFile(aiMdPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status")
}

func TestValidateWhyFile_SupersededMissingBy(t *testing.T) {
	tmpDir := t.TempDir()

	aiMdPath := filepath.Join(tmpDir, "oauth-pattern.ai.md")
	err := os.WriteFile(aiMdPath, []byte("# OAuth Pattern"), 0644)
	require.NoError(t, err)

	whyMdPath := filepath.Join(tmpDir, "oauth-pattern.why.md")
	whyContent := `---
rationale_for: "oauth-pattern"
decision_date: "2025-01-20"
decided_by: "security-team"
review_cycle: "quarterly"
status: "superseded"
superseded_by: ""
---

## Problem Statement
Test
`
	err = os.WriteFile(whyMdPath, []byte(whyContent), 0644)
	require.NoError(t, err)

	err = engram.ValidateWhyFile(aiMdPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "superseded_by field")
}

func TestValidateWhyFile_RationaleMismatch(t *testing.T) {
	tmpDir := t.TempDir()

	aiMdPath := filepath.Join(tmpDir, "oauth-pattern.ai.md")
	err := os.WriteFile(aiMdPath, []byte("# OAuth Pattern"), 0644)
	require.NoError(t, err)

	whyMdPath := filepath.Join(tmpDir, "oauth-pattern.why.md")
	whyContent := `---
rationale_for: "different-pattern"
decision_date: "2025-01-20"
decided_by: "security-team"
review_cycle: "quarterly"
status: "active"
superseded_by: ""
---

## Problem Statement
Test
`
	err = os.WriteFile(whyMdPath, []byte(whyContent), 0644)
	require.NoError(t, err)

	err = engram.ValidateWhyFile(aiMdPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rationale_for mismatch")
}

func TestValidateWhyFile_InvalidDate(t *testing.T) {
	tmpDir := t.TempDir()

	aiMdPath := filepath.Join(tmpDir, "oauth-pattern.ai.md")
	err := os.WriteFile(aiMdPath, []byte("# OAuth Pattern"), 0644)
	require.NoError(t, err)

	whyMdPath := filepath.Join(tmpDir, "oauth-pattern.why.md")
	whyContent := `---
rationale_for: "oauth-pattern"
decision_date: "2025-13-45"
decided_by: "security-team"
review_cycle: "quarterly"
status: "active"
superseded_by: ""
---

## Problem Statement
Test
`
	err = os.WriteFile(whyMdPath, []byte(whyContent), 0644)
	require.NoError(t, err)

	err = engram.ValidateWhyFile(aiMdPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid decision_date format")
}

func TestValidateWhyFile_InvalidReviewCycle(t *testing.T) {
	tmpDir := t.TempDir()

	aiMdPath := filepath.Join(tmpDir, "oauth-pattern.ai.md")
	err := os.WriteFile(aiMdPath, []byte("# OAuth Pattern"), 0644)
	require.NoError(t, err)

	whyMdPath := filepath.Join(tmpDir, "oauth-pattern.why.md")
	whyContent := `---
rationale_for: "oauth-pattern"
decision_date: "2025-01-20"
decided_by: "security-team"
review_cycle: "invalid-cycle"
status: "active"
superseded_by: ""
---

## Problem Statement
Test
`
	err = os.WriteFile(whyMdPath, []byte(whyContent), 0644)
	require.NoError(t, err)

	err = engram.ValidateWhyFile(aiMdPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid review_cycle")
}

func TestValidateWhyFile_MissingRationaleFor(t *testing.T) {
	tmpDir := t.TempDir()

	aiMdPath := filepath.Join(tmpDir, "oauth-pattern.ai.md")
	err := os.WriteFile(aiMdPath, []byte("# OAuth Pattern"), 0644)
	require.NoError(t, err)

	whyMdPath := filepath.Join(tmpDir, "oauth-pattern.why.md")
	whyContent := `---
decision_date: "2025-01-20"
decided_by: "security-team"
review_cycle: "quarterly"
status: "active"
---

## Problem Statement
Test
`
	err = os.WriteFile(whyMdPath, []byte(whyContent), 0644)
	require.NoError(t, err)

	err = engram.ValidateWhyFile(aiMdPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field: rationale_for")
}

func TestParseWhyFile(t *testing.T) {
	tmpDir := t.TempDir()

	whyMdPath := filepath.Join(tmpDir, "test.why.md")
	whyContent := `---
rationale_for: "test-pattern"
decision_date: "2025-01-20"
decided_by: "team"
review_cycle: "quarterly"
status: "active"
superseded_by: ""
---

## Problem Statement
This is a test problem statement.
`
	err := os.WriteFile(whyMdPath, []byte(whyContent), 0644)
	require.NoError(t, err)

	why, err := engram.ParseWhyFile(whyMdPath)
	require.NoError(t, err)
	assert.Equal(t, "test-pattern", why.RationaleFor)
	assert.Equal(t, "2025-01-20", why.DecisionDate)
	assert.Equal(t, "team", why.DecidedBy)
	assert.Equal(t, "quarterly", why.ReviewCycle)
	assert.Equal(t, "active", why.Status)
	assert.Equal(t, "", why.SupersededBy)
	assert.Contains(t, why.Content, "Problem Statement")
}

func TestParseWhyFile_NoFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()

	whyMdPath := filepath.Join(tmpDir, "test.why.md")
	whyContent := `## Problem Statement
This has no frontmatter.
`
	err := os.WriteFile(whyMdPath, []byte(whyContent), 0644)
	require.NoError(t, err)

	_, err = engram.ParseWhyFile(whyMdPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no frontmatter found")
}

func TestParseWhyFile_UnclosedFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()

	whyMdPath := filepath.Join(tmpDir, "test.why.md")
	whyContent := `---
rationale_for: "test-pattern"
decision_date: "2025-01-20"

## Problem Statement
Missing closing ---
`
	err := os.WriteFile(whyMdPath, []byte(whyContent), 0644)
	require.NoError(t, err)

	_, err = engram.ParseWhyFile(whyMdPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unclosed frontmatter")
}

func TestGenerateWhyTemplate(t *testing.T) {
	template := engram.GenerateWhyTemplate("oauth-pattern")

	assert.Contains(t, template, `rationale_for: "oauth-pattern"`)
	assert.Contains(t, template, "decision_date:")
	assert.Contains(t, template, "TODO")
	assert.Contains(t, template, "## Problem Statement")
	assert.Contains(t, template, "## Decision Criteria")
	assert.Contains(t, template, "## Alternatives Considered")
	assert.Contains(t, template, "## Decision")
	assert.Contains(t, template, "## Success Metrics")
	assert.Contains(t, template, "## Review Schedule")
}

func TestGenerateWhyTemplate_CurrentDate(t *testing.T) {
	template := engram.GenerateWhyTemplate("test")

	// Should contain current date in YYYY-MM-DD format
	assert.Contains(t, template, "decision_date:")
	assert.Regexp(t, `decision_date: "\d{4}-\d{2}-\d{2}"`, template)
}

func TestCreateWhyFileIfMissing(t *testing.T) {
	tmpDir := t.TempDir()

	aiMdPath := filepath.Join(tmpDir, "oauth-pattern.ai.md")
	err := os.WriteFile(aiMdPath, []byte("# OAuth Pattern"), 0644)
	require.NoError(t, err)

	// Generate .why.md
	err = engram.CreateWhyFileIfMissing(aiMdPath)
	require.NoError(t, err)

	// Verify file created
	whyMdPath := filepath.Join(tmpDir, "oauth-pattern.why.md")
	_, err = os.Stat(whyMdPath)
	assert.NoError(t, err)

	// Verify content
	content, err := os.ReadFile(whyMdPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), `rationale_for: "oauth-pattern"`)
	assert.Contains(t, string(content), "TODO")
}

func TestCreateWhyFileIfMissing_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()

	aiMdPath := filepath.Join(tmpDir, "oauth-pattern.ai.md")
	err := os.WriteFile(aiMdPath, []byte("# OAuth Pattern"), 0644)
	require.NoError(t, err)

	whyMdPath := filepath.Join(tmpDir, "oauth-pattern.why.md")
	existingContent := "# Existing content"
	err = os.WriteFile(whyMdPath, []byte(existingContent), 0644)
	require.NoError(t, err)

	// Should not overwrite
	err = engram.CreateWhyFileIfMissing(aiMdPath)
	require.NoError(t, err)

	// Verify content unchanged
	content, err := os.ReadFile(whyMdPath)
	require.NoError(t, err)
	assert.Equal(t, existingContent, string(content))
}

func TestRequireWhyFile(t *testing.T) {
	tmpDir := t.TempDir()

	aiMdPath := filepath.Join(tmpDir, "oauth-pattern.ai.md")
	err := os.WriteFile(aiMdPath, []byte("# OAuth Pattern"), 0644)
	require.NoError(t, err)

	whyMdPath := filepath.Join(tmpDir, "oauth-pattern.why.md")
	whyContent := `---
rationale_for: "oauth-pattern"
decision_date: "2025-01-20"
decided_by: "security-team"
review_cycle: "quarterly"
status: "active"
superseded_by: ""
---

## Problem Statement
Test
`
	err = os.WriteFile(whyMdPath, []byte(whyContent), 0644)
	require.NoError(t, err)

	// RequireWhyFile is an alias for ValidateWhyFile
	err = engram.RequireWhyFile(aiMdPath)
	assert.NoError(t, err)
}

func TestValidateWhyFile_AllStatuses(t *testing.T) {
	statuses := []struct {
		status       string
		supersededBy string
		shouldFail   bool
	}{
		{"active", "", false},
		{"deprecated", "", false},
		{"superseded", "new-pattern", false},
		{"superseded", "", true}, // Missing superseded_by
	}

	for _, tc := range statuses {
		t.Run(tc.status, func(t *testing.T) {
			tmpDir := t.TempDir()

			aiMdPath := filepath.Join(tmpDir, "test.ai.md")
			err := os.WriteFile(aiMdPath, []byte("# Test"), 0644)
			require.NoError(t, err)

			whyMdPath := filepath.Join(tmpDir, "test.why.md")
			whyContent := fmt.Sprintf(`---
rationale_for: "test"
decision_date: "2025-01-20"
decided_by: "team"
review_cycle: "quarterly"
status: "%s"
superseded_by: "%s"
---

## Problem
Test
`, tc.status, tc.supersededBy)
			err = os.WriteFile(whyMdPath, []byte(whyContent), 0644)
			require.NoError(t, err)

			err = engram.ValidateWhyFile(aiMdPath)
			if tc.shouldFail {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateWhyFile_AllReviewCycles(t *testing.T) {
	cycles := []string{"quarterly", "annually", "as-needed"}

	for _, cycle := range cycles {
		t.Run(cycle, func(t *testing.T) {
			tmpDir := t.TempDir()

			aiMdPath := filepath.Join(tmpDir, "test.ai.md")
			err := os.WriteFile(aiMdPath, []byte("# Test"), 0644)
			require.NoError(t, err)

			whyMdPath := filepath.Join(tmpDir, "test.why.md")
			whyContent := fmt.Sprintf(`---
rationale_for: "test"
decision_date: "2025-01-20"
decided_by: "team"
review_cycle: "%s"
status: "active"
superseded_by: ""
---

## Problem
Test
`, cycle)
			err = os.WriteFile(whyMdPath, []byte(whyContent), 0644)
			require.NoError(t, err)

			err = engram.ValidateWhyFile(aiMdPath)
			assert.NoError(t, err)
		})
	}
}
