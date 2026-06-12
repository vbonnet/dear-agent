package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandName_NoPlaceholders(t *testing.T) {
	results := expandName("Build & Test", nil)
	assert.Equal(t, []string{"Build & Test"}, results)
}

func TestExpandName_MatrixOS(t *testing.T) {
	matrix := map[string][]string{
		"os": {"ubuntu-latest", "macos-latest"},
	}
	results := expandName("Build & Test (${{ matrix.os }})", matrix)
	assert.ElementsMatch(t, []string{
		"Build & Test (ubuntu-latest)",
		"Build & Test (macos-latest)",
	}, results)
}

func TestExpandName_MultipleVars(t *testing.T) {
	matrix := map[string][]string{
		"os":   {"ubuntu-latest"},
		"arch": {"amd64", "arm64"},
	}
	results := expandName("Test (${{ matrix.os }}, ${{ matrix.arch }})", matrix)
	assert.ElementsMatch(t, []string{
		"Test (ubuntu-latest, amd64)",
		"Test (ubuntu-latest, arm64)",
	}, results)
}

func TestComputeDrift_Clean(t *testing.T) {
	jobs := map[string]bool{
		"Build & Test (ubuntu-latest)": true,
		"Build & Test (macos-latest)":  true,
		"govulncheck":                  true,
	}
	required := []string{"Build & Test (ubuntu-latest)", "govulncheck"}
	d := computeDrift(jobs, required)
	assert.Empty(t, d.missingFromWorkflows)
	assert.Equal(t, []string{"Build & Test (macos-latest)"}, d.missingFromProtection)
}

func TestComputeDrift_PhantomCheck(t *testing.T) {
	jobs := map[string]bool{
		"govulncheck": true,
	}
	required := []string{"phantom-check", "govulncheck"}
	d := computeDrift(jobs, required)
	assert.Equal(t, []string{"phantom-check"}, d.missingFromWorkflows)
}

func TestCollectJobNames_MatrixExpansion(t *testing.T) {
	dir := t.TempDir()
	wf := `
jobs:
  build:
    name: Build & Test (${{ matrix.os }})
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest]
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ci.yml"), []byte(wf), 0644))
	names, err := collectJobNames(dir)
	require.NoError(t, err)
	assert.True(t, names["Build & Test (ubuntu-latest)"])
	assert.True(t, names["Build & Test (macos-latest)"])
	assert.True(t, names["build"])
}

// TestFetchRequiredChecks_RejectsMalformedInput verifies that repo/branch are
// allowlist-validated before any URL is built, so untrusted values cannot redirect
// the request to a different host or path (the SSRF guard behind the G704 finding).
func TestFetchRequiredChecks_RejectsMalformedInput(t *testing.T) {
	cases := []struct {
		name        string
		repo        string
		branch      string
		wantErrPart string
	}{
		{"host injection in repo", "evil.com/x/repo", "main", "invalid repo"},
		{"scheme in repo", "http://evil.com/o/r", "main", "invalid repo"},
		{"too few segments", "owner", "main", "invalid repo"},
		{"too many segments", "owner/repo/extra", "main", "invalid repo"},
		{"at sign credential", "user@host/r", "main", "invalid repo"},
		{"branch with scheme", "owner/repo", "http://evil.com", "invalid branch"},
		{"branch with query", "owner/repo", "main?x=1", "invalid branch"},
		{"branch with space", "owner/repo", "ma in", "invalid branch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := fetchRequiredChecks(context.Background(), "token", tc.repo, tc.branch)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErrPart)
		})
	}
}

func TestFetchRequiredChecks_AcceptsValidRefShapes(t *testing.T) {
	// Valid owner/repo + slash-bearing branch should pass the validation guard and
	// only fail later at the network call. A pre-cancelled context makes Do() return
	// immediately (no real request), so we can assert the failure is not a validation
	// rejection without depending on the network.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := fetchRequiredChecks(ctx, "token", "owner-1/repo.go_x", "release/v1.2")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "invalid repo")
	assert.NotContains(t, err.Error(), "invalid branch")
}

func TestCollectJobNames_CodeQL(t *testing.T) {
	dir := t.TempDir()
	wf := `
jobs:
  analyze:
    name: Analyze Go Code
    strategy:
      matrix:
        language: [go]
    steps:
      - uses: github/codeql-action/analyze@v4
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "codeql.yml"), []byte(wf), 0644))
	names, err := collectJobNames(dir)
	require.NoError(t, err)
	assert.True(t, names["Analyze Go Code (go)"], "CodeQL check name with language suffix should be generated")
}
