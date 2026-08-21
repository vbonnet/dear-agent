package craplens

import (
	"go/parser"
	"go/token"
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

// TestAllFilesAdded pins the diff-side half of the new-package decision. It is
// necessary but not sufficient on its own: the caller also confirms the
// directory held no Go source at the base, because adding one file to an
// existing package also makes every changed file an addition.
func TestAllFilesAdded(t *testing.T) {
	files := touchedSet{
		"pkg/fresh/a.go": {Path: "pkg/fresh/a.go", Added: true},
		"pkg/fresh/b.go": {Path: "pkg/fresh/b.go", Added: true},
		"pkg/mixed/a.go": {Path: "pkg/mixed/a.go", Added: true},
		"pkg/mixed/b.go": {Path: "pkg/mixed/b.go", Added: false},
	}

	if !files.allFilesAdded("pkg/fresh") {
		t.Error("pkg/fresh should be new")
	}
	if files.allFilesAdded("pkg/mixed") {
		t.Error("pkg/mixed has an edited file, so it is not new")
	}
	if files.allFilesAdded("pkg/absent") {
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
		// Verified against the gocyclo binary: it walks the whole declaration
		// and counts branches inside a nested closure toward the enclosing
		// function. Scoring the closure separately instead would report 2 here
		// and break the parity CRAPLENS-05 promises.
		{
			name: "nested closure counts toward the enclosing function",
			body: "func f(a bool) func(int) int { if a { return nil }; return func(n int) int { if n > 1 { return 1 }; if n > 2 { return 2 }; if n > 3 { return 3 }; return 0 } }",
			want: 5,
		},
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

	parseProfile(raw, "github.com/example/mod", data)

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

// TestDeclaredFuncsMatchesGocycloScope pins WHICH declarations are scored,
// which is a separate question from how each one is counted.
//
// Both answers below were taken from the gocyclo binary rather than assumed:
// it reports a directly-assigned package-level function literal, and it does
// not report one nested inside a composite literal (the cobra RunE shape).
// Scoring the composite-literal case here would diverge from the linter this
// package promises parity with, so the blind spot is shared deliberately.
func TestDeclaredFuncsMatchesGocycloScope(t *testing.T) {
	src := `package p

type cmd struct{ RunE func(int) error }

func Plain() {}

var Bare = func(n int) int {
	if n > 1 {
		return 1
	}
	return 0
}

var Composite = &cmd{RunE: func(n int) error { return nil }}
`
	file, err := parser.ParseFile(token.NewFileSet(), "p.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	var names []string
	for _, decl := range file.Decls {
		for _, cand := range declaredFuncs(decl) {
			names = append(names, cand.name)
		}
	}

	want := []string{"Plain", "Bare"}
	if len(names) != len(want) {
		t.Fatalf("scored %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("scored[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}

// TestFailedPackagesDistinguishesFailFromNoTests is the regression test for
// the trust property this signal rests on. A package whose tests FAILED may
// still have left the profile blocks it reached before failing, so scoring it
// would understate its coverage and could report well-tested code as untested.
// A package with NO test files is the opposite: zero coverage is the true
// answer and reporting it is the entire point.
//
// Parsing go test's human output could not tell these apart reliably, which
// is why the collector reads -json events instead.
func TestFailedPackagesDistinguishesFailFromNoTests(t *testing.T) {
	const mod = "example.test"
	events := strings.Join([]string{
		`{"Action":"pass","Package":"example.test/passing"}`,
		`{"Action":"fail","Package":"example.test/failing"}`,
		`{"Action":"skip","Package":"example.test/notests"}`,
		`not json at all`,
		``,
	}, "\n")

	got := failedPackages(events, mod, []string{"passing", "failing", "notests", "silent"})

	want := map[string]bool{"failing": true, "silent": true}
	if len(got) != len(want) {
		t.Fatalf("unmeasured = %v, want exactly %v", got, []string{"failing", "silent"})
	}
	for _, dir := range got {
		if !want[dir] {
			t.Errorf("%q was reported unmeasured; a passing or test-free package must be measured", dir)
		}
	}
}

// TestFailedPackagesTreatsLateFailureAsUnmeasured covers a package that emits
// a passing test event and then fails overall, which is what a panic after
// some tests pass looks like.
func TestFailedPackagesTreatsLateFailureAsUnmeasured(t *testing.T) {
	events := strings.Join([]string{
		`{"Action":"pass","Package":"example.test/flaky"}`,
		`{"Action":"fail","Package":"example.test/flaky"}`,
	}, "\n")

	got := failedPackages(events, "example.test", []string{"flaky"})
	if len(got) != 1 || got[0] != "flaky" {
		t.Errorf("unmeasured = %v, want [flaky]: a package that ends in failure must not be scored", got)
	}
}

// TestPackageDirOfPrefersTheLongestMatch pins the ambiguity this repository
// actually contains: internal/tokens and engram/internal/tokens both exist, so
// an import path for the longer one is a suffix match for both.
func TestPackageDirOfPrefersTheLongestMatch(t *testing.T) {
	dirs := []string{"internal/tokens", "engram/internal/tokens"}

	got, ok := packageDirOf("github.com/x/y/engram/internal/tokens", "", dirs)
	if !ok || got != "engram/internal/tokens" {
		t.Errorf("packageDirOf = %q (%v), want engram/internal/tokens", got, ok)
	}

	got, ok = packageDirOf("github.com/x/y/internal/tokens", "", dirs)
	if !ok || got != "internal/tokens" {
		t.Errorf("packageDirOf = %q (%v), want internal/tokens", got, ok)
	}
}

// TestMatchPackageUsesTheModulePathExactly pins that a known module path
// resolves profile entries by prefix rather than by suffix guessing, including
// for the module root.
func TestMatchPackageUsesTheModulePathExactly(t *testing.T) {
	pkgs := map[string]packageCoverage{
		"internal/tokens":        {},
		"engram/internal/tokens": {},
		".":                      {},
	}

	dir, rel, ok := matchPackage(pkgs, "example.test", "example.test/engram/internal/tokens/a.go")
	if !ok || dir != "engram/internal/tokens" || rel != "engram/internal/tokens/a.go" {
		t.Errorf("nested = (%q, %q, %v)", dir, rel, ok)
	}

	dir, rel, ok = matchPackage(pkgs, "example.test", "example.test/root.go")
	if !ok || dir != "." || rel != "root.go" {
		t.Errorf("root = (%q, %q, %v), want (\".\", \"root.go\", true)", dir, rel, ok)
	}

	if _, _, ok = matchPackage(pkgs, "example.test", "example.test/untouched/a.go"); ok {
		t.Error("a package the diff did not touch must not match")
	}
}

// TestParseProfileTreatsZeroStatementPackageAsMeasured covers a profiled
// package holding no counted statements: that is measured at full coverage,
// not unmeasured, so its functions are scored rather than silently skipped.
func TestParseProfileTreatsZeroStatementPackageAsMeasured(t *testing.T) {
	raw := "mode: set\nexample.test/empty/e.go:1.1,2.2 0 0\n"
	data := unmeasured([]string{"empty"})

	parseProfile(raw, "example.test", data)

	if got := data.packages["empty"].coverage; got != 1 {
		t.Errorf("coverage = %v, want 1 for a package with no counted statements", got)
	}
}

// TestIsGeneratedSource pins detection by the standard toolchain marker rather
// than by filename. CRAPLENS-02 excludes generated files, and this repository
// contains generated source whose name matches no generated-looking suffix.
func TestIsGeneratedSource(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{
			name: "standard marker",
			src:  "// Code generated by tool. DO NOT EDIT.\n\npackage p\n",
			want: true,
		},
		{
			name: "marker with a different generator",
			src:  "// Code generated from patterns.yaml; DO NOT EDIT.\n\npackage p\n",
			want: true,
		},
		{name: "handwritten", src: "// Package p does a thing.\npackage p\n", want: false},
		{name: "prose mentioning generation", src: "// This code generated a report once.\npackage p\n", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isGeneratedSource([]byte(tc.src)); got != tc.want {
				t.Errorf("isGeneratedSource = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestMatchPackageSuffixFallback covers the path taken when the module path
// could not be determined (a `go list -m` failure), where resolution falls
// back to a longest-suffix match.
//
// Added because the signal in this PR flagged matchPackage at CRAP 39.6: the
// module-path branch was covered and the fallback was not.
func TestMatchPackageSuffixFallback(t *testing.T) {
	pkgs := map[string]packageCoverage{
		"internal/tokens":        {},
		"engram/internal/tokens": {},
		".":                      {},
	}

	tests := []struct {
		name     string
		fullPath string
		wantDir  string
		wantRel  string
		wantOK   bool
	}{
		{
			name:     "longest suffix wins over the shorter one",
			fullPath: "github.com/x/y/engram/internal/tokens/a.go",
			wantDir:  "engram/internal/tokens",
			wantRel:  "engram/internal/tokens/a.go",
			wantOK:   true,
		},
		{
			name:     "shorter package still matches its own path",
			fullPath: "github.com/x/y/internal/tokens/b.go",
			wantDir:  "internal/tokens",
			wantRel:  "internal/tokens/b.go",
			wantOK:   true,
		},
		{
			name:     "exact directory match",
			fullPath: "internal/tokens/c.go",
			wantDir:  "internal/tokens",
			wantRel:  "internal/tokens/c.go",
			wantOK:   true,
		},
		{
			name:     "untouched package does not match",
			fullPath: "github.com/x/y/other/d.go",
			wantOK:   false,
		},
		{
			name:     "root package is never matched by suffix",
			fullPath: "github.com/x/y/e.go",
			wantOK:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir, rel, ok := matchPackage(pkgs, "", tc.fullPath)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (dir=%q)", ok, tc.wantOK, dir)
			}
			if !ok {
				return
			}
			if dir != tc.wantDir || rel != tc.wantRel {
				t.Errorf("got (%q, %q), want (%q, %q)", dir, rel, tc.wantDir, tc.wantRel)
			}
		})
	}
}

// TestMatchPackageModulePathRejectsForeignPaths covers the branch where a
// module path is known but the profile entry belongs to a dependency rather
// than this module.
func TestMatchPackageModulePathRejectsForeignPaths(t *testing.T) {
	pkgs := map[string]packageCoverage{"internal/tokens": {}}

	if _, _, ok := matchPackage(pkgs, "example.test", "other.module/internal/tokens/a.go"); ok {
		t.Error("a path outside the module must not match when the module path is known")
	}
	if _, _, ok := matchPackage(pkgs, "example.test", "example.test/untouched/a.go"); ok {
		t.Error("a package the diff did not touch must not match")
	}
}

// TestPackageDirOfHandlesTheModuleRoot covers root-package resolution and the
// rejection of an empty import path.
func TestPackageDirOfHandlesTheModuleRoot(t *testing.T) {
	if dir, ok := packageDirOf("example.test", "example.test", []string{"."}); !ok || dir != "." {
		t.Errorf("root = (%q, %v), want (\".\", true)", dir, ok)
	}
	if _, ok := packageDirOf("example.test", "example.test", []string{"internal/x"}); ok {
		t.Error("the module root must not match when it is not a touched package")
	}
	if _, ok := packageDirOf("", "example.test", []string{"."}); ok {
		t.Error("an empty import path must not match")
	}
	if _, ok := packageDirOf("example.test/other", "example.test", []string{"internal/x"}); ok {
		t.Error("an untouched package must not match")
	}
}
