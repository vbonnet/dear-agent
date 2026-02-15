package enforcement

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestPatternDB() *PatternDatabase {
	return &PatternDatabase{
		Version: "1.0",
		Patterns: []Pattern{
			{
				ID:              "cd-chaining",
				Regex:           `cd\s+[^\s]+\s+&&`,
				Reason:          "Command chaining with cd",
				Alternative:     "Use tool-specific -C flag",
				Severity:        "high",
				Tier2Validation: true,
				Tier3Rejection:  true,
			},
			{
				ID:              "cat-file-read",
				Regex:           `^cat\s+[^\|]+$`,
				Reason:          "Using cat to read files",
				Alternative:     "Use Read tool",
				Severity:        "high",
				Tier2Validation: true,
			},
			{
				ID:              "grep-search",
				Regex:           `grep\s+`,
				Reason:          "Using bash grep",
				Alternative:     "Use Grep tool",
				Severity:        "high",
				Tier2Validation: true,
			},
			{
				ID:              "git-worktree-context",
				Regex:           `git\s+checkout\s+(main|master)`,
				Reason:          "Checking out main in non-worktree context",
				Alternative:     "Use git worktree",
				Severity:        "critical",
				ContextCheck:    "has_worktrees",
				Tier3Rejection:  true,
			},
		},
	}
}

func TestNewDetector(t *testing.T) {
	db := createTestPatternDB()
	detector, err := NewDetector(db)

	require.NoError(t, err)
	require.NotNil(t, detector)
	assert.Equal(t, db, detector.patterns)
	assert.Len(t, detector.compiledRegex, len(db.Patterns))
}

func TestNewDetector_InvalidRegex(t *testing.T) {
	db := &PatternDatabase{
		Patterns: []Pattern{
			{
				ID:    "bad-pattern",
				Regex: `[invalid(regex`, // Invalid regex
			},
			{
				ID:    "good-pattern",
				Regex: `test`,
			},
		},
	}

	detector, err := NewDetector(db)
	require.NoError(t, err)
	require.NotNil(t, detector)

	// Should skip the bad pattern
	assert.Len(t, detector.GetSkippedPatterns(), 1)
	assert.Equal(t, "bad-pattern", detector.GetSkippedPatterns()[0])

	// Should still compile the good pattern
	assert.Len(t, detector.compiledRegex, 1)
	_, ok := detector.compiledRegex["good-pattern"]
	assert.True(t, ok)
}

func TestDetect_CDChaining(t *testing.T) {
	db := createTestPatternDB()
	detector, err := NewDetector(db)
	require.NoError(t, err)

	testCases := []struct {
		name      string
		command   string
		shouldMatch bool
		patternID string
	}{
		{
			name:        "cd with && should match",
			command:     "cd /repo && git push",
			shouldMatch: true,
			patternID:   "cd-chaining",
		},
		{
			name:        "cd with && and more commands",
			command:     "cd ~/src && npm test",
			shouldMatch: true,
			patternID:   "cd-chaining",
		},
		{
			name:        "cd alone should not match",
			command:     "cd /repo",
			shouldMatch: false,
		},
		{
			name:        "cd with semicolon should not match (different pattern)",
			command:     "cd /repo; git push",
			shouldMatch: false,
		},
		{
			name:        "no cd command",
			command:     "git push",
			shouldMatch: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pattern, err := detector.Detect(tc.command)
			require.NoError(t, err)

			if tc.shouldMatch {
				require.NotNil(t, pattern, "Expected pattern to match")
				assert.Equal(t, tc.patternID, pattern.ID)
			} else {
				assert.Nil(t, pattern, "Expected no pattern match")
			}
		})
	}
}

func TestDetect_CatFileRead(t *testing.T) {
	db := createTestPatternDB()
	detector, err := NewDetector(db)
	require.NoError(t, err)

	testCases := []struct {
		name        string
		command     string
		shouldMatch bool
	}{
		{
			name:        "cat with file should match",
			command:     "cat file.txt",
			shouldMatch: true,
		},
		{
			name:        "cat with path should match",
			command:     "cat /path/to/file.log",
			shouldMatch: true,
		},
		{
			name:        "cat with pipe should not match (different pattern)",
			command:     "cat file.txt | grep pattern",
			shouldMatch: false,
		},
		{
			name:        "not a cat command",
			command:     "ls -la",
			shouldMatch: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pattern, err := detector.Detect(tc.command)
			require.NoError(t, err)

			if tc.shouldMatch {
				require.NotNil(t, pattern)
				assert.Equal(t, "cat-file-read", pattern.ID)
			} else {
				if pattern != nil {
					assert.NotEqual(t, "cat-file-read", pattern.ID)
				}
			}
		})
	}
}

func TestDetect_GrepSearch(t *testing.T) {
	db := createTestPatternDB()
	detector, err := NewDetector(db)
	require.NoError(t, err)

	testCases := []struct {
		name        string
		command     string
		shouldMatch bool
	}{
		{
			name:        "grep with pattern should match",
			command:     "grep 'TODO' file.txt",
			shouldMatch: true,
		},
		{
			name:        "grep with recursive flag should match",
			command:     "grep -r 'pattern' src/",
			shouldMatch: true,
		},
		{
			name:        "grep case insensitive should match",
			command:     "grep -i 'error' *.log",
			shouldMatch: true,
		},
		{
			name:        "no grep command",
			command:     "ls -la",
			shouldMatch: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pattern, err := detector.Detect(tc.command)
			require.NoError(t, err)

			if tc.shouldMatch {
				require.NotNil(t, pattern)
				assert.Equal(t, "grep-search", pattern.ID)
			} else {
				assert.Nil(t, pattern)
			}
		})
	}
}

