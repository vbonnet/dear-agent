package craplens

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// diffTimeout bounds the diff walk so a wedged repository cannot hold a CI job
// open for its whole timeout budget.
const diffTimeout = 60 * time.Second

// lineRange is an inclusive span of head-side line numbers a hunk touched.
type lineRange struct {
	start int
	end   int
}

// touchedFile is one changed Go source file and the head lines the diff wrote.
type touchedFile struct {
	// Path is repository-relative.
	Path string
	// Added reports whether the file is new in this diff, which is how a
	// wholly-new package is distinguished from an edited one.
	Added bool
	// Ranges are the head-side line spans the diff touched. Empty for a pure
	// deletion, which is filtered out before this point.
	Ranges []lineRange
}

// touched is the set of changed Go files, keyed by path.
type touchedSet map[string]*touchedFile

// allFilesAdded reports whether every changed file in the package was added by
// this diff. It is a necessary condition for a new package but not a
// sufficient one: adding a single file to an existing package also satisfies
// it, which is why the caller confirms against the base tree.
func (t touchedSet) allFilesAdded(pkgDir string) bool {
	sawOne := false
	for _, f := range t {
		if path.Dir(f.Path) != pkgDir {
			continue
		}
		sawOne = true
		if !f.Added {
			return false
		}
	}
	return sawOne
}

// changedGoFiles returns the non-test Go files the diff touched, with the
// head-side line ranges it wrote in each.
//
// The range is base...head (three dots), matching pr-size-scope.yml: it diffs
// head against the merge base, so commits landed on the base branch since the
// PR opened are not attributed to this PR.
//
// Test files are excluded on purpose. A test's own complexity is not what this
// signal is about, and including them would let a diff improve its score by
// adding a branchy test.
func changedGoFiles(ctx context.Context, repoDir, base, head string) (touchedSet, error) {
	ctx, cancel := context.WithTimeout(ctx, diffTimeout)
	defer cancel()

	argv := []string{}
	if repoDir != "" {
		argv = append(argv, "-C", repoDir)
	}
	// -U0 so each hunk header names exactly the lines that changed rather than
	// three lines of untouched context on either side, which would attribute a
	// neighbouring function to this diff.
	// -c core.quotePath=false: by default git C-quotes any pathname with
	// non-ASCII bytes, so `docs/café.go` arrives as "docs/caf\303\251.go"
	// and the +++ header parse below silently drops the file. Turning quoting
	// off keeps the pathname verbatim.
	argv = append(argv, "-c", "core.quotePath=false",
		"diff", "-M", "-U0", "--diff-filter=ACMR", base+"..."+head, "--", "*.go")
	cmd := exec.CommandContext(ctx, "git", argv...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff %s...%s failed: %w: %s", base, head, err, strings.TrimSpace(stderr.String()))
	}

	files, err := parseUnifiedDiff(string(out))
	if err != nil {
		return nil, err
	}
	return files, nil
}

// parseUnifiedDiff extracts changed files and their head-side line ranges from
// `git diff -U0` output.
//
// Within one file's section git emits `diff --git`, then any mode lines
// including `new file mode`, then `+++ b/<path>`, then the hunks. The
// added-file marker therefore arrives before the path it describes, so it is
// held in newFile until the path line names its file.
func parseUnifiedDiff(out string) (touchedSet, error) {
	files := touchedSet{}
	var current *touchedFile
	newFile := false

	scanner := bufio.NewScanner(strings.NewReader(out))
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "diff --git "):
			current = nil
			newFile = false
		case strings.HasPrefix(line, "new file mode"):
			newFile = true
		case strings.HasPrefix(line, "+++ "):
			p, ok := headerPath(strings.TrimPrefix(line, "+++ "))
			if !ok {
				current = nil
				continue
			}
			if !isScorableGoFile(p) {
				current = nil
				continue
			}
			if files[p] == nil {
				files[p] = &touchedFile{Path: p}
			}
			current = files[p]
			if newFile {
				current.Added = true
			}
		case strings.HasPrefix(line, "@@"):
			if current == nil {
				continue
			}
			if r, ok := parseHunkHeader(line); ok {
				current.Ranges = append(current.Ranges, r)
			}
		}
	}

	// A scanner that hit an over-long line stops early, and returning what it
	// managed to read would present a truncated diff as a complete one:
	// every file after the offending hunk would silently drop out of the
	// signal. Fail instead, so the step reports rather than under-reports.
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading diff: %w", err)
	}

	// A file whose every hunk was a pure deletion has no head-side lines left
	// to score.
	for p, f := range files {
		if len(f.Ranges) == 0 {
			delete(files, p)
		}
	}
	return files, nil
}

