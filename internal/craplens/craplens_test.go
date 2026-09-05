package craplens

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

// TestCRAPFormula pins the Crap4j formula at the points that matter: full
// coverage collapses to the complexity, no coverage is complexity squared plus
// complexity, and partial coverage lands between them.
func TestCRAPFormula(t *testing.T) {
	tests := []struct {
		name       string
		complexity int
		coverage   float64
		want       float64
	}{
		{name: "fully covered collapses to complexity", complexity: 10, coverage: 1, want: 10},
		{name: "uncovered is c^2+c", complexity: 10, coverage: 0, want: 110},
		{name: "trivial and uncovered", complexity: 1, coverage: 0, want: 2},
		{name: "half covered", complexity: 10, coverage: 0.5, want: 22.5},
		{name: "the audit's worst finding", complexity: 46, coverage: 0, want: 2162},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Function{Complexity: tc.complexity, Coverage: tc.coverage}.CRAP()
			if math.Abs(got-tc.want) > 0.01 {
				t.Errorf("CRAP(c=%d, cov=%.2f) = %.2f, want %.2f", tc.complexity, tc.coverage, got, tc.want)
			}
		})
	}
}

// TestCRAPUnknownCoverageIsNotScored pins the safety property that keeps this
// signal trustworthy: a package whose coverage could not be measured must not
// be scored as if it were untested.
func TestCRAPUnknownCoverageIsNotScored(t *testing.T) {
	f := Function{Complexity: 40, Coverage: CoverageUnknown}
	if got := f.CRAP(); got != CoverageUnknown {
		t.Errorf("CRAP with unknown coverage = %v, want CoverageUnknown", got)
	}
}

