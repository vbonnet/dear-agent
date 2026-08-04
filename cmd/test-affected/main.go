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
	"context"
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
	"syscall"
	"time"
)

const (
	gitCommandTimeout        = 30 * time.Second
	goTestTimeout            = 20 * time.Minute
	goCommandTimeout         = 55 * time.Minute
	goListCommandTimeout     = 20 * time.Minute
	goCommandTimeoutExitCode = 124
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
	// run: if true, exec `go test` on the selected packages instead of
	// just printing them. Removes the need for a shell wrapper.
	run bool
}

func run(args []string, stdout, stderr io.Writer) int {
	opts, code := parseFlags(args, stderr)
	if code != 0 {
		return code
	}
	if code := resolveOptions(&opts, stderr); code != 0 {
		return code
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

	if opts.run {
		return runTests(opts, out, stdout, stderr)
	}

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

// parseFlags wires the CLI surface and returns a non-zero code on
// parse failure (other than -h/--help, which is a clean exit).
func parseFlags(args []string, stderr io.Writer) (options, int) {
	fs := flag.NewFlagSet("test-affected", flag.ContinueOnError)
	fs.SetOutput(stderr)
	opts := options{}
	fs.StringVar(&opts.base, "base", "origin/main", "git base ref to diff against")
	fs.StringVar(&opts.head, "head", "HEAD", "git head ref")
	fs.StringVar(&opts.tags, "tags", "", "comma-separated build tags (passed to go list and emitted to caller)")
	fs.StringVar(&opts.root, "root", "", "repo root (default: git rev-parse --show-toplevel)")
	fs.BoolVar(&opts.verbose, "verbose", false, "log decisions to stderr")
	fs.BoolVar(&opts.emitAll, "all", false, "emit all affected packages, not just test-bearing ones")
	fs.BoolVar(&opts.run, "run", false, "exec `go test -race -count=1` on the selected packages instead of printing them")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return opts, 0
		}
		return opts, 2
	}
	return opts, 0
}

// resolveOptions fills in the defaults that need a side-effecting
// lookup (repo root) and validates inputs that other steps assume are
// non-empty. Returns a non-zero exit code if validation fails.
func resolveOptions(opts *options, stderr io.Writer) int {
	if opts.root == "" {
		out, err := gitOutput("", "rev-parse", "--show-toplevel")
		if err != nil {
			fmt.Fprintf(stderr, "test-affected: cannot find repo root: %v\n", err)
			return 2
		}
		opts.root = strings.TrimSpace(out)
	}
	// `go list` emits absolute Dir; if a caller passed --root relatively
	// (e.g. "."), filepath.Rel(opts.root, p.Dir) would fail with a
	// "can't make absolute path relative to relative root" error. Resolve
	// once, here.
	absRoot, err := filepath.Abs(opts.root)
	if err != nil {
		fmt.Fprintf(stderr, "test-affected: cannot resolve --root: %v\n", err)
		return 2
	}
	opts.root = absRoot

	if opts.base == "" || opts.head == "" {
		fmt.Fprintln(stderr, "test-affected: --base and --head must be non-empty refs")
		return 2
	}
	return 0
}

// runTests shells out to `go test -race -count=1 -timeout=<required CI timeout>` on the selected
// packages. Empty selection is a clean pass — there are no affected
// tests, and that is a valid CI outcome, not an error.
func runTests(opts options, pkgs []string, stdout, stderr io.Writer) int {
	if len(pkgs) == 0 {
		fmt.Fprintf(stderr, "test-affected: no packages affected (base=%s tags=%s)\n", opts.base, opts.tags)
		return 0
	}
	fmt.Fprintf(stderr, "test-affected: running %d package(s) (base=%s tags=%s)\n", len(pkgs), opts.base, opts.tags)
	ctx, cancel := context.WithTimeout(context.Background(), goCommandTimeout)
	defer cancel()
	return runGoTestCommand(ctx, goCommandTimeout, opts, pkgs, stdout, stderr)
}

