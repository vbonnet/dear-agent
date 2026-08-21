package craplens

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// coverageTimeout bounds the whole coverage run. Exceeding it leaves every
// package unknown, which downgrades the signal rather than failing the job.
// coverageTimeout must stay well inside the enclosing CI job's own budget
// (20 minutes for the size-scope job). If the job is cancelled instead of the
// step timing out, the comment steps never run and the advisory signal is lost
// entirely rather than degraded.
const coverageTimeout = 10 * time.Minute

// processWaitDelay bounds how long Wait keeps this process alive after the
// coverage run's own process group has been signalled, closing the captured
// pipes forcibly once it elapses. See protectGoTestProcessTree.
const processWaitDelay = time.Second

// maxCapturedOutput bounds each captured stream from the coverage run. The
// verdict events this package reads are small; the rest is test chatter.
const maxCapturedOutput = 8 << 20

// boundedBuffer accumulates up to limit bytes and discards the rest, so a
// runaway test cannot exhaust the runner through this capture.
type boundedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if remaining := b.limit - b.buf.Len(); remaining > 0 {
		if len(p) <= remaining {
			b.buf.Write(p)
		} else {
			b.buf.Write(p[:remaining])
		}
	}
	// Report the full length: a short write would make exec treat the
	// truncation as a pipe error and fail the run.
	return len(p), nil
}

func (b *boundedBuffer) String() string { return b.buf.String() }

// profileBlock is one entry from a Go coverage profile: a statement span in a
// file, how many statements it holds, and how many times it ran.
type profileBlock struct {
	startLine int
	startCol  int
	endLine   int
	endCol    int
	numStmt   int
	count     int
}

// packageCoverage is a package's measured statement coverage.
type packageCoverage struct {
	coverage float64
}

// coverageData is the parsed result of one coverage run.
type coverageData struct {
	// packages maps a repository-relative package directory to its coverage.
	packages map[string]packageCoverage
	// blocks maps a repository-relative file path to its profile blocks.
	blocks map[string][]profileBlock
}

// functionCoverage returns the fraction of a function's statements that the
// package's tests exercised, derived by intersecting the coverage profile's
// blocks with the function's declaration span.
//
// A function containing no counted statements is reported as fully covered:
// there is nothing in it that a test could fail to reach, so scoring it as
// untested would be a false alarm.
func (c coverageData) functionCoverage(f Function) float64 {
	blocks := c.blocks[f.File]
	if len(blocks) == 0 {
		return CoverageUnknown
	}

	end := max(f.EndLine, f.Line)

	total, covered := 0, 0
	for _, b := range blocks {
		if !blockStartsWithin(b, f.Line, f.StartCol, end, f.EndCol) {
			continue
		}
		total += b.numStmt
		if b.count > 0 {
			covered += b.numStmt
		}
	}
	if total == 0 {
		return 1
	}
	return float64(covered) / float64(total)
}

// blockStartsWithin reports whether a profile block's start position falls
// within a declaration's span from (startLine, startCol) to (endLine, endCol).
//
// Comparing by line alone would double-count a block on a line two function
// literals share — a shape gofmt preserves for
// `var A, B = func() {...}, func() {...}` — crediting both functions with
// whichever one's test happened to run. Column only matters on the two
// boundary lines: any block strictly between them belongs to this function
// regardless of its column. A zero column (a caller that never populated it)
// disables that check rather than rejecting the block, matching the line-only
// behavior this replaces.
func blockStartsWithin(b profileBlock, startLine, startCol, endLine, endCol int) bool {
	if b.startLine < startLine || b.startLine > endLine {
		return false
	}
	if b.startLine == startLine && startCol > 0 && b.startCol > 0 && b.startCol < startCol {
		return false
	}
	if b.startLine == endLine && endCol > 0 && b.startCol > 0 && b.startCol > endCol {
		return false
	}
	return true
}