// headerPath extracts the head-side pathname from a `+++` diff header.
//
// core.quotePath=false stops git quoting non-ASCII bytes, but it still
// C-quotes a pathname containing a tab, newline, backslash, or double quote:
// `+++ "b/p/tab\tname.go"`. Requiring a bare `b/` prefix would drop those
// files silently, which is the same class of miss the quotePath flag fixed for
// non-ASCII. Go string-literal syntax covers git's C-quoting, including its
// octal escapes, so strconv.Unquote decodes it.
func headerPath(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "/dev/null" {
		return "", false
	}
	if strings.HasPrefix(raw, `"`) {
		unquoted, err := strconv.Unquote(raw)
		if err != nil {
			return "", false
		}
		raw = unquoted
	}
	path, ok := strings.CutPrefix(raw, "b/")
	if !ok {
		return "", false
	}
	return path, isScorableGoFile(path)
}

// isScorableGoFile reports whether a path is Go source this signal scores.
// Tests, vendored code, and testdata fixtures are excluded by path.
//
// Generated files are excluded too, but a path check alone cannot find them:
// this repository contains generated source such as
// engram/hooks-bin/internal/validator/patterns.go that matches neither
// suffix. Content is checked separately by isGeneratedSource.
func isScorableGoFile(p string) bool {
	if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
		return false
	}
	for part := range strings.SplitSeq(p, "/") {
		switch part {
		case "vendor", "testdata", "node_modules", ".worktrees":
			return false
		}
	}
	return !strings.HasSuffix(p, ".pb.go") && !strings.HasSuffix(p, "_generated.go")
}

// generatedMarker matches the marker line the Go toolchain convention
// prescribes for machine-written source. Only the leading portion of a file is
// searched, because the convention places it before the package clause.
var generatedMarker = regexp.MustCompile(`(?m)^// Code generated .* DO NOT EDIT\.$`)

// isGeneratedSource reports whether file content carries the standard
// generated-code marker. CRAPLENS-02 excludes generated files, and asking an
// author to add tests for a file a generator will overwrite is noise.
func isGeneratedSource(src []byte) bool {
	head := src
	if len(head) > 4096 {
		head = head[:4096]
	}
	return generatedMarker.Match(head)
}

// parseHunkHeader reads the head-side span from a unified-diff hunk header of
// the form `@@ -a,b +c,d @@`. A hunk with a zero head-side count is a pure
// deletion and touches no head line.
func parseHunkHeader(line string) (lineRange, bool) {
	_, rest, found := strings.Cut(line, "+")
	if !found {
		return lineRange{}, false
	}
	if end := strings.IndexAny(rest, " @"); end >= 0 {
		rest = rest[:end]
	}
	startStr, countStr, hasCount := strings.Cut(rest, ",")
	start, err := strconv.Atoi(startStr)
	if err != nil {
		return lineRange{}, false
	}
	count := 1
	if hasCount {
		count, err = strconv.Atoi(countStr)
		if err != nil {
			return lineRange{}, false
		}
	}
	if count == 0 {
		return lineRange{}, false
	}
	return lineRange{start: start, end: start + count - 1}, true
}

// packagesOf returns the distinct package directories the diff touched, as
// repository-relative paths.
func packagesOf(files touchedSet) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range files {
		dir := path.Dir(f.Path)
		if !seen[dir] {
			seen[dir] = true
			out = append(out, dir)
		}
	}
	return out
}

