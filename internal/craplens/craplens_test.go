package craplens

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// TestRenderSilentWhenNotFlagged pins that an unflagged report produces no
// comment body at all, so the workflow cannot post an empty comment.
func TestRenderSilentWhenNotFlagged(t *testing.T) {
	if got := (Report{Scored: 12, WithinAgentTarget: 12}).Render(); got != "" {
		t.Errorf("Render() = %q, want empty", got)
	}
}

// TestRenderNamesTheProblem pins the parts of the comment a reader acts on:
// which function, its score, and the fact that the signal cannot block.
func TestRenderNamesTheProblem(t *testing.T) {
	body := Report{
		Threshold: 30,
		Scored:    4,
		Over: []Function{
			{File: "agm/cmd/agm-bus/main.go", Line: 99, Name: "cmdServe", Complexity: 46, Coverage: 0},
		},
		Untested: []Package{{ImportPath: "agm/cmd/agm-bus", New: true}},
	}.Render()

	for _, want := range []string{"cmdServe", "agm/cmd/agm-bus/main.go:99", "2162", "new package", "advisory", "never fails a check"} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered body is missing %q:\n%s", want, body)
		}
	}
}

// TestRenderDoesNotDuplicateOwnedChecks pins the harness-hygiene property that
// this signal states what it does NOT own, so a reader does not expect it to
// replace errcheck or gocyclo.
func TestRenderDoesNotDuplicateOwnedChecks(t *testing.T) {
	body := Report{Threshold: 30, Untested: []Package{{ImportPath: "x"}}}.Render()
	for _, want := range []string{"errcheck", "gocyclo"} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered body should name %q as the owner it defers to:\n%s", want, body)
		}
	}
}

// TestRenderBoundsEachList pins that a diff touching hundreds of functions
// still produces a comment somebody will read.
func TestRenderBoundsEachList(t *testing.T) {
	var over []Function
	for i := range 40 {
		over = append(over, Function{File: "a.go", Line: i, Name: "f", Complexity: 20, Coverage: 0})
	}
	body := Report{Threshold: 30, Over: over}.Render()

	if strings.Count(body, "| 420 |") > maxListed {
		t.Errorf("rendered %d rows, want at most %d", strings.Count(body, "| 420 |"), maxListed)
	}
	if !strings.Contains(body, "and 30 more") {
		t.Errorf("truncation was not disclosed:\n%s", body)
	}
}