func runGoTestCommand(ctx context.Context, timeout time.Duration, opts options, pkgs []string, stdout, stderr io.Writer) int {
	args := goTestArgs(opts, pkgs)
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = opts.root
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	protectGoCommandProcessTree(cmd)
	if err := cmd.Run(); err != nil {
		if errors.Is(context.Cause(ctx), context.DeadlineExceeded) {
			fmt.Fprintf(stderr, "test-affected: go test timed out after %s\n", timeout)
			return goCommandTimeoutExitCode
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(stderr, "test-affected: go test failed to start: %v\n", err)
		return 2
	}
	return 0
}

func goTestArgs(opts options, pkgs []string) []string {
	args := []string{"test", "-race", "-count=1", "-timeout=" + goTestTimeout.String()}
	if opts.tags != "" {
		args = append(args, "-tags="+opts.tags)
	}
	return append(args, pkgs...)
}

func protectGoCommandProcessTree(cmd *exec.Cmd) {
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
	cmd.WaitDelay = time.Second
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

// isMainModulePkg reports whether p belongs to the main module under
// test (i.e., the repo this command was run in). Standard library and
// third-party deps are excluded so the output is something the caller
// can hand to `go test`.
func isMainModulePkg(p *goListPackage) bool {
	if p.Standard {
		return false
	}
	if p.Module == nil {
		return false
	}
	return p.Module.Main
}

// listPackages runs `go list -deps -test -json ./...` rooted at root and
// returns the decoded packages. We pass -test so that go list synthesises
// the _test variants and includes their imports in Deps — without it, a
// package that is only reachable through an _test.go import would look
// dependency-free.
func listPackages(root, tags string) ([]*goListPackage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), goListCommandTimeout)
	defer cancel()
	return listPackagesWithContext(ctx, root, tags)
}

func listPackagesWithContext(ctx context.Context, root, tags string) ([]*goListPackage, error) {
	args := []string{"list", "-deps", "-test", "-json"}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	args = append(args, "./...")
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	protectGoCommandProcessTree(cmd)
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
		// `-test` synthesises three records per testable package: the
		// real one ("x/y"), an instrumented variant whose ImportPath
		// embeds a space and a [bracketed] form, and a binary record
		// ending in ".test". We only want the real one — its Deps
		// already includes the test-only edges thanks to -test.
		if strings.ContainsAny(p.ImportPath, " [") {
			continue
		}
		if strings.HasSuffix(p.ImportPath, ".test") {
			continue
		}
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
	ctx, cancel := context.WithTimeout(context.Background(), gitCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
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
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
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
	".github/workflows/", // CI wiring changes — re-run everything
	"cmd/test-affected/", // changing the selector itself
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
	forceFull   bool
	forceReason string
	changedPkgs map[string]struct{} // ImportPath → present
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
		// Walk up until we hit a known package dir, including the repo
		// root ("."). A Go file at the repo root would be a valid
		// package and previously got silently dropped because the loop
		// terminated before consulting dirToPkg["."]. Handles _test.go
		// files in subpackages, testdata/ children, fixtures, and
		// assorted siblings.
		for {
			if pkg, ok := dirToPkg[dir]; ok {
				d.changedPkgs[pkg] = struct{}{}
				break
			}
			if dir == "." || dir == "/" || dir == "" {
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
//
// Only packages belonging to the main module are returned — third-party
// deps and stdlib are noise from the caller's perspective.
func selectPackages(pkgs []*goListPackage, d decision, emitAll bool) []string {
	if !d.forceFull && len(d.changedPkgs) == 0 {
		return nil
	}
	var out []string
	for _, p := range pkgs {
		if !shouldEmitPackage(p, emitAll) {
			continue
		}
		if d.forceFull || isAffectedByChanges(p, d.changedPkgs) {
			out = append(out, p.ImportPath)
		}
	}
	return out
}

// shouldEmitPackage encodes the "is this even a candidate?" filter:
// main-module-only, and (unless emitAll) only test-bearing packages.
func shouldEmitPackage(p *goListPackage, emitAll bool) bool {
	if !isMainModulePkg(p) {
		return false
	}
	if !emitAll && !p.isTestBearing() {
		return false
	}
	return true
}

// isAffectedByChanges reports whether p is the changed package itself or
// transitively depends on one. The dependency closure check uses Deps
// (the full transitive set reported by `go list -deps -test`), plus the
// raw TestImports/XTestImports edges in case a corner of the graph isn't
// folded into Deps for the variant we're inspecting.
func isAffectedByChanges(p *goListPackage, changed map[string]struct{}) bool {
	if _, ok := changed[p.ImportPath]; ok {
		return true
	}
	if anyIn(p.Deps, changed) {
		return true
	}
	if anyIn(p.TestImports, changed) {
		return true
	}
	if anyIn(p.XTestImports, changed) {
		return true
	}
	return false
}

func anyIn(needles []string, haystack map[string]struct{}) bool {
	for _, n := range needles {
		if _, ok := haystack[n]; ok {
			return true
		}
	}
	return false
}
