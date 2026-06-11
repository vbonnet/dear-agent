package main

import (
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
