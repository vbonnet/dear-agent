package enforcement

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMatcher(t *testing.T) {
	matcher, err := NewMatcher(`cd\s+[^\s]+`)
	require.NoError(t, err)
	require.NotNil(t, matcher)
	assert.NotNil(t, matcher.regex)
}

func TestNewMatcher_InvalidRegex(t *testing.T) {
	matcher, err := NewMatcher(`[invalid(regex`)
	assert.Error(t, err)
	assert.Nil(t, matcher)
}

func TestMatch(t *testing.T) {
	matcher, err := NewMatcher(`cd\s+[^\s]+`)
	require.NoError(t, err)

	testCases := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "matches cd command",
			input:    "cd /repo",
			expected: true,
		},
		{
			name:     "matches cd with && ",
			input:    "cd /repo && git push",
			expected: true,
		},
		{
			name:     "does not match without cd",
			input:    "git push",
			expected: false,
		},
		{
			name:     "does not match cd alone",
			input:    "cd",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := matcher.Match(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestFindMatch(t *testing.T) {
	matcher, err := NewMatcher(`cd\s+([^\s]+)`)
	require.NoError(t, err)

	result := matcher.FindMatch("cd /repo && git push")
	require.NotNil(t, result)
	assert.True(t, result.Matched)
	assert.Equal(t, "cd /repo", result.MatchText)
	assert.Equal(t, 0, result.StartIndex)
	assert.Equal(t, 8, result.EndIndex)
	assert.Len(t, result.Groups, 2) // Full match + captured group
	assert.Equal(t, "/repo", result.Groups[1])
}

func TestFindMatch_NoMatch(t *testing.T) {
	matcher, err := NewMatcher(`cd\s+[^\s]+`)
	require.NoError(t, err)

	result := matcher.FindMatch("git push")
	require.NotNil(t, result)
	assert.False(t, result.Matched)
	assert.Empty(t, result.MatchText)
}

func TestFindAllMatches(t *testing.T) {
	matcher, err := NewMatcher(`cd\s+([^\s]+)`)
	require.NoError(t, err)

	input := "cd /repo1 && cd /repo2 && cd /repo3"
	results := matcher.FindAllMatches(input)

	require.Len(t, results, 3)

	// First match
	assert.True(t, results[0].Matched)
	assert.Equal(t, "cd /repo1", results[0].MatchText)
	assert.Equal(t, "/repo1", results[0].Groups[1])

	// Second match
	assert.Equal(t, "cd /repo2", results[1].MatchText)
	assert.Equal(t, "/repo2", results[1].Groups[1])

	// Third match
	assert.Equal(t, "cd /repo3", results[2].MatchText)
	assert.Equal(t, "/repo3", results[2].Groups[1])
}

func TestFindAllMatches_NoMatches(t *testing.T) {
	matcher, err := NewMatcher(`cd\s+[^\s]+`)
	require.NoError(t, err)

	results := matcher.FindAllMatches("git push origin main")
	assert.Nil(t, results)
}

func TestReplaceAll(t *testing.T) {
	matcher, err := NewMatcher(`cd\s+([^\s]+)`)
	require.NoError(t, err)

	input := "cd /repo && git push"
	result := matcher.ReplaceAll(input, "git -C $1")

	// $1 is replaced with the first captured group (/repo)
	assert.Equal(t, "git -C /repo && git push", result)
}

func TestReplaceAll_MultipleReplacements(t *testing.T) {
	matcher, err := NewMatcher(`cd\s+([^\s]+)`)
	require.NoError(t, err)

	input := "cd /repo1 && cd /repo2"
	result := matcher.ReplaceAll(input, "git -C $1")

	// Each $1 is replaced with its respective captured group
	assert.Equal(t, "git -C /repo1 && git -C /repo2", result)
}

func TestSplit(t *testing.T) {
	matcher, err := NewMatcher(`\s*&&\s*`)
	require.NoError(t, err)

	input := "cd /repo && git push && git status"
	parts := matcher.Split(input, -1)

	require.Len(t, parts, 3)
	assert.Equal(t, "cd /repo", parts[0])
	assert.Equal(t, "git push", parts[1])
	assert.Equal(t, "git status", parts[2])
}

func TestSplit_WithLimit(t *testing.T) {
	matcher, err := NewMatcher(`\s*&&\s*`)
	require.NoError(t, err)

	input := "cd /repo && git push && git status"
	parts := matcher.Split(input, 2)

	require.Len(t, parts, 2)
	assert.Equal(t, "cd /repo", parts[0])
	assert.Equal(t, "git push && git status", parts[1])
}

func TestQuickMatch(t *testing.T) {
	testCases := []struct {
		name     string
		pattern  string
		input    string
		expected bool
	}{
		{
			name:     "simple match",
			pattern:  `cd\s+`,
			input:    "cd /repo",
			expected: true,
		},
		{
			name:     "no match",
			pattern:  `cd\s+`,
			input:    "git push",
			expected: false,
		},
		{
			name:     "complex pattern match",
			pattern:  `^cat\s+[^\|]+$`,
			input:    "cat file.txt",
			expected: true,
		},
		{
			name:     "complex pattern no match",
			pattern:  `^cat\s+[^\|]+$`,
			input:    "cat file.txt | grep pattern",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := QuickMatch(tc.pattern, tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestQuickMatch_InvalidRegex(t *testing.T) {
	result, err := QuickMatch(`[invalid(regex`, "test")
	assert.Error(t, err)
	assert.False(t, result)
}

func TestExtractGroups(t *testing.T) {
	matcher, err := NewMatcher(`git\s+(?P<command>\w+)\s+(?P<branch>\S+)`)
	require.NoError(t, err)

	input := "git checkout main"
	groups := matcher.ExtractGroups(input)

	require.NotEmpty(t, groups)
	assert.Equal(t, "checkout", groups["command"])
	assert.Equal(t, "main", groups["branch"])
}

func TestExtractGroups_NoMatch(t *testing.T) {
	matcher, err := NewMatcher(`git\s+(?P<command>\w+)\s+(?P<branch>\S+)`)
	require.NoError(t, err)

	input := "ls -la"
	groups := matcher.ExtractGroups(input)

	assert.Empty(t, groups)
}

func TestExtractGroups_UnnamedGroups(t *testing.T) {
	matcher, err := NewMatcher(`git\s+(\w+)\s+(\S+)`)
	require.NoError(t, err)

	input := "git checkout main"
	groups := matcher.ExtractGroups(input)

	// Unnamed groups should not be in the map
	assert.Empty(t, groups)
}

func TestMatcher_ComplexPatterns(t *testing.T) {
	testCases := []struct {
		name     string
		pattern  string
		input    string
		expected bool
	}{
		{
			name:     "cd chaining pattern",
			pattern:  `cd\s+[^\s]+\s+&&`,
			input:    "cd /repo && git push",
			expected: true,
		},
		{
			name:     "cat file read pattern",
			pattern:  `^cat\s+[^\|]+$`,
			input:    "cat file.txt",
			expected: true,
		},
		{
			name:     "grep pattern",
			pattern:  `grep\s+`,
			input:    "grep 'pattern' file.txt",
			expected: true,
		},
		{
			name:     "find pattern",
			pattern:  `find\s+`,
			input:    "find . -name '*.py'",
			expected: true,
		},
		{
			name:     "echo redirect pattern",
			pattern:  `echo\s+.*>\s*[^\s]+`,
			input:    "echo 'text' > file.txt",
			expected: true,
		},
		{
			name:     "sqlite3 direct access",
			pattern:  `sqlite3\s+.*\.beads/`,
			input:    "sqlite3 .beads/db.sqlite3 'SELECT * FROM beads'",
			expected: true,
		},
		{
			name:     "git checkout main",
			pattern:  `git\s+(checkout|switch)\s+(main|master)`,
			input:    "git checkout main",
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			matcher, err := NewMatcher(tc.pattern)
			require.NoError(t, err)

			result := matcher.Match(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestMatcher_EdgeCases(t *testing.T) {
	matcher, err := NewMatcher(`test`)
	require.NoError(t, err)

	testCases := []struct {
		name  string
		input string
		match bool
	}{
		{
			name:  "empty string",
			input: "",
			match: false,
		},
		{
			name:  "whitespace only",
			input: "   ",
			match: false,
		},
		{
			name:  "pattern at start",
			input: "test string",
			match: true,
		},
		{
			name:  "pattern at end",
			input: "string test",
			match: true,
		},
		{
			name:  "pattern in middle",
			input: "string test string",
			match: true,
		},
		{
			name:  "pattern repeated",
			input: "test test test",
			match: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := matcher.Match(tc.input)
			assert.Equal(t, tc.match, result)
		})
	}
}

func BenchmarkMatcher_Match(b *testing.B) {
	matcher, _ := NewMatcher(`cd\s+[^\s]+\s+&&`)
	input := "cd /repo && git push"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matcher.Match(input)
	}
}

func BenchmarkMatcher_FindMatch(b *testing.B) {
	matcher, _ := NewMatcher(`cd\s+([^\s]+)`)
	input := "cd /repo && git push"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matcher.FindMatch(input)
	}
}

func BenchmarkQuickMatch(b *testing.B) {
	pattern := `cd\s+[^\s]+\s+&&`
	input := "cd /repo && git push"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		QuickMatch(pattern, input)
	}
}