// collectCoverage runs the package tests under coverage and parses the result.
//
// Every failure mode downgrades to unknown coverage rather than an error:
// several packages in this module need a live tmux socket, a Dolt server, or a
// container runtime, and a package that cannot be measured must not be
// reported as untested.
func collectCoverage(ctx context.Context, repoDir string, pkgDirs []string) coverageData {
	data := unmeasured(pkgDirs)
	if len(pkgDirs) == 0 {
		return data
	}

	ctx, cancel := context.WithTimeout(ctx, coverageTimeout)
	defer cancel()

	profile, err := os.CreateTemp("", "craplens-*.out")
	if err != nil {
		return data
	}
	profilePath := profile.Name()
	_ = profile.Close()
	defer func() { _ = os.Remove(profilePath) }()

	// -json because the per-package verdict has to be read reliably. The
	// human output cannot distinguish "no test files" from "failed" without
	// pattern-matching prose that changes between toolchain releases, and
	// getting that wrong reports a well-tested package as untested.
	argv := []string{"test", "-json", "-covermode=set", "-coverprofile=" + profilePath, "-count=1"}
	for _, dir := range pkgDirs {
		argv = append(argv, "./"+dir)
	}
	cmd := exec.CommandContext(ctx, "go", argv...)
	cmd.Dir = repoDir
	// Bounded, not unbounded: a noisy package streams every output event
	// through -json, and holding all of it for up to the coverage timeout
	// could exhaust the runner and cancel the whole job, which loses the
	// advisory comment entirely instead of degrading one package.
	stdout := &boundedBuffer{limit: maxCapturedOutput}
	stderr := &boundedBuffer{limit: maxCapturedOutput}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	protectGoTestProcessTree(cmd)

	// A failing test still writes a profile for the packages that passed, so a
	// non-zero exit is expected and not fatal: this signal reports on what
	// could be measured. The suite's own pass or fail is a different gate's
	// job. A failure to START the toolchain is different, and is surfaced
	// rather than swallowed, because it means every package is unmeasured for
	// a reason the reader should know about.
	var exitErr *exec.ExitError
	if err := cmd.Run(); err != nil && !errors.As(err, &exitErr) {
		fmt.Fprintf(os.Stderr, "craplens: coverage run could not start: %v\n%s\n", err, stderr.String())
		return data
	}

	modPath := modulePath(ctx, repoDir)

	raw, err := os.ReadFile(profilePath)
	if err != nil {
		return data
	}
	parseProfile(string(raw), modPath, data)

	// A package whose tests failed can still leave the blocks reached before
	// the failure, which would understate its coverage and could report a
	// well-tested package as untested. Drop those back to unknown.
	for _, dir := range failedPackages(stdout.String(), modPath, pkgDirs) {
		data.packages[dir] = packageCoverage{coverage: CoverageUnknown}
	}

	return data
}

// protectGoTestProcessTree bounds how long a cancelled coverage run can keep
// this process alive.
//
// cmd.Stdout and cmd.Stderr above are not *os.File, so Wait copies from them
// in background goroutines that only stop once the write ends close. A test
// that leaves a descendant process holding those inherited pipes survives the
// context's own cancellation of the direct `go` process, so without a bound
// here Wait would keep blocking until the enclosing CI job's own timeout
// killed everything, losing the advisory comment instead of degrading one
// package. Setpgid plus killing the negated PID reaches the whole process
// group rather than only the immediate child; WaitDelay is the backstop that
// still applies if a descendant escaped the group entirely. Matches
// cmd/test-affected's protectGoCommandProcessTree, which runs `go test` under
// the same constraint.
func protectGoTestProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil || cmd.Process.Pid <= 0 {
			return nil
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}
	cmd.WaitDelay = processWaitDelay
}

// testEvent is the subset of `go test -json` output this package reads.
type testEvent struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	// Test is empty on a package-level event and set on a per-test one.
	Test string `json:"Test"`
}