// TestFlagged pins when the signal speaks. Silence is the normal outcome, and
// a checkout mismatch must stay silent because nothing was measured.
func TestFlagged(t *testing.T) {
	tests := []struct {
		name   string
		report Report
		want   bool
	}{
		{name: "empty", report: Report{}, want: false},
		{name: "over threshold", report: Report{Over: []Function{{}}}, want: true},
		{name: "untested package", report: Report{Untested: []Package{{}}}, want: true},
		{name: "unknown coverage alone", report: Report{Unknown: []string{"a"}}, want: false},
		{
			name:   "checkout mismatch stays silent",
			report: Report{Over: []Function{{}}, Untested: []Package{{}}, CheckoutMismatch: true},
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.report.Flagged(); got != tc.want {
				t.Errorf("Flagged() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestAnalyzeRequiresRevisions pins the usage error.
func TestAnalyzeRequiresRevisions(t *testing.T) {
	for _, tc := range []struct{ base, head string }{{"", "HEAD"}, {"HEAD", ""}, {"", ""}} {
		if _, err := Analyze(t.Context(), "", tc.base, tc.head, 0); err == nil {
			t.Errorf("Analyze(base=%q, head=%q) = nil error, want a usage error", tc.base, tc.head)
		}
	}
}

// TestAnalyzeEndToEnd builds a throwaway repository with one tested package and
// one untested one, then asserts the signal names the untested one and scores
// its branchy function. This is the test that would fail if any stage of the
// pipeline stopped joining diff, complexity, and coverage.
func TestAnalyzeEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain unavailable")
	}
	repo := newTestRepo(t)

	report, err := Analyze(t.Context(), repo, "base", "HEAD", DefaultThreshold)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if report.CheckoutMismatch {
		t.Fatal("the fixture checkout is at head; coverage should have been measured")
	}

	var untested []string
	for _, p := range report.Untested {
		untested = append(untested, p.ImportPath)
	}
	if len(untested) != 1 || untested[0] != "risky" {
		t.Errorf("untested packages = %v, want [risky]", untested)
	}
	if !report.Untested[0].New {
		t.Error("risky is wholly new in this diff and should be reported as a new package")
	}

	var names []string
	for _, f := range report.Over {
		names = append(names, f.Name)
	}
	if len(names) != 1 || names[0] != "Classify" {
		t.Fatalf("functions over threshold = %v, want [Classify]", names)
	}
	if report.Over[0].Coverage != 0 {
		t.Errorf("Classify coverage = %v, want 0", report.Over[0].Coverage)
	}
	if report.Scored == 0 {
		t.Error("the tested package's function should have been scored")
	}
	if report.WithinAgentTarget == 0 {
		t.Error("the tested package's simple function should be within the agent target")
	}
}

// TestAnalyzeSkipsCoverageOnCheckoutMismatch pins the honest degradation: with
// the working tree on a different revision than head, nothing is scored and
// nothing is flagged, rather than numbers derived from the wrong source.
func TestAnalyzeSkipsCoverageOnCheckoutMismatch(t *testing.T) {
	repo := newTestRepo(t)
	gittest.Run(t, repo, "checkout", "-q", "base")

	report, err := Analyze(t.Context(), repo, "base", "main", DefaultThreshold)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if !report.CheckoutMismatch {
		t.Fatal("expected a checkout mismatch to be reported")
	}
	if report.Flagged() {
		t.Error("a report with nothing measured must not be flagged")
	}
	if report.Changed == 0 {
		t.Error("changed functions should still be counted")
	}
}

// TestAnalyzeIgnoresUnrelatedBuildArtifacts covers the checkout this
// repository's own `make preflight` leaves behind: an ignored `build/`
// directory of compiled binaries. Treating any ignored path as dirty made the
// local lens this repository documents refuse to measure coverage until that
// directory was deleted, even though it cannot affect `go test` for any
// touched package.
func TestAnalyzeIgnoresUnrelatedBuildArtifacts(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain unavailable")
	}
	repo := newTestRepo(t)
	write(t, repo, ".gitignore", "/build/\n")
	gittest.Run(t, repo, "add", ".gitignore")
	gittest.Run(t, repo, "commit", "-q", "-m", "ignore build/")
	write(t, repo, "build/agm", "not a go file\n")

	report, err := Analyze(t.Context(), repo, "base", "HEAD", DefaultThreshold)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if report.CheckoutMismatch {
		t.Fatal("an ignored build artifact outside every touched package must not be treated as a dirty checkout")
	}
}

// TestAnalyzeTreatsNoStatementPackageAsMeasuredNotUnknown is the
// collectCoverage-level counterpart to
// TestParseProfileTreatsZeroStatementPackageAsMeasured: a package whose only
// source declares a type (no function body has a single executable
// statement) still runs its test and passes, but `go test -coverprofile`
// writes nothing for it beyond the leading `mode: set` line. Because
// parseProfile only ever populates a package's coverage while walking
// profile lines, such a package never appears in its perPackage map at all
// and must not be silently left at collectCoverage's pre-run
// CoverageUnknown default.
func TestAnalyzeTreatsNoStatementPackageAsMeasuredNotUnknown(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain unavailable")
	}
	dir := t.TempDir()
	gittest.Run(t, dir, "init", "-q", "-b", "main")
	gittest.Run(t, dir, "config", "user.email", "test@example.invalid")
	gittest.Run(t, dir, "config", "user.name", "Test")
	write(t, dir, "go.mod", "module example.test\n\ngo 1.24\n")
	gittest.Run(t, dir, "add", "-A")
	gittest.Run(t, dir, "commit", "-q", "-m", "base")
	gittest.Run(t, dir, "branch", "base")

	write(t, dir, "onlytype/onlytype.go", `package onlytype

// Marker has no methods; nothing in this package's production source has a
// single executable statement to instrument.
type Marker struct{}
`)
	write(t, dir, "onlytype/onlytype_test.go", `package onlytype

import "testing"

func TestMarkerExists(t *testing.T) {
	_ = Marker{}
}
`)
	gittest.Run(t, dir, "add", "-A")
	gittest.Run(t, dir, "commit", "-q", "-m", "add onlytype")

	report, err := Analyze(t.Context(), dir, "base", "HEAD", DefaultThreshold)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if report.CheckoutMismatch {
		t.Fatal("the fixture checkout is at head; coverage should have been measured")
	}
	if slices.Contains(report.Unknown, "onlytype") {
		t.Fatal("a package with no countable statements was left unmeasured instead of treated as fully covered")
	}
}

// newTestRepo builds a throwaway git repository with a base commit and a head
// commit that adds one tested package and one untested, branchy package.
func newTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	gittest.Run(t, dir, "init", "-q", "-b", "main")
	gittest.Run(t, dir, "config", "user.email", "test@example.invalid")
	gittest.Run(t, dir, "config", "user.name", "Test")

	write(t, dir, "go.mod", "module example.test\n\ngo 1.24\n")
	write(t, dir, "README.md", "seed\n")
	gittest.Run(t, dir, "add", "-A")
	gittest.Run(t, dir, "commit", "-q", "-m", "base")
	gittest.Run(t, dir, "branch", "base")

	write(t, dir, "safe/safe.go", `package safe

// Add is simple and fully covered.
func Add(a, b int) int {
	return a + b
}
`)
	write(t, dir, "safe/safe_test.go", `package safe

import "testing"

func TestAdd(t *testing.T) {
	if Add(1, 2) != 3 {
		t.Fatal("bad")
	}
}
`)
	write(t, dir, "risky/risky.go", `package risky

// Classify is branchy and has no test at all.
func Classify(n int, a, b, c, d bool) string {
	if n > 10 && a {
		return "high-a"
	}
	if n > 10 || b {
		return "high-b"
	}
	if n > 5 && c {
		return "mid-c"
	}
	if n > 5 || d {
		return "mid-d"
	}
	switch n {
	case 1:
		return "one"
	case 2:
		return "two"
	case 3:
		return "three"
	}
	for i := range n {
		if i > 100 {
			return "big"
		}
	}
	return "low"
}
`)
	gittest.Run(t, dir, "add", "-A")
	gittest.Run(t, dir, "commit", "-q", "-m", "head")
	return dir
}

