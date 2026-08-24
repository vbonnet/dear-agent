package craplens

import (
	"math"
	"slices"
	"strings"
	"testing"
)

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

// TestFunctionCoverageTreatsEmptyFunctionInMeasuredPackageAsCovered covers a
// changed, buildable file that contains an empty function and no other
// countable statements at all: its successful coverage profile has no block
// for that file, which the len(blocks)==0 early return alone cannot tell
// apart from a build-tagged file absent on this platform. CRAPLENS-06
// requires a statement-free function to be fully covered — check the
// package's own coverage state (collectCoverage's zero-statement handling)
// to make that distinction instead of calling every unprofiled file unknown.
func TestFunctionCoverageTreatsEmptyFunctionInMeasuredPackageAsCovered(t *testing.T) {
	data := coverageData{
		packages:      map[string]packageCoverage{"pkg/x": {coverage: 1}},
		blocks:        map[string][]profileBlock{},
		compiledFiles: map[string]bool{"pkg/x/empty.go": true},
	}
	got := data.functionCoverage(Function{File: "pkg/x/empty.go", Line: 1, EndLine: 3})
	if got != 1 {
		t.Errorf("coverage of an empty function in a measured package = %v, want 1 (fully covered)", got)
	}
}

func TestFunctionCoverageUnknownForExcludedSibling(t *testing.T) {
	data := coverageData{
		packages:      map[string]packageCoverage{"pkg/x": {coverage: 1}},
		blocks:        map[string][]profileBlock{"pkg/x/used.go": {{numStmt: 1, count: 1}}},
		compiledFiles: map[string]bool{"pkg/x/used.go": true},
	}
	if got := data.functionCoverage(Function{File: "pkg/x/excluded.go", Line: 1, EndLine: 3}); got != CoverageUnknown {
		t.Fatalf("excluded sibling coverage = %v, want CoverageUnknown", got)
	}
}

// TestFunctionCoverageStaysUnknownWhenPackageItselfIsUnmeasured is that
// test's counterpart: the same zero-blocks-for-this-file signal must still
// mean "unmeasured", not "covered", when the package itself was never
// confirmed measured (a build-tagged package absent on this platform, or one
// this run simply never populated).
func TestFunctionCoverageStaysUnknownWhenPackageItselfIsUnmeasured(t *testing.T) {
	data := coverageData{
		packages: map[string]packageCoverage{"pkg/x": {coverage: CoverageUnknown}},
		blocks:   map[string][]profileBlock{},
	}
	got := data.functionCoverage(Function{File: "pkg/x/empty.go", Line: 1, EndLine: 3})
	if got != CoverageUnknown {
		t.Errorf("coverage in an unmeasured package = %v, want CoverageUnknown", got)
	}
}

