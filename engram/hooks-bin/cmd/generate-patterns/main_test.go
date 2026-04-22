package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// ---------------------------------------------------------------------------
// validate() tests
// ---------------------------------------------------------------------------

func TestValidate_ValidPatterns(t *testing.T) {
	patterns := []Pattern{
		{ID: "p1", Order: 10, RE2Regex: `\bcd\s+`, PatternName: "cd command", Remediation: "use -C flag"},
		{ID: "p2", Order: 20, RE2Regex: `&&`, PatternName: "chaining", Remediation: "avoid chaining"},
	}
	errs := validate(patterns)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidate_MissingPatternName(t *testing.T) {
	patterns := []Pattern{
		{ID: "p1", Order: 10, RE2Regex: `\bcd\s+`, PatternName: "", Remediation: "use -C flag"},
	}
	errs := validate(patterns)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0], "missing pattern_name") {
		t.Errorf("expected 'missing pattern_name' error, got: %s", errs[0])
	}
}

func TestValidate_MissingRemediation(t *testing.T) {
	patterns := []Pattern{
		{ID: "p1", Order: 10, RE2Regex: `\bcd\s+`, PatternName: "cd command", Remediation: ""},
	}
	errs := validate(patterns)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0], "missing remediation") {
		t.Errorf("expected 'missing remediation' error, got: %s", errs[0])
	}
}

func TestValidate_InvalidRE2Regex(t *testing.T) {
	patterns := []Pattern{
		{ID: "p1", Order: 10, RE2Regex: `(unclosed`, PatternName: "bad regex", Remediation: "fix it"},
	}
	errs := validate(patterns)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0], "re2_regex does not compile") {
		t.Errorf("expected regex compile error, got: %s", errs[0])
	}
}

func TestValidate_DuplicateOrder(t *testing.T) {
	patterns := []Pattern{
		{ID: "p1", Order: 10, RE2Regex: `a`, PatternName: "first", Remediation: "fix"},
		{ID: "p2", Order: 10, RE2Regex: `b`, PatternName: "second", Remediation: "fix"},
	}
	errs := validate(patterns)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0], "duplicate order") {
		t.Errorf("expected 'duplicate order' error, got: %s", errs[0])
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	patterns := []Pattern{
		{ID: "p1", Order: 10, RE2Regex: `(bad`, PatternName: "", Remediation: ""},
	}
	errs := validate(patterns)
	// Missing pattern_name, missing remediation, and invalid regex = 3 errors.
	if len(errs) != 3 {
		t.Errorf("expected 3 errors, got %d: %v", len(errs), errs)
	}
}

// ---------------------------------------------------------------------------
// lintExamples() tests
// ---------------------------------------------------------------------------

func TestLintExamples_MatchingExamplesPass(t *testing.T) {
	patterns := []Pattern{
		{
			ID:       "p1",
			Order:    10,
			RE2Regex: `\bcd\s+`,
			Examples: []string{"cd /tmp", "cd /home/user"},
		},
	}
	errs := lintExamples(patterns)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestLintExamples_NonMatchingExample(t *testing.T) {
	patterns := []Pattern{
		{
			ID:       "p1",
			Order:    10,
			RE2Regex: `\bcd\s+`,
			Examples: []string{"git status"},
		},
	}
	errs := lintExamples(patterns)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0], "example should match but doesn't") {
		t.Errorf("expected match failure message, got: %s", errs[0])
	}
}

func TestLintExamples_ShouldNotMatchButDoes(t *testing.T) {
	patterns := []Pattern{
		{
			ID:             "p1",
			Order:          10,
			RE2Regex:       `\bcd\s+`,
			ShouldNotMatch: []string{"cd /tmp"},
		},
	}
	errs := lintExamples(patterns)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0], "should_not_match but does") {
		t.Errorf("expected should_not_match failure message, got: %s", errs[0])
	}
}

func TestLintExamples_NoExamplesPass(t *testing.T) {
	patterns := []Pattern{
		{ID: "p1", Order: 10, RE2Regex: `\bcd\s+`},
	}
	errs := lintExamples(patterns)
	if len(errs) != 0 {
		t.Errorf("expected no errors for pattern with no examples, got %v", errs)
	}
}

