package craplens

import (
	"bufio"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path"
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
	// --text: a head-tree .gitattributes marking *.go as binary would
	// otherwise make git diff emit only "Binary files ... differ" with no
	// +++ header or hunk at all, so parseUnifiedDiff sees an empty touched
	// set — Analyze then reports a clean, fully-known verdict for a PR it
	// never actually measured, which could delete a standing code-health
	// finding instead of leaving it in place. Go source is always text.
	argv = append(argv, "-c", "core.quotePath=false",
		"diff", "-M", "-U0", "--text", "--diff-filter=ACMRT", base+"..."+head, "--", "*.go")
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
	state := diffScanState{files: files, inHeader: true}

	scanner := bufio.NewScanner(strings.NewReader(out))
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		state.consume(scanner.Text())
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

// diffScanState carries the per-file position of the unified-diff walk.
type diffScanState struct {
	files   touchedSet
	current *touchedFile
	newFile bool
	// inHeader is true between a `diff --git` line and the first hunk. It is
	// what tells a `+++ ` file header apart from an ADDED SOURCE LINE whose
	// content begins with `++ `, which git also emits as a line starting
	// `+++ `. Without it such a line cleared the current file and every later
	// hunk in it was dropped.
	inHeader bool
}

// consume advances the walk by one diff line.
func (st *diffScanState) consume(line string) {
	switch {
	case strings.HasPrefix(line, "diff --git "):
		st.current = nil
		st.newFile = false
		st.inHeader = true
	case st.inHeader && strings.HasPrefix(line, "new file mode"):
		st.newFile = true
	case st.inHeader && strings.HasPrefix(line, "rename to "):
		// Git emits `rename from`/`rename to` instead of `new file mode` for a
		// renamed-and-edited file, so this destination path is new to
		// wherever it landed even though it is not new to the repository.
		// allFilesAdded only asks whether every touched file in a package
		// arrived fresh into it; treeHasGoFiles at the caller is what still
		// catches the case where the destination package already existed.
		st.newFile = true
	case st.inHeader && strings.HasPrefix(line, "+++ "):
		st.startFile(strings.TrimPrefix(line, "+++ "))
	case strings.HasPrefix(line, "@@"):
		st.inHeader = false
		st.addHunk(line)
	}
}

// startFile begins collecting hunks for the file a `+++` header names.
func (st *diffScanState) startFile(raw string) {
	path, ok := headerPath(raw)
	if !ok {
		st.current = nil
		return
	}
	if st.files[path] == nil {
		st.files[path] = &touchedFile{Path: path}
	}
	st.current = st.files[path]
	if st.newFile {
		st.current.Added = true
	}
}

// addHunk records a hunk header's head-side span against the current file.
func (st *diffScanState) addHunk(line string) {
	if st.current == nil {
		return
	}
	if r, ok := parseHunkHeader(line); ok {
		st.current.Ranges = append(st.current.Ranges, r)
	}
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

// isGeneratedSource reports whether file content carries the standard
// generated-code marker. CRAPLENS-02 excludes generated files, and asking an
// author to add tests for a file a generator will overwrite is noise.
//
// This parses just far enough to read the comments preceding the package
// clause and defers to ast.IsGenerated, rather than searching the file's raw
// bytes for the marker line: a handwritten file that happens to contain the
// same text after its package clause — inside a raw-string code-generation
// template, or an explanatory comment about the convention itself, both of
// which exist in this repo (engram/hooks-bin/cmd/generate-patterns/main.go's
// tmplSource) — must not be misclassified as generated just because the
// marker appears somewhere in its first few KiB.
func isGeneratedSource(src []byte) bool {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments|parser.PackageClauseOnly)
	if err != nil {
		// A file this command cannot even parse this far is not one CRAPLENS
		// can score anyway; report it as handwritten so it surfaces through
		// the normal complexity/coverage path rather than being silently
		// excluded here too.
		return false
	}
	return ast.IsGenerated(f)
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
	argv = append(argv, "ls-tree", "-z", "--name-only", spec)
	out, err := exec.CommandContext(ctx, "git", argv...).Output()
	if err != nil {
		// The directory did not exist at that revision, which is exactly the
		// new-package case.
		return false
	}
	for name := range strings.SplitSeq(strings.TrimRight(string(out), "\x00"), "\x00") {
		if strings.HasSuffix(name, ".go") {
			return true
		}
	}
	return false
}

// workingTreeIsClean reports whether the checkout has no staged, unstaged, or
// untracked changes, and no ignored file sitting inside a touched package
// that `go test` could still pull into its build or its test run.
//
// Coverage is measured by running tests against the checkout while function
// spans and complexity come from the committed head tree. With a dirty tree
// those two disagree: an untracked test can make an uncovered function look
// covered, and an uncommitted edit can shift the lines a profile block maps to.
//
// Any ignored file inside a touched package is treated as dirty, not just an
// ignored .go source: a `//go:embed` directive or a fixture read by path can
// pull in an ignored non-Go asset just as surely as an ignored .go file
// compiles in, and there is no cheap way to tell "this ignored file happens
// to be irrelevant" from "this ignored file is embedded" without parsing
// every touched package's embed directives. Refusing to measure is the safe
// default; a false "cannot measure" costs nothing more than an unflagged
// diff, while a false "clean" scores a function against local-filesystem
// content the head tree does not actually contain.
//
// An ignored path outside every touched package — `make preflight`'s
// `build/` directory, for one — cannot affect either tree and is not treated
// as dirty, or the local lens this repository documents would refuse to
// measure anything for a developer who ran that target once and left the
// directory in place.
func workingTreeIsClean(ctx context.Context, repoDir string, pkgs []string) bool {
	ctx, cancel := context.WithTimeout(ctx, diffTimeout)
	defer cancel()

	argv := []string{}
	if repoDir != "" {
		argv = append(argv, "-C", repoDir)
	}
	// --ignored=matching as well: an ignored _test.go file is invisible to a
	// plain status but `go test` still compiles it, and it could make an
	// uncovered function appear covered.
	//
	// -z: text-mode --porcelain C-quotes a path containing a space or
	// non-ASCII byte (e.g. `"p/a name_test.go"`), and the retained closing
	// quote made the .go suffix check below false, silently accepting an
	// ignored test file as clean. -z uses NUL-terminated, never-quoted
	// paths instead. A rename entry's extra NUL-separated old path is safe
	// to leave unhandled: its status is never "!!", so the loop below
	// already returns false on that entry before the stray old-path
	// fragment is ever reached.
	argv = append(argv, "status", "--porcelain=v1", "-z", "--untracked-files=normal", "--ignored=matching")
	out, err := exec.CommandContext(ctx, "git", argv...).Output()
	if err != nil {
		return false
	}

	for entry := range strings.SplitSeq(strings.TrimRight(string(out), "\x00"), "\x00") {
		if entry == "" {
			continue
		}
		if len(entry) < 4 {
			return false // malformed porcelain output; do not risk a false clean
		}
		status, p := entry[:2], entry[3:]
		if status != "!!" {
			return false // a real staged, unstaged, or untracked change
		}
		if !inTouchedPackage(p, pkgs) {
			continue // ignored, and outside every touched package
		}
		return false // an ignored file inside a touched package can still reach its build via //go:embed or a fixture read
	}
	return true
}

// inTouchedPackage reports whether a repo-relative path sits inside one of
// the given package directories, or any of their subdirectories: a
// //go:embed directive or a fixture read by path can reach a nested asset
// just as surely as a direct child.
func inTouchedPackage(p string, pkgs []string) bool {
	for _, pkg := range pkgs {
		// The module root package owns every repository-relative path, not
		// just its direct children: a root package can reach a nested asset
		// with `//go:embed assets/*` or a fixture read exactly as any other
		// package reaches its own subdirectories. Matching only direct
		// children would score the root package against local content the
		// head tree does not contain, which is the false "clean" this
		// function exists to prevent.
		if pkg == "." {
			return true
		}
		if p == pkg || strings.HasPrefix(p, pkg+"/") {
			return true
		}
	}
	return false
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