// failedPackages returns the touched package directories whose tests did not
// complete successfully.
//
// A package that reports "fail" is unmeasured: a failing run can still leave
// the profile blocks it reached before failing, and scoring those would
// understate coverage and could report a well-tested package as untested.
//
// A package with no test files reports "skip", not "fail", and IS measured:
// zero coverage is the true answer for it, and surfacing that is the whole
// point of this signal. A package that reported no terminal action at all
// (a build failure, a panic that produced no event) is also unmeasured.
func failedPackages(jsonOut, modPath string, pkgDirs []string) []string {
	measured := map[string]bool{}
	failedSeen := map[string]bool{}

	for line := range strings.SplitSeq(jsonOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var ev testEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		dir, ok := packageDirOf(ev.Package, modPath, pkgDirs)
		if !ok {
			continue
		}
		// Only a PACKAGE-level event decides this. go test -json emits a
		// pass event per test as well as per package, and they are told apart
		// by Test being empty. Accepting a per-test pass would mark a package
		// measured from its first passing test, so a package that later failed
		// after more output than the capture holds would be scored on the
		// partial profile its failure left behind.
		if ev.Test != "" {
			continue
		}
		switch ev.Action {
		case "pass", "skip":
			measured[dir] = true
		case "fail":
			failedSeen[dir] = true
		}
	}

	var failed []string
	for _, dir := range pkgDirs {
		if !measured[dir] || failedSeen[dir] {
			failed = append(failed, dir)
		}
	}
	return failed
}

// packageDirOf maps an import path from a test event back to one of the
// repository-relative package directories under measurement.
func packageDirOf(importPath, modPath string, pkgDirs []string) (string, bool) {
	if importPath == "" {
		return "", false
	}
	if modPath != "" && strings.HasPrefix(importPath, modPath+"/") {
		rel := strings.TrimPrefix(importPath, modPath+"/")
		for _, dir := range pkgDirs {
			if dir == rel {
				return dir, true
			}
		}
		return "", false
	}
	if modPath != "" && importPath == modPath {
		for _, dir := range pkgDirs {
			if dir == "." {
				return dir, true
			}
		}
		return "", false
	}

	best := ""
	for _, dir := range pkgDirs {
		if dir == "." {
			continue
		}
		if importPath == dir || strings.HasSuffix(importPath, "/"+dir) {
			if len(dir) > len(best) {
				best = dir
			}
		}
	}
	return best, best != ""
}

// parseProfile reads a Go coverage profile into per-file blocks and per-package
// totals.
//
// Profile lines are `<import-path>/<file>:<sl>.<sc>,<el>.<ec> <numStmt> <count>`.
// The import path is mapped back to a repository-relative directory by matching
// the package directories the diff touched, so this works without resolving the
// module path.
func parseProfile(raw, modulePath string, data coverageData) {
	type totals struct{ total, covered int }
	perPackage := map[string]*totals{}

	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		// Split from the END: the last two fields are the statement count and
		// the hit count, and everything before them is the location. Cutting
		// at the FIRST space discards any record whose filename contains one,
		// degrading the whole package to unknown.
		loc, counts, ok := cutTrailingCounts(line)
		if !ok {
			continue
		}
		// Cut at the LAST colon, not the first: a filename may legally
		// contain one, and cutting at the first would truncate the path and
		// make parseSpan reject the remainder, silently leaving the function
		// unmeasured.
		idx := strings.LastIndex(loc, ":")
		if idx < 0 {
			continue
		}
		fullPath, span := loc[:idx], loc[idx+1:]
		block, ok := parseSpan(span, counts)
		if !ok {
			continue
		}

		pkgDir, rel, ok := matchPackage(data.packages, modulePath, fullPath)
		if !ok {
			continue
		}
		data.blocks[rel] = append(data.blocks[rel], block)

		t := perPackage[pkgDir]
		if t == nil {
			t = &totals{}
			perPackage[pkgDir] = t
		}
		t.total += block.numStmt
		if block.count > 0 {
			t.covered += block.numStmt
		}
	}

	for pkgDir, t := range perPackage {
		if t.total == 0 {
			// The package was profiled and simply holds no counted
			// statements. That is measured, not unmeasured: leaving it
			// unknown would also make functionCoverage's empty-function
			// handling unreachable for it.
			data.packages[pkgDir] = packageCoverage{coverage: 1}
			continue
		}
		data.packages[pkgDir] = packageCoverage{coverage: float64(t.covered) / float64(t.total)}
	}
}

// cutTrailingCounts splits a coverage-profile line into its location prefix
// and its trailing "<numStmt> <count>" fields.
func cutTrailingCounts(line string) (string, string, bool) {
	second := strings.LastIndex(line, " ")
	if second <= 0 {
		return "", "", false
	}
	first := strings.LastIndex(line[:second], " ")
	if first <= 0 {
		return "", "", false
	}
	return line[:first], line[first+1:], true
}