// overlaps reports whether a function's line span intersects any changed range.
func overlaps(ranges []lineRange, start, end int) bool {
	for _, r := range ranges {
		if r.start <= end && start <= r.end {
			return true
		}
	}
	return false
}

// fileAt returns a file's contents at a revision.
func fileAt(ctx context.Context, repoDir, rev, rel string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, diffTimeout)
	defer cancel()

	argv := []string{}
	if repoDir != "" {
		argv = append(argv, "-C", repoDir)
	}
	// `rev:path` is a single object argument, not a pathspec, so there is no
	// `--` to place it after. A revision cannot begin with a dash here
	// because it comes from the caller's -base/-head, and git rejects a
	// malformed spec rather than treating it as an option.
	argv = append(argv, "show", rev+":"+rel)
	cmd := exec.CommandContext(ctx, "git", argv...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git show %s:%s failed: %w: %s", rev, rel, err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// treeHasGoFiles reports whether a directory held any Go source at a revision.
//
// This is what decides whether a zero-coverage package is NEW. Inferring it
// from the diff alone is wrong: a PR that adds one file to an existing package
// touches only added files, so every file in the touched set is an addition
// and the package would be mislabeled as new.
func treeHasGoFiles(ctx context.Context, repoDir, rev, dir string) bool {
	ctx, cancel := context.WithTimeout(ctx, diffTimeout)
	defer cancel()

	argv := []string{}
	if repoDir != "" {
		argv = append(argv, "-C", repoDir)
	}
	// `rev:.` is not a valid object name, so the module root has to be
	// addressed as the bare tree. Without this the lookup always errors and
	// an existing zero-coverage root package would be labeled new.
	spec := rev + ":" + dir
	if dir == "." || dir == "" {
		spec = rev + ":"
	}
	argv = append(argv, "ls-tree", "--name-only", spec)
	out, err := exec.CommandContext(ctx, "git", argv...).Output()
	if err != nil {
		// The directory did not exist at that revision, which is exactly the
		// new-package case.
		return false
	}
	for name := range strings.SplitSeq(string(out), "\n") {
		if strings.HasSuffix(strings.TrimSpace(name), ".go") {
			return true
		}
	}
	return false
}

// workingTreeIsClean reports whether the checkout has no staged, unstaged, or
// untracked changes.
//
// Coverage is measured by running tests against the checkout while function
// spans and complexity come from the committed head tree. With a dirty tree
// those two disagree: an untracked test can make an uncovered function look
// covered, and an uncommitted edit can shift the lines a profile block maps to.
func workingTreeIsClean(ctx context.Context, repoDir string) bool {
	ctx, cancel := context.WithTimeout(ctx, diffTimeout)
	defer cancel()

	argv := []string{}
	if repoDir != "" {
		argv = append(argv, "-C", repoDir)
	}
	// --ignored=matching as well: an ignored _test.go file is invisible to a
	// plain status but `go test` still compiles it, and it could make an
	// uncovered function appear covered.
	argv = append(argv, "status", "--porcelain", "--untracked-files=normal", "--ignored=matching")
	out, err := exec.CommandContext(ctx, "git", argv...).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == ""
}

// headIsCheckedOut reports whether the working tree is at the head revision.
//
// Coverage is measured by running the package tests against the checkout, so
// it is only meaningful when the checkout IS the head. When it is not, every
// package is left unknown rather than scored against the wrong source.
func headIsCheckedOut(ctx context.Context, repoDir, head string) bool {
	resolve := func(rev string) (string, bool) {
		argv := []string{}
		if repoDir != "" {
			argv = append(argv, "-C", repoDir)
		}
		argv = append(argv, "rev-parse", "--verify", rev+"^{commit}")
		out, err := exec.CommandContext(ctx, "git", argv...).Output()
		if err != nil {
			return "", false
		}
		return strings.TrimSpace(string(out)), true
	}
	want, ok := resolve(head)
	if !ok {
		return false
	}
	got, ok := resolve("HEAD")
	return ok && got == want
}