// TestAnalyzeTreatsExistingPackageWithQuotedBaseFilenameAsNotNew guards
// against a real regression: text-mode `git ls-tree` C-quotes a base-side
// filename containing a non-ASCII byte, and the retained closing quote made
// the .go suffix check false, so a package whose only pre-existing source
// needs quoting was mislabeled as brand new the moment any file was added to
// it.
func TestAnalyzeTreatsExistingPackageWithQuotedBaseFilenameAsNotNew(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain unavailable")
	}
	dir := t.TempDir()
	gittest.Run(t, dir, "init", "-q", "-b", "main")
	gittest.Run(t, dir, "config", "user.email", "test@example.invalid")
	gittest.Run(t, dir, "config", "user.name", "Test")
	write(t, dir, "go.mod", "module example.test\n\ngo 1.24\n")
	write(t, dir, "existing/café.go", "package existing\n")
	gittest.Run(t, dir, "add", "-A")
	gittest.Run(t, dir, "commit", "-q", "-m", "base")
	gittest.Run(t, dir, "branch", "base")

	write(t, dir, "existing/new.go", `package existing

// Classify is branchy and has no test at all.
func Classify(n int, a, b, c, d bool) string {
	if n > 10 && a {
		return "high-a"
	}
	if n > 10 || b {
		return "high-b"
	}
	if n > 5 && c {
		return "mid-c"
	}
	if n > 5 || d {
		return "mid-d"
	}
	return "low"
}
`)
	gittest.Run(t, dir, "add", "-A")
	gittest.Run(t, dir, "commit", "-q", "-m", "add new.go to an existing package")

	report, err := Analyze(t.Context(), dir, "base", "HEAD", DefaultThreshold)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if report.CheckoutMismatch {
		t.Fatal("the fixture checkout is at head; coverage should have been measured")
	}
	var found *Package
	for i, p := range report.Untested {
		if p.ImportPath == "existing" {
			found = &report.Untested[i]
		}
	}
	if found == nil {
		t.Fatal("the existing package's untested new function should have been reported")
	}
	if found.New {
		t.Error("a package whose only pre-existing file needs quoting was mislabeled as new")
	}
}

// TestAnalyzeMeasuresGoSourceMarkedBinaryInGitattributes guards against a
// real regression: a head-tree .gitattributes marking *.go as binary made
// `git diff` emit only "Binary files ... differ" with no +++ header or
// hunk, so parseUnifiedDiff saw an empty touched set and Analyze reported a
// clean, fully-known verdict for a PR it never actually measured.
func TestAnalyzeMeasuresGoSourceMarkedBinaryInGitattributes(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain unavailable")
	}
	dir := t.TempDir()
	gittest.Run(t, dir, "init", "-q", "-b", "main")
	gittest.Run(t, dir, "config", "user.email", "test@example.invalid")
	gittest.Run(t, dir, "config", "user.name", "Test")
	write(t, dir, "go.mod", "module example.test\n\ngo 1.24\n")
	gittest.Run(t, dir, "add", "-A")
	gittest.Run(t, dir, "commit", "-q", "-m", "base")
	gittest.Run(t, dir, "branch", "base")

	write(t, dir, ".gitattributes", "*.go binary\n")
	write(t, dir, "risky/risky.go", `package risky

// Classify is branchy and has no test at all.
func Classify(n int, a, b, c, d bool) string {
	if n > 10 && a {
		return "high-a"
	}
	if n > 10 || b {
		return "high-b"
	}
	if n > 5 && c {
		return "mid-c"
	}
	if n > 5 || d {
		return "mid-d"
	}
	return "low"
}
`)
	gittest.Run(t, dir, "add", "-A")
	gittest.Run(t, dir, "commit", "-q", "-m", "add a binary-marked package")

	report, err := Analyze(t.Context(), dir, "base", "HEAD", DefaultThreshold)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if report.CheckoutMismatch {
		t.Fatal("the fixture checkout is at head; coverage should have been measured")
	}
	var names []string
	for _, p := range report.Untested {
		names = append(names, p.ImportPath)
	}
	if len(names) != 1 || names[0] != "risky" {
		t.Fatalf("untested packages = %v, want [risky]: a .gitattributes binary marker on *.go silently emptied the touched set", names)
	}
}

func write(t *testing.T, dir, rel, body string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