func TestDetectWithContext(t *testing.T) {
	db := createTestPatternDB()
	detector, err := NewDetector(db)
	require.NoError(t, err)

	testCases := []struct {
		name        string
		command     string
		context     Context
		shouldMatch bool
		patternID   string
	}{
		{
			name:    "git checkout main with worktrees should match",
			command: "git checkout main",
			context: Context{
				HasWorktrees:   true,
				IsMainWorktree: false,
				WorkingDir:     "/home/user/repo",
			},
			shouldMatch: true,
			patternID:   "git-worktree-context",
		},
		{
			name:    "git checkout main without worktrees should not match",
			command: "git checkout main",
			context: Context{
				HasWorktrees:   false,
				IsMainWorktree: false,
				WorkingDir:     "/home/user/repo",
			},
			shouldMatch: false,
		},
		{
			name:    "git checkout master with worktrees should match",
			command: "git checkout master",
			context: Context{
				HasWorktrees:   true,
				IsMainWorktree: false,
				WorkingDir:     "/home/user/repo",
			},
			shouldMatch: true,
			patternID:   "git-worktree-context",
		},
		{
			name:    "other git commands should not match",
			command: "git status",
			context: Context{
				HasWorktrees: true,
			},
			shouldMatch: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pattern, err := detector.DetectWithContext(tc.command, tc.context)
			require.NoError(t, err)

			if tc.shouldMatch {
				require.NotNil(t, pattern, "Expected pattern to match")
				assert.Equal(t, tc.patternID, pattern.ID)
			} else {
				if pattern != nil {
					// If we got a match, it shouldn't be the context-specific one
					assert.NotEqual(t, "git-worktree-context", pattern.ID)
				}
			}
		})
	}
}

func TestDetectAll(t *testing.T) {
	db := createTestPatternDB()
	detector, err := NewDetector(db)
	require.NoError(t, err)

	// Command that matches multiple patterns
	command := "grep pattern file.txt"

	violations, err := detector.DetectAll(command)
	require.NoError(t, err)
	require.NotNil(t, violations)

	// Should match grep-search pattern
	assert.Greater(t, len(violations), 0)

	foundGrep := false
	for _, v := range violations {
		if v.ID == "grep-search" {
			foundGrep = true
		}
	}
	assert.True(t, foundGrep, "Should find grep-search pattern")
}

func TestDetectAll_NoMatches(t *testing.T) {
	db := createTestPatternDB()
	detector, err := NewDetector(db)
	require.NoError(t, err)

	command := "ls -la"

	violations, err := detector.DetectAll(command)
	require.NoError(t, err)
	assert.Len(t, violations, 0)
}

func TestDetectAllWithContext(t *testing.T) {
	db := createTestPatternDB()
	detector, err := NewDetector(db)
	require.NoError(t, err)

	command := "git checkout main"
	ctx := Context{
		HasWorktrees:   true,
		IsMainWorktree: false,
	}

	violations, err := detector.DetectAllWithContext(command, ctx)
	require.NoError(t, err)
	require.Greater(t, len(violations), 0)

	// Should find the git-worktree-context pattern
	foundPattern := false
	for _, v := range violations {
		if v.ID == "git-worktree-context" {
			foundPattern = true
		}
	}
	assert.True(t, foundPattern)
}

func TestCheckContext(t *testing.T) {
	db := createTestPatternDB()
	detector, err := NewDetector(db)
	require.NoError(t, err)

	testCases := []struct {
		name         string
		pattern      Pattern
		context      Context
		shouldPass   bool
	}{
		{
			name: "has_worktrees check with worktrees",
			pattern: Pattern{
				ContextCheck: "has_worktrees",
			},
			context: Context{
				HasWorktrees: true,
			},
			shouldPass: true,
		},
		{
			name: "has_worktrees check without worktrees",
			pattern: Pattern{
				ContextCheck: "has_worktrees",
			},
			context: Context{
				HasWorktrees: false,
			},
			shouldPass: false,
		},
		{
			name: "is_main_worktree check when true",
			pattern: Pattern{
				ContextCheck: "is_main_worktree",
			},
			context: Context{
				IsMainWorktree: true,
			},
			shouldPass: true,
		},
		{
			name: "is_main_worktree check when false",
			pattern: Pattern{
				ContextCheck: "is_main_worktree",
			},
			context: Context{
				IsMainWorktree: false,
			},
			shouldPass: false,
		},
		{
			name: "not_main_worktree check when false",
			pattern: Pattern{
				ContextCheck: "not_main_worktree",
			},
			context: Context{
				IsMainWorktree: false,
			},
			shouldPass: true,
		},
		{
			name: "unknown context check should pass",
			pattern: Pattern{
				ContextCheck: "unknown_check",
			},
			context: Context{},
			shouldPass: true,
		},
		{
			name: "empty context check should pass",
			pattern: Pattern{
				ContextCheck: "",
			},
			context: Context{},
			shouldPass: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := detector.checkContext(&tc.pattern, tc.context)
			assert.Equal(t, tc.shouldPass, result)
		})
	}
}

func TestDetector_Performance(t *testing.T) {
	// Load real patterns and ensure detection is fast
	db := createTestPatternDB()
	detector, err := NewDetector(db)
	require.NoError(t, err)

	commands := []string{
		"cd /repo && git push",
		"cat file.txt",
		"grep pattern file.txt",
		"ls -la",
		"git status",
	}

	// Run detection multiple times to verify regex caching works
	for i := 0; i < 100; i++ {
		for _, cmd := range commands {
			_, err := detector.Detect(cmd)
			require.NoError(t, err)
		}
	}
}
