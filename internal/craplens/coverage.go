package craplens

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"
)

// coverageTimeout bounds the whole coverage run. Exceeding it leaves every
// package unknown, which downgrades the signal rather than failing the job.
const coverageTimeout = 12 * time.Minute

// profileBlock is one entry from a Go coverage profile: a statement span in a
// file, how many statements it holds, and how many times it ran.
type profileBlock struct {
	startLine int
	endLine   int
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
// blocks with the function's line span.
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
		if b.startLine < f.Line || b.startLine > end {
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

	argv := []string{"test", "-covermode=set", "-coverprofile=" + profilePath, "-count=1"}
	for _, dir := range pkgDirs {
		argv = append(argv, "./"+dir)
	}
	cmd := exec.CommandContext(ctx, "go", argv...)
	cmd.Dir = repoDir
	// A failing test still writes a profile for the packages that passed, so
	// the exit status is deliberately ignored: this signal reports on what
	// could be measured and stays silent about the rest. The test suite's own
	// pass or fail is a different gate's job.
	_ = cmd.Run()

	raw, err := os.ReadFile(profilePath)
	if err != nil {
		return data
	}
	parseProfile(string(raw), data)
	return data
}

// parseProfile reads a Go coverage profile into per-file blocks and per-package
// totals.
//
// Profile lines are `<import-path>/<file>:<sl>.<sc>,<el>.<ec> <numStmt> <count>`.
// The import path is mapped back to a repository-relative directory by matching
// the package directories the diff touched, so this works without resolving the
// module path.
func parseProfile(raw string, data coverageData) {
	type totals struct{ total, covered int }
	perPackage := map[string]*totals{}

	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		loc, counts, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		fullPath, span, ok := strings.Cut(loc, ":")
		if !ok {
			continue
		}
		block, ok := parseSpan(span, counts)
		if !ok {
			continue
		}

		pkgDir, rel, ok := matchPackage(data.packages, fullPath)
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
			continue
		}
		data.packages[pkgDir] = packageCoverage{coverage: float64(t.covered) / float64(t.total)}
	}
}

// matchPackage maps a profile's import-path-qualified file onto one of the
// repository-relative package directories the diff touched, returning that
// directory and the repository-relative file path.
func matchPackage(pkgs map[string]packageCoverage, fullPath string) (string, string, bool) {
	dir := path.Dir(fullPath)
	base := path.Base(fullPath)
	for pkgDir := range pkgs {
		if dir == pkgDir || strings.HasSuffix(dir, "/"+pkgDir) {
			return pkgDir, path.Join(pkgDir, base), true
		}
	}
	return "", "", false
}

// parseSpan reads a profile entry's line span, statement count, and hit count.
func parseSpan(span, counts string) (profileBlock, bool) {
	startPart, endPart, ok := strings.Cut(span, ",")
	if !ok {
		return profileBlock{}, false
	}
	startLine, ok := leadingInt(startPart)
	if !ok {
		return profileBlock{}, false
	}
	endLine, ok := leadingInt(endPart)
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
	return profileBlock{startLine: startLine, endLine: endLine, numStmt: numStmt, count: count}, true
}

// leadingInt reads the line number from a `<line>.<column>` pair.
func leadingInt(s string) (int, bool) {
	lineStr, _, _ := strings.Cut(s, ".")
	n, err := strconv.Atoi(lineStr)
	if err != nil {
		return 0, false
	}
	return n, true
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
