// Command test-affected prints the set of Go test packages affected by a
// pull request, so CI can run only the integration tests that actually
// matter.
//
// It works in two steps:
//
//  1. Compute the set of "changed" packages by diffing Go files between a
//     base ref (default origin/main) and HEAD. Non-Go changes are mostly
//     ignored, except for a small set of "force-full" paths (go.mod,
//     go.sum, the Makefile, this command itself, CI workflows, etc.) which
//     conservatively cause every test package to be printed — the same
//     answer go test ./... would produce.
//  2. Walk the dependency graph from `go list -deps -test -json` and emit
//     every test-bearing package that transitively depends on a changed
//     package. A package is "test-bearing" if it has TestGoFiles or
//     XTestGoFiles. The transitive closure means a change inside
//     internal/foo also lights up agm/test/integration when the latter
//     imports it through any chain.
//
// Build tags matter: integration / e2e / contract tests are only visible
// to `go list` if the matching tag is passed. Use --tags to opt in, and
// the tool will only consider test packages that exist under that tag set.
//
// Output is newline-separated import paths on stdout, suitable for piping:
//
//	pkgs=$(go run ./cmd/test-affected --tags=integration)
//	[ -n "$pkgs" ] && go test -tags=integration -race -count=1 $pkgs
//
// Empty output means "no integration tests are affected by this PR" —
// that is a valid, fast pass, not an error.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

type options struct {
	base    string
	head    string
	tags    string
	root    string
	verbose bool
	// emitAll: if true, output every package, not just test-bearing ones.
	// Useful for sanity checks; the default matches the CI contract.
	emitAll bool
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("test-affected", flag.ContinueOnError)
	fs.SetOutput(stderr)
	opts := options{}
	fs.StringVar(&opts.base, "base", "origin/main", "git base ref to diff against")
	fs.StringVar(&opts.head, "head", "HEAD", "git head ref")
	fs.StringVar(&opts.tags, "tags", "", "comma-separated build tags (passed to go list and emitted to caller)")
	fs.StringVar(&opts.root, "root", "", "repo root (default: git rev-parse --show-toplevel)")
	fs.BoolVar(&opts.verbose, "verbose", false, "log decisions to stderr")
	fs.BoolVar(&opts.emitAll, "all", false, "emit all affected packages, not just test-bearing ones")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if opts.root == "" {
		out, err := gitOutput("", "rev-parse", "--show-toplevel")
		if err != nil {
			fmt.Fprintf(stderr, "test-affected: cannot find repo root: %v\n", err)
			return 2
		}
		opts.root = strings.TrimSpace(out)
	}

	changed, err := changedFiles(opts.root, opts.base, opts.head)
	if err != nil {
		fmt.Fprintf(stderr, "test-affected: git diff failed: %v\n", err)
		return 2
	}
	if opts.verbose {
		fmt.Fprintf(stderr, "test-affected: %d changed files between %s and %s\n", len(changed), opts.base, opts.head)
	}

	pkgs, err := listPackages(opts.root, opts.tags)
	if err != nil {
		fmt.Fprintf(stderr, "test-affected: go list failed: %v\n", err)
		return 2
	}

	decision := decide(opts, changed, pkgs)

	if decision.forceFull {
		if opts.verbose {
			fmt.Fprintf(stderr, "test-affected: forcing full test run (reason: %s)\n", decision.forceReason)
		}
	}

	out := selectPackages(pkgs, decision, opts.emitAll)
	sort.Strings(out)
	w := bufio.NewWriter(stdout)
	for _, p := range out {
		fmt.Fprintln(w, p)
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(stderr, "test-affected: write failed: %v\n", err)
		return 2
	}

	if opts.verbose {
		fmt.Fprintf(stderr, "test-affected: %d test packages affected (out of %d total)\n", len(out), len(pkgs))
	}
	return 0
}

// goListPackage mirrors the subset of `go list -json` we use. Fields
// omitted here are present in the JSON stream but unused.
type goListPackage struct {
	Dir            string
	ImportPath     string
	GoFiles        []string
	TestGoFiles    []string
	XTestGoFiles   []string
	IgnoredGoFiles []string
	Imports        []string
	TestImports    []string
	XTestImports   []string
	Deps           []string
	Standard       bool
	Module         *struct {
		Path string
		Main bool
	}
}

func (p *goListPackage) isTestBearing() bool {
	return len(p.TestGoFiles) > 0 || len(p.XTestGoFiles) > 0
}