// TestFunctionCoverageSeparatesFunctionsSharingALine covers two package-level
// function literals gofmt can leave on one line, such as
// `var A, B = func() int { return 1 }, func() int { return 2 }`. A line-only
// intersection would credit every block on that line to both functions; only
// A's block is hit here, so a line-only test would report both at 50% instead
// of A fully covered and B not covered at all — either inventing or hiding an
// over-threshold score depending on which function CRAP is scoring.
func TestFunctionCoverageSeparatesFunctionsSharingALine(t *testing.T) {
	data := coverageData{
		blocks: map[string][]profileBlock{
			"pkg/x/y.go": {
				{startLine: 5, startCol: 12, endLine: 5, endCol: 18, numStmt: 1, count: 1},
				{startLine: 5, startCol: 27, endLine: 5, endCol: 33, numStmt: 1, count: 0},
			},
		},
	}

	a := Function{File: "pkg/x/y.go", Line: 5, StartCol: 10, EndLine: 5, EndCol: 20}
	if got := data.functionCoverage(a); got != 1 {
		t.Errorf("coverage of the hit function = %v, want 1", got)
	}

	b := Function{File: "pkg/x/y.go", Line: 5, StartCol: 25, EndLine: 5, EndCol: 35}
	if got := data.functionCoverage(b); got != 0 {
		t.Errorf("coverage of the unhit function = %v, want 0", got)
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

// TestFailedPackagesIgnoresPerTestEvents covers the distinction between a
// per-test pass and a package-level verdict. Accepting the former marked a
// package measured from its first passing test, so a package that later failed
// would be scored on the partial profile its failure left behind.
func TestFailedPackagesIgnoresPerTestEvents(t *testing.T) {
	events := strings.Join([]string{
		`{"Action":"pass","Package":"example.test/p","Test":"TestOne"}`,
		`{"Action":"fail","Package":"example.test/p"}`,
	}, "\n")

	got := failedPackages(events, "example.test", []string{"p"})
	if len(got) != 1 || got[0] != "p" {
		t.Errorf("unmeasured = %v, want [p]: a per-test pass must not mark the package measured", got)
	}

	// And a package-level pass alone still counts as measured.
	onlyPass := `{"Action":"pass","Package":"example.test/p"}`
	if got := failedPackages(onlyPass, "example.test", []string{"p"}); len(got) != 0 {
		t.Errorf("unmeasured = %v, want none for a package-level pass", got)
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

// TestParseSpanHandlesColonInFilename covers a filename containing a colon:
// cutting the profile location at the first colon truncated the path and made
// the span unparseable, silently leaving the function unmeasured.
func TestParseSpanHandlesColonInFilename(t *testing.T) {
	raw := "mode: set\nexample.test/p/a:b.go:2.14,2.26 1 1\n"
	data := unmeasured([]string{"p"})

	parseProfile(raw, "example.test", data)

	blocks := data.blocks["p/a:b.go"]
	if len(blocks) != 1 {
		t.Fatalf("parsed %d blocks for a colon-containing filename, want 1", len(blocks))
	}
	if blocks[0].startLine != 2 || blocks[0].numStmt != 1 || blocks[0].count != 1 {
		t.Errorf("block = %+v", blocks[0])
	}
}

// TestBoundedBufferStopsAtItsLimit covers the capture cap. Reporting a short
// write would make exec treat the truncation as a pipe error and fail the
// whole coverage run, so the writer must always claim the full length.
func TestBoundedBufferStopsAtItsLimit(t *testing.T) {
	b := &boundedBuffer{limit: 10}

	n, err := b.Write([]byte("12345"))
	if n != 5 || err != nil {
		t.Fatalf("first write = (%d, %v)", n, err)
	}
	n, err = b.Write([]byte("67890EXTRA"))
	if n != 10 || err != nil {
		t.Fatalf("overflowing write must report the full length, got (%d, %v)", n, err)
	}
	if got := b.String(); got != "1234567890" {
		t.Errorf("buffered %q, want %q", got, "1234567890")
	}

	n, err = b.Write([]byte("more"))
	if n != 4 || err != nil {
		t.Fatalf("write past the limit = (%d, %v)", n, err)
	}
	if got := b.String(); got != "1234567890" {
		t.Errorf("buffer grew past its limit: %q", got)
	}
}

// TestCutTrailingCounts covers profile records whose filename contains a
// space. Splitting at the first space discarded the record and degraded the
// whole package to unknown.
func TestCutTrailingCounts(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantLoc string
		wantCnt string
		wantOK  bool
	}{
		{
			name:    "ordinary",
			line:    "example.test/p/a.go:2.10,2.26 1 0",
			wantLoc: "example.test/p/a.go:2.10,2.26",
			wantCnt: "1 0",
			wantOK:  true,
		},
		{
			name:    "filename with a space",
			line:    "example.test/p/a b.go:2.10,2.26 1 0",
			wantLoc: "example.test/p/a b.go:2.10,2.26",
			wantCnt: "1 0",
			wantOK:  true,
		},
		{name: "too few fields", line: "nospaces", wantOK: false},
		{name: "one field only", line: "loc 1", wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			loc, counts, ok := cutTrailingCounts(tc.line)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && (loc != tc.wantLoc || counts != tc.wantCnt) {
				t.Errorf("got (%q, %q), want (%q, %q)", loc, counts, tc.wantLoc, tc.wantCnt)
			}
		})
	}
}

// TestWithGoWorkspaceDisabledForcesModuleMode pins that GOWORK=off always
// wins for the go subprocesses this package spawns, whether or not the
// caller's own environment already set (or omitted) GOWORK — a developer's
// ignored go.work file must not be able to smuggle workspace mode into a
// local crap-lint run.
func TestWithGoWorkspaceDisabledForcesModuleMode(t *testing.T) {
	tests := []struct {
		name string
		env  []string
	}{
		{name: "no GOWORK set", env: []string{"PATH=/bin", "HOME=/home/x"}},
		{name: "GOWORK already on", env: []string{"PATH=/bin", "GOWORK=on"}},
		{name: "GOWORK pointed at a workspace file", env: []string{"GOWORK=/home/x/go.work", "PATH=/bin"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := withGoWorkspaceDisabled(tc.env)

			var goworkValues []string
			for _, kv := range got {
				if strings.HasPrefix(kv, "GOWORK=") {
					goworkValues = append(goworkValues, kv)
				}
			}
			if len(goworkValues) != 1 || goworkValues[0] != "GOWORK=off" {
				t.Errorf("GOWORK entries in result = %v, want exactly one \"GOWORK=off\"", goworkValues)
			}

			for _, kv := range tc.env {
				if strings.HasPrefix(kv, "GOWORK=") {
					continue
				}
				if !slices.Contains(got, kv) {
					t.Errorf("non-GOWORK entry %q from the input env was dropped", kv)
				}
			}
		})
	}
}
