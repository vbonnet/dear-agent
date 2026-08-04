package earslint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newDefault(t *testing.T) *Linter {
	t.Helper()
	l, err := New(Config{})
	if err != nil {
		t.Fatalf("New(default): %v", err)
	}
	return l
}

func TestLint_ValidPatterns(t *testing.T) {
	l := newDefault(t)
	cases := map[string]string{
		"event-driven":   "When the user submits the form, the system shall validate all fields.",
		"state-driven":   "While the connection is active, the system shall stream telemetry.",
		"feature-driven": "Where the premium feature is enabled, the system shall show analytics.",
		"option":         "If the token is expired, then the system shall reject the request.",
		"unwanted":       "The system shall not store plaintext passwords.",
		"ubiquitous":     "The system shall log every authentication attempt.",
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			res, err := l.Lint("SPEC.md", strings.NewReader(line))
			if err != nil {
				t.Fatalf("Lint: %v", err)
			}
			if res.ValidRequirements != 1 {
				t.Errorf("want 1 valid requirement, got %d (findings: %v)", res.ValidRequirements, res.Findings)
			}
			if res.NonConforming() != 0 {
				t.Errorf("want 0 non-conforming, got %d", res.NonConforming())
			}
			if res.Failed(true) {
				t.Errorf("valid requirement should not fail even in strict mode")
			}
		})
	}
}