// matchPackage maps a profile's import-path-qualified file onto one of the
// repository-relative package directories the diff touched, returning that
// directory and the repository-relative file path.
//
// modulePath, when known, makes this exact: a profile path is always
// <modulePath>/<pkgDir>/<file>, so stripping the prefix yields the
// repository-relative path directly and the module root resolves to ".".
//
// Without it the fallback is a longest-suffix match. Longest matters and a
// first match would be wrong: this repository has both internal/tokens and
// engram/internal/tokens, so a profile path for the longer one is a suffix
// match for both, and iterating a map would pick between them at random.
func matchPackage(pkgs map[string]packageCoverage, modulePath, fullPath string) (string, string, bool) {
	// A known module path is authoritative, not a first attempt. Every entry
	// in a profile for this module is prefixed with it, so an entry that is
	// not must not be attributed to one of our packages by suffix; doing that
	// would fold a dependency's blocks into our numbers.
	if modulePath != "" {
		rel, ok := strings.CutPrefix(fullPath, modulePath+"/")
		if !ok {
			return "", "", false
		}
		dir := path.Dir(rel)
		if _, ok := pkgs[dir]; ok {
			return dir, rel, true
		}
		return "", "", false
	}

	dir := path.Dir(fullPath)
	base := path.Base(fullPath)
	best := ""
	for pkgDir := range pkgs {
		if pkgDir == "." {
			continue
		}
		if dir == pkgDir || strings.HasSuffix(dir, "/"+pkgDir) {
			if len(pkgDir) > len(best) {
				best = pkgDir
			}
		}
	}
	if best == "" {
		return "", "", false
	}
	return best, path.Join(best, base), true
}

// modulePath returns the module's import path, or the empty string when it
// cannot be determined. It is used to resolve profile paths exactly rather
// than by suffix.
func modulePath(ctx context.Context, repoDir string) string {
	cmd := exec.CommandContext(ctx, "go", "list", "-m")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// parseSpan reads a profile entry's position span, statement count, and hit
// count. The column half of each `<line>.<column>` pair is kept, not just the
// line, so functionCoverage can tell apart two functions that share a line.
func parseSpan(span, counts string) (profileBlock, bool) {
	startPart, endPart, ok := strings.Cut(span, ",")
	if !ok {
		return profileBlock{}, false
	}
	startLine, startCol, ok := parsePos(startPart)
	if !ok {
		return profileBlock{}, false
	}
	endLine, endCol, ok := parsePos(endPart)
	if !ok {
		return profileBlock{}, false
	}

	fields := strings.Fields(counts)
	if len(fields) != 2 {
		return profileBlock{}, false
	}
	numStmt, err := strconv.Atoi(fields[0])
	if err != nil {
		return profileBlock{}, false
	}
	count, err := strconv.Atoi(fields[1])
	if err != nil {
		return profileBlock{}, false
	}
	return profileBlock{
		startLine: startLine, startCol: startCol,
		endLine: endLine, endCol: endCol,
		numStmt: numStmt, count: count,
	}, true
}

// parsePos reads a `<line>.<column>` pair. A missing column defaults to 0
// rather than rejecting the entry, matching this parser's existing tolerance
// for profile lines slightly outside the documented format.
func parsePos(s string) (line, col int, ok bool) {
	lineStr, colStr, hasCol := strings.Cut(s, ".")
	line, err := strconv.Atoi(lineStr)
	if err != nil {
		return 0, 0, false
	}
	if hasCol {
		col, _ = strconv.Atoi(colStr)
	}
	return line, col, true
}

// unmeasured returns a coverageData in which every package is unknown. It is
// the starting point for a run, and the whole result when coverage cannot be
// collected at all.
func unmeasured(pkgDirs []string) coverageData {
	data := coverageData{
		packages: map[string]packageCoverage{},
		blocks:   map[string][]profileBlock{},
	}
	for _, dir := range pkgDirs {
		data.packages[dir] = packageCoverage{coverage: CoverageUnknown}
	}
	return data
}
