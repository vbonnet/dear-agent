package quality

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveAndLoadBaseline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")

	original := &Baseline{
		IssueCount:  5,
		Timestamp:   "2026-03-30T00:00:00Z",
		GoVetOutput: []string{"file.go:10: issue one", "file.go:20: issue two"},
		CommitHash:  "abc123",
	}

	err := SaveBaseline(path, original)
	require.NoError(t, err)

	loaded, err := LoadBaseline(path)
	require.NoError(t, err)

	assert.Equal(t, original.IssueCount, loaded.IssueCount)
	assert.Equal(t, original.Timestamp, loaded.Timestamp)
	assert.Equal(t, original.CommitHash, loaded.CommitHash)
	assert.Equal(t, original.GoVetOutput, loaded.GoVetOutput)
}

func TestCountIssues(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected int
	}{
		{name: "empty output", input: []string{}, expected: 0},
		{name: "nil output", input: nil, expected: 0},
		{name: "single issue", input: []string{"main.go:5: error"}, expected: 1},
		{name: "multiple issues", input: []string{
			"main.go:5: unused variable",
			"util.go:10: missing return",
			"handler.go:3: unreachable code",
		}, expected: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, CountIssues(tt.input))
		})
	}
}

func TestCheckRegression(t *testing.T) {
	baseline := &Baseline{IssueCount: 5}

	t.Run("same count passes", func(t *testing.T) {
		err := CheckRegression(5, baseline)
		assert.NoError(t, err)
	})

	t.Run("lower count passes", func(t *testing.T) {
		err := CheckRegression(3, baseline)
		assert.NoError(t, err)
	})

	t.Run("higher count fails", func(t *testing.T) {
		err := CheckRegression(8, baseline)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "regression detected")
		assert.Contains(t, err.Error(), "8 issues")
		assert.Contains(t, err.Error(), "baseline: 5")
		assert.Contains(t, err.Error(), "+3")
	})

	t.Run("zero baseline with issues fails", func(t *testing.T) {
		zeroBaseline := &Baseline{IssueCount: 0}
		err := CheckRegression(1, zeroBaseline)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "regression detected")
	})

	t.Run("zero baseline with zero issues passes", func(t *testing.T) {
		zeroBaseline := &Baseline{IssueCount: 0}
		err := CheckRegression(0, zeroBaseline)
		assert.NoError(t, err)
	})
}

func TestLoadBaseline_FileNotFound(t *testing.T) {
	_, err := LoadBaseline("/nonexistent/path/baseline.json")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read baseline file")
}

func TestLoadBaseline_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")

	require.NoError(t, os.WriteFile(path, []byte("not json"), 0644))

	_, err := LoadBaseline(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse baseline JSON")
}

func TestSaveBaseline_AutoTimestamp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")

	b := &Baseline{IssueCount: 0}
	err := SaveBaseline(path, b)
	require.NoError(t, err)

	loaded, err := LoadBaseline(path)
	require.NoError(t, err)
	assert.NotEmpty(t, loaded.Timestamp)
}

func TestParseOutputLines(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		lines := parseOutputLines("")
		assert.Empty(t, lines)
	})

	t.Run("lines with whitespace", func(t *testing.T) {
		lines := parseOutputLines("  line1  \n\n  line2  \n")
		assert.Equal(t, []string{"line1", "line2"}, lines)
	})
}
