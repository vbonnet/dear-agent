package collectors

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/aggregator"
)

const sampleGolangCILintJSON = `{
  "Issues": [
    {"FromLinter":"errcheck","Text":"...", "Pos":{"Filename":"a.go"}},
    {"FromLinter":"errcheck","Text":"...", "Pos":{"Filename":"a.go"}},
    {"FromLinter":"gocyclo","Text":"...", "Pos":{"Filename":"b.go"}}
  ]
}`

func TestParseGolangCILint(t *testing.T) {
	t.Parallel()
	sigs, err := parseGolangCILint([]byte(sampleGolangCILintJSON))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	bySubject := map[string]float64{}
	for _, s := range sigs {
		bySubject[s.Subject] = s.Value
		if s.Kind != aggregator.KindLintTrend {
			t.Errorf("Kind = %s, want lint_trend", s.Kind)
		}
	}
	if bySubject["a.go"] != 2 {
		t.Errorf("a.go count = %v, want 2", bySubject["a.go"])
	}
	if bySubject["b.go"] != 1 {
		t.Errorf("b.go count = %v, want 1", bySubject["b.go"])
	}
}

func TestParseGolangCILintEmpty(t *testing.T) {
	t.Parallel()
	sigs, err := parseGolangCILint(nil)
	if err != nil {
		t.Fatalf("parse(nil): %v", err)
	}
	if len(sigs) != 0 {
		t.Errorf("parse(nil) returned %d signals, want 0", len(sigs))
	}
}

func TestParseGolangCILintMalformed(t *testing.T) {
	t.Parallel()
	if _, err := parseGolangCILint([]byte("{not json")); err == nil {
		t.Error("malformed JSON should fail")
	}
}

// golangci-lint v2 appends a human-readable summary after the JSON
// document on stdout. Verify the parser stops at the JSON object's
// closing brace and ignores the trailing summary instead of failing
// with "invalid character '3' after top-level value".
func TestParseGolangCILintV2TrailingSummary(t *testing.T) {
	t.Parallel()
	const v2Output = `{"Issues":[{"FromLinter":"typecheck","Text":"declared and not used: x","Pos":{"Filename":"main.go"}}],"Report":{"Linters":[{"Name":"errcheck","Enabled":true}]}}
3 issues:
* typecheck: 3
`
	sigs, err := parseGolangCILint([]byte(v2Output))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(sigs) != 1 {
		t.Fatalf("got %d signals, want 1", len(sigs))
	}
	if sigs[0].Subject != "main.go" {
		t.Errorf("Subject = %q, want main.go", sigs[0].Subject)
	}
	if sigs[0].Value != 1 {
		t.Errorf("Value = %v, want 1", sigs[0].Value)
	}
}

func TestLintTrendInputFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := dir + "/lint.json"
	if err := writeFile(path, []byte(sampleGolangCILintJSON)); err != nil {
		t.Fatal(err)
	}
	c := &LintTrend{InputFile: path}
	sigs, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	subjects := make([]string, 0, len(sigs))
	for _, s := range sigs {
		subjects = append(subjects, s.Subject)
	}
	sort.Strings(subjects)
	if strings.Join(subjects, ",") != "a.go,b.go" {
		t.Errorf("subjects = %v, want [a.go b.go]", subjects)
	}
}

func TestLintTrendToolMissing(t *testing.T) {
	t.Parallel()
	c := &LintTrend{Repo: "/r", LookPathFn: missingLookPath}
	_, err := c.Collect(context.Background())
	if !aggregator.IsToolMissing(err) {
		t.Errorf("expected ErrToolMissing, got %v", err)
	}
}

func TestLintTrendNoRepoNoFile(t *testing.T) {
	t.Parallel()
	c := &LintTrend{}
	if _, err := c.Collect(context.Background()); err == nil {
		t.Error("missing both Repo and InputFile should fail")
	}
}