func TestLint_MarkdownListAndEmphasis(t *testing.T) {
	l := newDefault(t)
	doc := strings.Join([]string{
		"# Requirements",
		"",
		"- When the user logs in, the system shall create a session.",
		"1. The system shall not leak credentials.",
		"* **While** the job runs, the system shall report progress.",
	}, "\n")
	res, err := l.Lint("SPEC.md", strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if res.ValidRequirements != 3 {
		t.Fatalf("want 3 valid, got %d (findings: %v)", res.ValidRequirements, res.Findings)
	}
}

func TestNormalizeRequirementLine(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "bold list", raw: "- **REQ-01** When a request arrives, the system shall respond.", want: "REQ-01 When a request arrives, the system shall respond."},
		{name: "numbered code", raw: "2) `REQ-02` The system shall retain user_id and a * b.", want: "REQ-02 The system shall retain user_id and a * b."},
		{name: "heading emphasis", raw: "### __REQ-03__ While a job runs, the system shall report progress.", want: "REQ-03 While a job runs, the system shall report progress."},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := NormalizeRequirementLine(test.raw); got != test.want {
				t.Fatalf("NormalizeRequirementLine() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLint_SingleEmphasisAndBackticks(t *testing.T) {
	l := newDefault(t)
	doc := strings.Join([]string{
		"- `When` the request arrives, the system shall respond.",
		"- _While_ the job runs, the system shall report progress.",
	}, "\n")
	res, err := l.Lint("SPEC.md", strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if res.ValidRequirements != 2 {
		t.Fatalf("want 2 valid (backtick/underscore wrapped), got %d (findings: %v)",
			res.ValidRequirements, res.Findings)
	}
	if res.NonConforming() != 0 {
		t.Errorf("want 0 non-conforming, got %d", res.NonConforming())
	}
}

func TestLint_OptionWithoutThen(t *testing.T) {
	l := newDefault(t)
	// EARS "option" form, written without the optional "then".
	res, err := l.Lint("SPEC.md", strings.NewReader("If the token is expired, the system shall reject the request."))
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if res.ValidRequirements != 1 {
		t.Fatalf("want 1 valid (no-then option), got %d (findings: %v)",
			res.ValidRequirements, res.Findings)
	}
	if res.NonConforming() != 0 {
		t.Errorf("want 0 non-conforming, got %d", res.NonConforming())
	}
}

func TestLint_NonConforming(t *testing.T) {
	l := newDefault(t)
	doc := strings.Join([]string{
		"The system shall log requests.",           // valid ubiquitous
		"Eventually the thing shall work somehow.", // non-conforming (no "the X shall")
	}, "\n")
	res, err := l.Lint("SPEC.md", strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if res.ValidRequirements != 1 {
		t.Errorf("want 1 valid, got %d", res.ValidRequirements)
	}
	if res.NonConforming() != 1 {
		t.Errorf("want 1 non-conforming, got %d", res.NonConforming())
	}
	// Non-strict: a non-conforming line alongside a valid one does NOT fail.
	if res.Failed(false) {
		t.Errorf("non-strict should not fail when at least one valid requirement exists")
	}
	// Strict: any non-conforming requirement fails.
	if !res.Failed(true) {
		t.Errorf("strict should fail on non-conforming requirement")
	}
	// The non-conforming line should be reported with its text and line number.
	var found bool
	for _, f := range res.Findings {
		if f.Line == 2 && f.Severity == SeverityError && strings.Contains(f.Text, "Eventually") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a finding for the non-conforming line 2, got %+v", res.Findings)
	}
}

// TestLint_PreservesSnakeCaseAndMath guards against normalization corrupting
// intra-word underscores (snake_case identifiers) or spaced asterisks (math /
// wildcards) when removing markdown emphasis. The reported finding text must
// match the source requirement verbatim.
func TestLint_PreservesSnakeCaseAndMath(t *testing.T) {
	l := newDefault(t)
	doc := "Eventually the system shall update the user_id when count > a * b."
	res, err := l.Lint("SPEC.md", strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if len(res.Findings) == 0 {
		t.Fatal("expected a finding for the non-conforming requirement")
	}
	got := res.Findings[0].Text
	want := "Eventually the system shall update the user_id when count > a * b."
	if got != want {
		t.Errorf("normalization corrupted snake_case/math: got %q, want %q", got, want)
	}
}

func TestLint_ZeroRequirements(t *testing.T) {
	l := newDefault(t)
	doc := "# Spec\n\nThis is just prose with no requirements.\n"
	res, err := l.Lint("SPEC.md", strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if res.ValidRequirements != 0 {
		t.Errorf("want 0 valid, got %d", res.ValidRequirements)
	}
	if !res.Failed(false) {
		t.Errorf("zero-requirement file must fail even in non-strict mode")
	}
	var hasZeroFinding bool
	for _, f := range res.Findings {
		if strings.Contains(f.Message, "no valid EARS requirements") {
			hasZeroFinding = true
		}
	}
	if !hasZeroFinding {
		t.Errorf("expected a zero-requirements finding, got %+v", res.Findings)
	}
}

func TestLint_IgnoresFencedCode(t *testing.T) {
	l := newDefault(t)
	doc := strings.Join([]string{
		"The system shall expose an API.",
		"```go",
		"// the parser shall not be confused by this comment",
		"var shall = true",
		"```",
		"~~~",
		"the tilde fence shall also be ignored",
		"~~~",
	}, "\n")
	res, err := l.Lint("SPEC.md", strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if res.TotalRequirements != 1 || res.ValidRequirements != 1 {
		t.Errorf("fenced code should be ignored: total=%d valid=%d findings=%v",
			res.TotalRequirements, res.ValidRequirements, res.Findings)
	}
}

func TestLint_CustomKeyword(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RequirementKeyword = "must"
	cfg.Patterns = []Pattern{{Name: "ubiquitous-must", Regex: `(?i)^the\s+.+\s+must\s+.+`}}
	l, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := l.Lint("SPEC.md", strings.NewReader("The system must persist state."))
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if res.ValidRequirements != 1 {
		t.Errorf("custom keyword: want 1 valid, got %d", res.ValidRequirements)
	}
}

func TestNew_InvalidRegex(t *testing.T) {
	_, err := New(Config{Patterns: []Pattern{{Name: "bad", Regex: "("}}})
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
}

func TestLintFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SPEC.md")
	if err := os.WriteFile(path, []byte("The system shall work.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	l := newDefault(t)
	res, err := l.LintFile(path)
	if err != nil {
		t.Fatalf("LintFile: %v", err)
	}
	if res.ValidRequirements != 1 {
		t.Errorf("want 1 valid, got %d", res.ValidRequirements)
	}
	if res.File != path {
		t.Errorf("want File=%s, got %s", path, res.File)
	}
}

func TestLintFile_Missing(t *testing.T) {
	l := newDefault(t)
	if _, err := l.LintFile(filepath.Join(t.TempDir(), "nope.md")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

// TestLint_IDPrefixedRequirements verifies that bold-markdown ID prefixes like
// **EBD-01** or **FSG-03** are accepted by all six EARS templates after markdown
// stripping leaves "EBD-01 When ..." in the candidate text.
func TestLint_IDPrefixedRequirements(t *testing.T) {
	l := newDefault(t)
	cases := map[string]string{
		"event-driven":   "**EBD-01** When the user submits the form, the system shall validate all fields.",
		"state-driven":   "**EBD-02** While the connection is active, the system shall stream telemetry.",
		"feature-driven": "**EBD-03** Where the premium feature is enabled, the system shall show analytics.",
		"option":         "**EBD-04** If the token is expired, then the system shall reject the request.",
		"unwanted":       "**FSG-01** The system shall not store plaintext passwords.",
		"ubiquitous":     "**FSG-02** The system shall log every authentication attempt.",
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			res, err := l.Lint("SPEC.md", strings.NewReader(line))
			if err != nil {
				t.Fatalf("Lint: %v", err)
			}
			if res.ValidRequirements != 1 {
				t.Errorf("ID-prefixed %s: want 1 valid, got %d (findings: %v)", name, res.ValidRequirements, res.Findings)
			}
			if res.NonConforming() != 0 {
				t.Errorf("ID-prefixed %s: want 0 non-conforming, got %d", name, res.NonConforming())
			}
		})
	}
}