func TestLintExamples_InvalidRegexSkipped(t *testing.T) {
	patterns := []Pattern{
		{
			ID:       "p1",
			Order:    10,
			RE2Regex: `(unclosed`,
			Examples: []string{"anything"},
		},
	}
	// Invalid regex should be skipped (already caught by validate).
	errs := lintExamples(patterns)
	if len(errs) != 0 {
		t.Errorf("expected no errors (invalid regex skipped), got %v", errs)
	}
}

// ---------------------------------------------------------------------------
// regexLiteral() tests
// ---------------------------------------------------------------------------

func TestRegexLiteral_NormalUsesBacktick(t *testing.T) {
	input := `\bcd\s+`
	got := regexLiteral(input)
	if got != "`\\bcd\\s+`" {
		t.Errorf("expected backtick-quoted literal, got: %s", got)
	}
}

func TestRegexLiteral_BacktickUsesDoubleQuoteWithX60(t *testing.T) {
	input := "echo `date`"
	got := regexLiteral(input)
	if !strings.HasPrefix(got, `"`) {
		t.Errorf("expected double-quoted literal, got: %s", got)
	}
	if !strings.Contains(got, `\x60`) {
		t.Errorf("expected \\x60 escaping for backtick, got: %s", got)
	}
	// Should not contain a raw backtick.
	if strings.Contains(got[1:len(got)-1], "`") {
		t.Errorf("raw backtick found inside escaped literal: %s", got)
	}
}

func TestRegexLiteral_BackslashesEscapedInDoubleQuoteMode(t *testing.T) {
	input := "foo`bar\\baz"
	got := regexLiteral(input)
	// Contains backtick, so double-quote mode.
	if !strings.HasPrefix(got, `"`) {
		t.Fatalf("expected double-quote mode, got: %s", got)
	}
	// Backslash should be doubled.
	if !strings.Contains(got, `\\`) {
		t.Errorf("expected escaped backslash, got: %s", got)
	}
	// Backtick should be \x60.
	if !strings.Contains(got, `\x60`) {
		t.Errorf("expected \\x60 for backtick, got: %s", got)
	}
}

// ---------------------------------------------------------------------------
// goStringLiteral() tests
// ---------------------------------------------------------------------------

func TestGoStringLiteral_SimpleString(t *testing.T) {
	got := goStringLiteral("hello world")
	if got != `"hello world"` {
		t.Errorf("expected %q, got %s", `"hello world"`, got)
	}
}

func TestGoStringLiteral_QuotesEscaped(t *testing.T) {
	got := goStringLiteral(`say "hello"`)
	if got != `"say \"hello\""` {
		t.Errorf("expected quotes escaped, got: %s", got)
	}
}

func TestGoStringLiteral_BackslashesEscaped(t *testing.T) {
	got := goStringLiteral(`path\to\file`)
	if got != `"path\\to\\file"` {
		t.Errorf("expected backslashes escaped, got: %s", got)
	}
}

func TestGoStringLiteral_NewlinesEscaped(t *testing.T) {
	got := goStringLiteral("line1\nline2")
	if got != `"line1\nline2"` {
		t.Errorf("expected newlines escaped, got: %s", got)
	}
}

// ---------------------------------------------------------------------------
// generate() tests (end-to-end)
// ---------------------------------------------------------------------------

func TestGenerate_SmallSet(t *testing.T) {
	patterns := []Pattern{
		{ID: "p1", Order: 10, RE2Regex: `\bcd\s+`, PatternName: "cd command", Remediation: "use -C flag"},
		{ID: "p2", Order: 20, RE2Regex: `&&`, PatternName: "chaining", Remediation: "avoid &&"},
	}

	var buf bytes.Buffer
	if err := generate(&buf, patterns); err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	output := buf.String()

	// Verify the output is valid Go by checking key structural elements.
	if !strings.Contains(output, "package validator") {
		t.Error("output missing 'package validator'")
	}
	if !strings.Contains(output, `import "regexp"`) {
		t.Error("output missing import")
	}
}