// TestParseHunkHeader pins the diff arithmetic the whole signal rests on. A
// wrong head-side span attributes changes to the wrong function.
func TestParseHunkHeader(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		want   lineRange
		wantOK bool
	}{
		{name: "explicit count", line: "@@ -10,3 +20,5 @@ func x()", want: lineRange{20, 24}, wantOK: true},
		{name: "implicit single line", line: "@@ -10 +20 @@", want: lineRange{20, 20}, wantOK: true},
		{name: "pure deletion has no head lines", line: "@@ -10,3 +20,0 @@", wantOK: false},
		{name: "malformed", line: "@@ nonsense @@", wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseHunkHeader(tc.line)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Errorf("range = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestIsScorableGoFile pins the exclusions. Scoring test files would let a diff
// improve its own score by adding a branchy test.
func TestIsScorableGoFile(t *testing.T) {
	tests := map[string]bool{
		"agm/cmd/agm-bus/main.go":      true,
		"internal/craplens/diff.go":    true,
		"agm/cmd/agm-bus/main_test.go": false,
		"vendor/x/y.go":                false,
		"pkg/testdata/fixture.go":      false,
		"api/service.pb.go":            false,
		"pkg/x_generated.go":           false,
		"README.md":                    false,
	}

	for path, want := range tests {
		if got := isScorableGoFile(path); got != want {
			t.Errorf("isScorableGoFile(%q) = %v, want %v", path, got, want)
		}
	}
}

// TestParseUnifiedDiffTracksAddedFiles pins that a wholly-new file is
// recognised as added, which is what distinguishes a new package shipping
// untested from an existing untested package that got edited.
func TestParseUnifiedDiffTracksAddedFiles(t *testing.T) {
	out := strings.Join([]string{
		"diff --git a/pkg/new/thing.go b/pkg/new/thing.go",
		"new file mode 100644",
		"--- /dev/null",
		"+++ b/pkg/new/thing.go",
		"@@ -0,0 +1,12 @@",
		"diff --git a/pkg/old/thing.go b/pkg/old/thing.go",
		"index 111..222 100644",
		"--- a/pkg/old/thing.go",
		"+++ b/pkg/old/thing.go",
		"@@ -5,2 +5,3 @@",
		"diff --git a/pkg/old/thing_test.go b/pkg/old/thing_test.go",
		"--- a/pkg/old/thing_test.go",
		"+++ b/pkg/old/thing_test.go",
		"@@ -1,2 +1,9 @@",
	}, "\n")

	files := parseUnifiedDiff(out)

	if len(files) != 2 {
		t.Fatalf("parsed %d files, want 2 (the test file must be excluded): %v", len(files), sortedKeys(files))
	}
	if !files["pkg/new/thing.go"].Added {
		t.Error("pkg/new/thing.go should be marked added")
	}
	if files["pkg/old/thing.go"].Added {
		t.Error("pkg/old/thing.go is an edit, not an addition")
	}
	if got := files["pkg/old/thing.go"].Ranges; len(got) != 1 || got[0] != (lineRange{5, 7}) {
		t.Errorf("pkg/old/thing.go ranges = %+v, want [{5 7}]", got)
	}
}

// TestParseUnifiedDiffDropsPureDeletions pins that a file whose every hunk
// removed lines is not reported: it has no head-side line left to score.
func TestParseUnifiedDiffDropsPureDeletions(t *testing.T) {
	out := strings.Join([]string{
		"diff --git a/pkg/x/y.go b/pkg/x/y.go",
		"--- a/pkg/x/y.go",
		"+++ b/pkg/x/y.go",
		"@@ -5,9 +4,0 @@",
	}, "\n")

	if files := parseUnifiedDiff(out); len(files) != 0 {
		t.Errorf("parsed %v, want nothing for a pure deletion", sortedKeys(files))
	}
}

// TestPackageIsNew pins that a package counts as new only when every changed
// file in it was added, so adding one file to an existing package does not
// report the whole package as new.
func TestPackageIsNew(t *testing.T) {
	files := touchedSet{
		"pkg/fresh/a.go": {Path: "pkg/fresh/a.go", Added: true},
		"pkg/fresh/b.go": {Path: "pkg/fresh/b.go", Added: true},
		"pkg/mixed/a.go": {Path: "pkg/mixed/a.go", Added: true},
		"pkg/mixed/b.go": {Path: "pkg/mixed/b.go", Added: false},
	}

	if !files.packageIsNew("pkg/fresh") {
		t.Error("pkg/fresh should be new")
	}
	if files.packageIsNew("pkg/mixed") {
		t.Error("pkg/mixed has an edited file, so it is not new")
	}
	if files.packageIsNew("pkg/absent") {
		t.Error("a package the diff did not touch is not new")
	}
}

// TestComplexityMatchesGocyclo pins the counting rules against gocyclo's, since
// golangci-lint already runs gocyclo on this repository and two signals that
// disagree about the same function teach readers to trust neither.
func TestComplexityMatchesGocyclo(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{name: "straight line", body: "func f() { println(1) }", want: 1},
		{name: "one if", body: "func f(a bool) { if a { println(1) } }", want: 2},
		{name: "if else counts once", body: "func f(a bool) { if a { println(1) } else { println(2) } }", want: 2},
		{name: "for loop", body: "func f() { for i := 0; i < 3; i++ { println(i) } }", want: 2},
		{name: "range", body: "func f(xs []int) { for range xs { println(1) } }", want: 2},
		{name: "logical and", body: "func f(a, b bool) { if a && b { println(1) } }", want: 3},
		{name: "logical or", body: "func f(a, b bool) { if a || b { println(1) } }", want: 3},
		{name: "switch cases", body: "func f(n int) { switch n { case 1: case 2: case 3: } }", want: 4},
		{name: "default adds no branch", body: "func f(n int) { switch n { case 1: default: } }", want: 2},
		{name: "select default adds no branch", body: "func f(c chan int) { select { case <-c: default: } }", want: 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseSingleFunc(t, tc.body)
			if got := complexity(fn); got != tc.want {
				t.Errorf("complexity = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestFuncNameQualifiesMethods pins that two same-named methods on different
// receivers stay distinguishable in the report.
func TestFuncNameQualifiesMethods(t *testing.T) {
	tests := map[string]string{
		"func f() {}":                 "f",
		"func (s Server) Start() {}":  "(Server).Start",
		"func (s *Server) Start() {}": "(*Server).Start",
		"func (s *Store[T]) Get() {}": "(*Store).Get",
	}

	for body, want := range tests {
		if got := funcName(parseSingleFunc(t, body)); got != want {
			t.Errorf("funcName(%q) = %q, want %q", body, got, want)
		}
	}
}

// TestFunctionCoverageFromProfile pins the profile join: coverage is derived by
// intersecting profile blocks with the function's line span.
func TestFunctionCoverageFromProfile(t *testing.T) {
	data := coverageData{
		packages: map[string]packageCoverage{"pkg/x": {coverage: 0.5}},
		blocks: map[string][]profileBlock{
			"pkg/x/y.go": {
				{startLine: 10, endLine: 14, numStmt: 3, count: 1},
				{startLine: 15, endLine: 18, numStmt: 1, count: 0},
				{startLine: 40, endLine: 44, numStmt: 2, count: 0},
			},
		},
	}

	covered := data.functionCoverage(Function{File: "pkg/x/y.go", Line: 10, EndLine: 18})
	if math.Abs(covered-0.75) > 0.001 {
		t.Errorf("coverage of the first function = %.3f, want 0.75", covered)
	}

	uncovered := data.functionCoverage(Function{File: "pkg/x/y.go", Line: 40, EndLine: 44})
	if uncovered != 0 {
		t.Errorf("coverage of the second function = %.3f, want 0", uncovered)
	}
}

// TestFunctionCoverageUnknownForUnprofiledFile pins that a file absent from the
// profile is unknown rather than zero. This is the difference between "we could
// not measure this" and "this is untested", and reporting the second when the
// first is true is what would make the signal untrustworthy.
func TestFunctionCoverageUnknownForUnprofiledFile(t *testing.T) {
	data := coverageData{blocks: map[string][]profileBlock{}}
	if got := data.functionCoverage(Function{File: "pkg/x/y.go", Line: 1, EndLine: 9}); got != CoverageUnknown {
		t.Errorf("coverage = %v, want CoverageUnknown", got)
	}
}

// TestParseProfile pins reading a real coverage-profile body, including that
// per-package totals are statement-weighted rather than a mean of blocks.
func TestParseProfile(t *testing.T) {
	raw := strings.Join([]string{
		"mode: set",
		"github.com/example/mod/pkg/x/y.go:10.2,14.16 3 1",
		"github.com/example/mod/pkg/x/y.go:15.2,18.9 1 0",
		"github.com/example/mod/other/z.go:1.1,2.2 5 1",
	}, "\n")
	data := unmeasured([]string{"pkg/x"})

	parseProfile(raw, data)

	if got := data.packages["pkg/x"].coverage; math.Abs(got-0.75) > 0.001 {
		t.Errorf("pkg/x coverage = %.3f, want 0.75", got)
	}
	if len(data.blocks["pkg/x/y.go"]) != 2 {
		t.Errorf("parsed %d blocks for pkg/x/y.go, want 2", len(data.blocks["pkg/x/y.go"]))
	}
	if _, ok := data.blocks["other/z.go"]; ok {
		t.Error("a file outside the touched packages should not be recorded")
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
