package enforcement

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadPatterns(t *testing.T) {
	// Create a temporary test pattern file
	tmpDir := t.TempDir()
	patternFile := filepath.Join(tmpDir, "test-patterns.yaml")

	content := `version: "1.0"
updated: "2026-02-15"
purpose: "Test patterns"
used_by:
  - "test"

patterns:
  - id: test-pattern-1
    regex: 'cd\s+[^\s]+\s+&&'
    reason: "Test pattern"
    alternative: "Use alternative"
    examples:
      - "cd /repo && git push"
    severity: high
    tier2_validation: true
    tier3_rejection: true

  - id: test-pattern-2
    regex: 'cat\s+'
    reason: "Another test pattern"
    alternative: "Use Read tool"
    examples:
      - "cat file.txt"
    severity: medium
    tier2_validation: false
    tier3_rejection: true
`

	err := os.WriteFile(patternFile, []byte(content), 0644)
	require.NoError(t, err)

	// Test successful loading
	db, err := LoadPatterns(patternFile)
	require.NoError(t, err)
	require.NotNil(t, db)

	assert.Equal(t, "1.0", db.Version)
	assert.Equal(t, "2026-02-15", db.Updated)
	assert.Equal(t, "Test patterns", db.Purpose)
	assert.Equal(t, []string{"test"}, db.UsedBy)
	assert.Len(t, db.Patterns, 2)

	// Verify first pattern
	p1 := db.Patterns[0]
	assert.Equal(t, "test-pattern-1", p1.ID)
	assert.Equal(t, `cd\s+[^\s]+\s+&&`, p1.Regex)
	assert.Equal(t, "Test pattern", p1.Reason)
	assert.Equal(t, "Use alternative", p1.Alternative)
	assert.Equal(t, "high", p1.Severity)
	assert.True(t, p1.Tier2Validation)
	assert.True(t, p1.Tier3Rejection)

	// Verify second pattern
	p2 := db.Patterns[1]
	assert.Equal(t, "test-pattern-2", p2.ID)
	assert.Equal(t, "medium", p2.Severity)
	assert.False(t, p2.Tier2Validation)
	assert.True(t, p2.Tier3Rejection)
}

func TestLoadPatterns_FileNotFound(t *testing.T) {
	db, err := LoadPatterns("/nonexistent/file.yaml")
	assert.Error(t, err)
	assert.Nil(t, db)
	assert.Contains(t, err.Error(), "failed to read pattern file")
}

func TestLoadPatterns_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	patternFile := filepath.Join(tmpDir, "invalid.yaml")

	invalidContent := `invalid: yaml: content: [[[`
	err := os.WriteFile(patternFile, []byte(invalidContent), 0644)
	require.NoError(t, err)

	db, err := LoadPatterns(patternFile)
	assert.Error(t, err)
	assert.Nil(t, db)
	assert.Contains(t, err.Error(), "failed to parse pattern YAML")
}

func TestLoadPatterns_NoPatterns(t *testing.T) {
	tmpDir := t.TempDir()
	patternFile := filepath.Join(tmpDir, "empty.yaml")

	emptyContent := `version: "1.0"
patterns: []
`
	err := os.WriteFile(patternFile, []byte(emptyContent), 0644)
	require.NoError(t, err)

	db, err := LoadPatterns(patternFile)
	assert.Error(t, err)
	assert.Nil(t, db)
	assert.Contains(t, err.Error(), "no patterns found")
}

func TestLoadPatternsByType_Bash(t *testing.T) {
	// This test requires the actual pattern files to exist
	// We'll check if they exist before running the test
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	bashPatternsPath := filepath.Join(home, "src", "ws", "oss", "repos", "engram", "patterns", "bash-anti-patterns.yaml")
	if _, err := os.Stat(bashPatternsPath); os.IsNotExist(err) {
		t.Skip("Bash patterns file not found, skipping test")
	}

	db, err := LoadPatternsByType("bash")
	require.NoError(t, err)
	require.NotNil(t, db)

	assert.Greater(t, len(db.Patterns), 0, "Should have at least one bash pattern")

	// Verify some known patterns exist
	cdPattern := db.GetPattern("cd-chaining")
	assert.NotNil(t, cdPattern, "cd-chaining pattern should exist")
	if cdPattern != nil {
		assert.Equal(t, "high", cdPattern.Severity)
	}
}

func TestLoadPatternsByType_Beads(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	beadsPatternsPath := filepath.Join(home, "src", "ws", "oss", "repos", "engram", "patterns", "beads-anti-patterns.yaml")
	if _, err := os.Stat(beadsPatternsPath); os.IsNotExist(err) {
		t.Skip("Beads patterns file not found, skipping test")
	}

	db, err := LoadPatternsByType("beads")
	require.NoError(t, err)
	require.NotNil(t, db)

	assert.Greater(t, len(db.Patterns), 0, "Should have at least one beads pattern")
}