func TestGenerate_ContainsDONOTEDIT(t *testing.T) {
	patterns := []Pattern{
		{ID: "p1", Order: 10, RE2Regex: `a`, PatternName: "test", Remediation: "fix"},
	}

	var buf bytes.Buffer
	if err := generate(&buf, patterns); err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	if !strings.Contains(buf.String(), "Code generated by generate-patterns; DO NOT EDIT.") {
		t.Error("output missing DO NOT EDIT header")
	}
}

func TestGenerate_CorrectPatternCount(t *testing.T) {
	patterns := []Pattern{
		{ID: "p1", Order: 10, RE2Regex: `a`, PatternName: "alpha", Remediation: "fix a"},
		{ID: "p2", Order: 20, RE2Regex: `b`, PatternName: "beta", Remediation: "fix b"},
		{ID: "p3", Order: 30, RE2Regex: `c`, PatternName: "gamma", Remediation: "fix c"},
	}

	var buf bytes.Buffer
	if err := generate(&buf, patterns); err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	output := buf.String()

	// Count MustCompile calls.
	count := strings.Count(output, "regexp.MustCompile(")
	if count != 3 {
		t.Errorf("expected 3 MustCompile calls, got %d", count)
	}

	// Count pattern name entries.
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if !strings.Contains(output, name) {
			t.Errorf("output missing pattern name %q", name)
		}
	}
}

func TestGenerate_BacktickPattern(t *testing.T) {
	patterns := []Pattern{
		{ID: "p1", Order: 10, RE2Regex: "echo `date`", PatternName: "backtick cmd", Remediation: "avoid backticks"},
	}

	var buf bytes.Buffer
	if err := generate(&buf, patterns); err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	output := buf.String()

	// Should use \x60 escaping, not raw backtick inside the regex literal.
	if !strings.Contains(output, `\x60`) {
		t.Error("expected \\x60 escaping for backtick regex")
	}

	// The MustCompile line should use double-quote, not backtick.
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "MustCompile(") {
			if strings.Contains(line, "MustCompile(`") {
				t.Error("backtick regex should not use backtick quoting in MustCompile")
			}
			break
		}
	}
}

// ---------------------------------------------------------------------------
// Integration: round-trip with actual YAML
// ---------------------------------------------------------------------------

func TestIntegration_RoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	// Locate the YAML relative to this test file's directory.
	// The generator lives at hooks/cmd/generate-patterns/ and the YAML at
	// patterns/bash-anti-patterns.yaml (three levels up from hooks/, then patterns/).
	yamlPath := filepath.Join("..", "..", "..", "patterns", "bash-anti-patterns.yaml")
	committedPath := filepath.Join("..", "..", "internal", "validator", "patterns.go")

	// Read YAML.
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("reading YAML: %v", err)
	}

	var db PatternDB
	if err := yaml.Unmarshal(data, &db); err != nil {
		t.Fatalf("parsing YAML: %v", err)
	}

	// Filter active patterns (same logic as main).
	var active []Pattern
	for _, p := range db.Patterns {
		if p.RE2Regex != "" && !p.Relaxed {
			active = append(active, p)
		}
	}

	// Sort by order ascending.
	sort.Slice(active, func(i, j int) bool {
		return active[i].Order < active[j].Order
	})

	// Validate.
	errs := validate(active)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("validation error: %s", e)
		}
		t.FailNow()
	}

	// Lint examples.
	lintErrs := lintExamples(active)
	for _, e := range lintErrs {
		t.Errorf("lint error: %s", e)
	}

	// Generate output.
	var buf bytes.Buffer
	if err := generate(&buf, active); err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	// Read committed patterns.go.
	committed, err := os.ReadFile(committedPath)
	if err != nil {
		t.Fatalf("reading committed patterns.go: %v", err)
	}

	if buf.String() != string(committed) {
		t.Error("generated output does not match committed patterns.go; run:\n  go run ./cmd/generate-patterns -yaml ../../patterns/bash-anti-patterns.yaml -output internal/validator/patterns.go")
	}

	// Verify all generated regexes compile as a sanity check.
	for _, p := range active {
		if _, err := regexp.Compile(p.RE2Regex); err != nil {
			t.Errorf("pattern %q regex does not compile: %v", p.ID, err)
		}
	}
}
