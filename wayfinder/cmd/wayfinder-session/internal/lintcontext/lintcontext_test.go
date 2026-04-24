package lintcontext

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to determine test file location")
	}
	return filepath.Join(filepath.Dir(filename), "testdata")
}

func TestSummarize_NoConfigs(t *testing.T) {
	result, err := Summarize(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Fatalf("expected empty summary for dir with no configs, got: %q", result)
	}
}

func TestSummarize_NonexistentDir(t *testing.T) {
	result, err := Summarize("/nonexistent/path/that/does/not/exist")
	if err != nil {
		t.Fatalf("unexpected error for nonexistent dir: %v", err)
	}
	if result != "" {
		t.Fatalf("expected empty summary for nonexistent dir, got: %q", result)
	}
}

func TestSummarize_GolangCI(t *testing.T) {
	dir := filepath.Join(testdataDir(t), "golangci")
	result, err := Summarize(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == "" {
		t.Fatal("expected non-empty summary for Go project")
	}

	// Verify key content.
	assertContains(t, result, "Go")
	assertContains(t, result, "golangci-lint")
	assertContains(t, result, "errcheck")
	assertContains(t, result, "govet")
	assertContains(t, result, "staticcheck")
	assertContains(t, result, "gosec")
	assertContains(t, result, "gocyclo")
	assertContains(t, result, "15")
}

func TestSummarize_Python(t *testing.T) {
	dir := filepath.Join(testdataDir(t), "python")
	result, err := Summarize(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == "" {
		t.Fatal("expected non-empty summary for Python project")
	}

	assertContains(t, result, "Python")
	assertContains(t, result, "ruff")
	assertContains(t, result, "ANN")
	assertContains(t, result, "TCH")
	assertContains(t, result, "pyright")
	assertContains(t, result, "strict")
}

func TestSummarize_ESLintJSON(t *testing.T) {
	dir := filepath.Join(testdataDir(t), "eslint")
	result, err := Summarize(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == "" {
		t.Fatal("expected non-empty summary for ESLint project")
	}

	assertContains(t, result, "TypeScript/JavaScript")
	assertContains(t, result, "eslint")
	assertContains(t, result, "eslint:recommended")
}

func TestSummarize_ESLintFlatConfig(t *testing.T) {
	dir := filepath.Join(testdataDir(t), "eslint-flat")
	result, err := Summarize(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == "" {
		t.Fatal("expected non-empty summary for flat ESLint config")
	}

	assertContains(t, result, "eslint")
	assertContains(t, result, "eslint.config.js")
}

func TestParseGolangCI_Settings(t *testing.T) {
	dir := filepath.Join(testdataDir(t), "golangci")
	config, err := parseGolangCI(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config == nil {
		t.Fatal("expected non-nil config")
	}

	if len(config.Linters) != 5 {
		t.Fatalf("expected 5 linters, got %d: %v", len(config.Linters), config.Linters)
	}

	if val, ok := config.Settings["gocyclo.min-complexity"]; !ok || val != "15" {
		t.Fatalf("expected gocyclo.min-complexity=15, got %q", val)
	}
}

func TestParsePyproject_RuffAndPyright(t *testing.T) {
	dir := filepath.Join(testdataDir(t), "python")
	config, err := parsePyproject(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config == nil {
		t.Fatal("expected non-nil config")
	}

	if len(config.Linters) != 2 {
		t.Fatalf("expected 2 linters (ruff, pyright), got %d: %v", len(config.Linters), config.Linters)
	}

	foundRuff := false
	foundPyright := false
	for _, l := range config.Linters {
		switch l {
		case "ruff":
			foundRuff = true
		case "pyright":
			foundPyright = true
		}
	}
	if !foundRuff {
		t.Error("expected ruff in linters")
	}
	if !foundPyright {
		t.Error("expected pyright in linters")
	}
}

func TestFormatSummary_Structure(t *testing.T) {
	configs := []LintConfig{
		{
			Language: "Go",
			Tool:     "golangci-lint",
			Linters:  []string{"errcheck", "govet"},
			Settings: map[string]string{"gocyclo.min-complexity": "15"},
		},
	}
	result := formatSummary(configs)

	assertContains(t, result, "## Lint Rules Summary")
	assertContains(t, result, "### Go (golangci-lint)")
	assertContains(t, result, "errcheck, govet")
	assertContains(t, result, "gocyclo.min-complexity")
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected output to contain %q, got:\n%s", needle, haystack)
	}
}