func TestLoadPatternsByType_Git(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	gitPatternsPath := filepath.Join(home, "src", "ws", "oss", "repos", "engram", "patterns", "git-anti-patterns.yaml")
	if _, err := os.Stat(gitPatternsPath); os.IsNotExist(err) {
		t.Skip("Git patterns file not found, skipping test")
	}

	db, err := LoadPatternsByType("git")
	require.NoError(t, err)
	require.NotNil(t, db)

	assert.Greater(t, len(db.Patterns), 0, "Should have at least one git pattern")
}

func TestGetPattern(t *testing.T) {
	db := &PatternDatabase{
		Patterns: []Pattern{
			{ID: "pattern-1", Severity: "high"},
			{ID: "pattern-2", Severity: "medium"},
			{ID: "pattern-3", Severity: "low"},
		},
	}

	// Test finding existing pattern
	p := db.GetPattern("pattern-2")
	require.NotNil(t, p)
	assert.Equal(t, "pattern-2", p.ID)
	assert.Equal(t, "medium", p.Severity)

	// Test non-existent pattern
	p = db.GetPattern("nonexistent")
	assert.Nil(t, p)
}

func TestFilterBySeverity(t *testing.T) {
	db := &PatternDatabase{
		Patterns: []Pattern{
			{ID: "p1", Severity: "high"},
			{ID: "p2", Severity: "medium"},
			{ID: "p3", Severity: "high"},
			{ID: "p4", Severity: "low"},
			{ID: "p5", Severity: "high"},
		},
	}

	// Test filtering high severity
	high := db.FilterBySeverity("high")
	assert.Len(t, high, 3)
	for _, p := range high {
		assert.Equal(t, "high", p.Severity)
	}

	// Test filtering medium severity
	medium := db.FilterBySeverity("medium")
	assert.Len(t, medium, 1)
	assert.Equal(t, "p2", medium[0].ID)

	// Test filtering non-existent severity
	none := db.FilterBySeverity("critical")
	assert.Len(t, none, 0)
}

func TestFilterByTier(t *testing.T) {
	db := &PatternDatabase{
		Patterns: []Pattern{
			{ID: "p1", Tier2Validation: true, Tier3Rejection: false},
			{ID: "p2", Tier2Validation: true, Tier3Rejection: true},
			{ID: "p3", Tier2Validation: false, Tier3Rejection: true},
			{ID: "p4", Tier2Validation: false, Tier3Rejection: false},
		},
	}

	// Test tier2 filtering
	tier2 := db.FilterByTier("tier2")
	assert.Len(t, tier2, 2)
	for _, p := range tier2 {
		assert.True(t, p.Tier2Validation)
	}

	// Test tier3 filtering
	tier3 := db.FilterByTier("tier3")
	assert.Len(t, tier3, 2)
	for _, p := range tier3 {
		assert.True(t, p.Tier3Rejection)
	}

	// Test unknown tier (should return empty)
	unknown := db.FilterByTier("tier99")
	assert.Len(t, unknown, 0)
}

func TestPatternDatabaseIntegrity(t *testing.T) {
	// Test all real pattern databases for integrity
	patternTypes := []string{"bash", "beads", "git"}

	for _, pType := range patternTypes {
		t.Run(pType, func(t *testing.T) {
			home, err := os.UserHomeDir()
			require.NoError(t, err)

			patternPath := filepath.Join(home, "src", "ws", "oss", "repos", "engram", "patterns",
				pType+"-anti-patterns.yaml")

			if _, err := os.Stat(patternPath); os.IsNotExist(err) {
				t.Skipf("%s patterns file not found, skipping test", pType)
			}

			db, err := LoadPatterns(patternPath)
			require.NoError(t, err)
			require.NotNil(t, db)

			// Validate each pattern has required fields
			for _, p := range db.Patterns {
				assert.NotEmpty(t, p.ID, "Pattern ID should not be empty")
				assert.NotEmpty(t, p.Regex, "Pattern regex should not be empty")
				assert.NotEmpty(t, p.Reason, "Pattern reason should not be empty")
				assert.NotEmpty(t, p.Alternative, "Pattern alternative should not be empty")
				assert.NotEmpty(t, p.Severity, "Pattern severity should not be empty")

				// Severity should be one of: low, medium, high, critical
				assert.Contains(t, []string{"low", "medium", "high", "critical"}, p.Severity,
					"Pattern %s has invalid severity: %s", p.ID, p.Severity)
			}
		})
	}
}