// listPackages runs `go list -deps -test -json ./...` rooted at root and
// returns the decoded packages. We pass -test so that go list synthesises
// the _test variants and includes their imports in Deps — without it, a
// package that is only reachable through an _test.go import would look
// dependency-free.
func listPackages(root, tags string) ([]*goListPackage, error) {
	args := []string{"list", "-deps", "-test", "-json"}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	args = append(args, "./...")
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("go list: %w: %s", err, stderr.String())
	}

	dec := json.NewDecoder(&stdout)
	var pkgs []*goListPackage
	seen := map[string]int{}
	for {
		var p goListPackage
		if err := dec.Decode(&p); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode go list output: %w", err)
		}
		// `-test` synthesises ".test" variants with the same Dir; we keep
		// only the first occurrence per ImportPath so callers don't see
		// duplicates. Both forms agree on Deps, which is what matters.
		if _, ok := seen[p.ImportPath]; ok {
			continue
		}
		seen[p.ImportPath] = len(pkgs)
		pp := p
		pkgs = append(pkgs, &pp)
	}
	return pkgs, nil
}

func gitOutput(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if root != "" {
		cmd.Dir = root
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}

// changedFiles returns the list of paths (repo-relative) that differ
// between base and head, including added and deleted files. We use
// three-dot range so the comparison is "what HEAD has that base does not"
// — the same semantics GitHub uses for PR diffs.
func changedFiles(root, base, head string) ([]string, error) {
	out, err := gitOutput(root, "diff", "--name-only", base+"..."+head)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// forceFullPatterns lists path prefixes / exact paths that conservatively
// invalidate any package-level cache and force a full run. The rule is
// "if changing this can plausibly affect any test, run everything." Keep
// this list short and obvious; specific test wiring belongs in the graph,
// not here.
var forceFullPatterns = []string{
	"go.mod",
	"go.sum",
	"go.work",
	"go.work.sum",
	"Makefile",
	".github/workflows/",   // CI wiring changes — re-run everything
	"cmd/test-affected/",   // changing the selector itself
	"scripts/test-affected", // wrapper
}

func isForceFullPath(p string) bool {
	for _, prefix := range forceFullPatterns {
		if p == prefix || strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

type decision struct {
	forceFull    bool
	forceReason  string
	changedPkgs  map[string]struct{} // ImportPath → present
}

func decide(opts options, changed []string, pkgs []*goListPackage) decision {
	d := decision{changedPkgs: map[string]struct{}{}}

	// Build a Dir → ImportPath index. We use absolute paths because go
	// list emits absolute Dirs.
	dirToPkg := map[string]string{}
	for _, p := range pkgs {
		if p.Standard || p.Dir == "" {
			continue
		}
		rel, err := filepath.Rel(opts.root, p.Dir)
		if err != nil {
			continue
		}
		dirToPkg[rel] = p.ImportPath
		// Also accept the absolute form for safety.
		dirToPkg[p.Dir] = p.ImportPath
	}

	for _, f := range changed {
		if isForceFullPath(f) {
			d.forceFull = true
			d.forceReason = f
			// Don't return early — finish populating changedPkgs so
			// verbose mode is useful, but the caller will ignore it.
		}
		dir := filepath.Dir(f)
		// Walk up until we hit a known package dir or repo root. This
		// handles _test.go files in subpackages, testdata/ children,
		// fixtures, and assorted siblings.
		for dir != "." && dir != "/" && dir != "" {
			if pkg, ok := dirToPkg[dir]; ok {
				d.changedPkgs[pkg] = struct{}{}
				break
			}
			dir = filepath.Dir(dir)
		}
	}

	if opts.verbose {
		var sorted []string
		for k := range d.changedPkgs {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		for _, p := range sorted {
			fmt.Fprintf(os.Stderr, "test-affected:   changed-pkg %s\n", p)
		}
	}

	return d
}

// selectPackages returns the import paths of every test-bearing package
// that transitively depends on any changed package. When emitAll is set,
// it returns every affected package (not just test-bearing ones).
//
// "Transitively depends on" is computed against the union of Imports,
// TestImports, XTestImports, and Deps. Deps is the closure as reported by
// the go tool, so a single membership check per dependency is enough.
func selectPackages(pkgs []*goListPackage, d decision, emitAll bool) []string {
	if d.forceFull {
		var out []string
		for _, p := range pkgs {
			if p.Standard {
				continue
			}
			if !emitAll && !p.isTestBearing() {
				continue
			}
			out = append(out, p.ImportPath)
		}
		return out
	}
	if len(d.changedPkgs) == 0 {
		return nil
	}

	var out []string
	for _, p := range pkgs {
		if p.Standard {
			continue
		}
		if !emitAll && !p.isTestBearing() {
			continue
		}
		if _, ok := d.changedPkgs[p.ImportPath]; ok {
			out = append(out, p.ImportPath)
			continue
		}
		// Check the dependency closure plus the test-only edges that
		// `go list -deps -test` may not have folded into Deps for every
		// variant.
		matched := false
		for _, dep := range p.Deps {
			if _, ok := d.changedPkgs[dep]; ok {
				matched = true
				break
			}
		}
		if !matched {
			for _, dep := range append(append([]string{}, p.TestImports...), p.XTestImports...) {
				if _, ok := d.changedPkgs[dep]; ok {
					matched = true
					break
				}
			}
		}
		if matched {
			out = append(out, p.ImportPath)
		}
	}
	return out
}
