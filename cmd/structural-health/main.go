// Command structural-health runs a set of structural-health scans over the
// repository and fails CI only on *regressions* relative to a checked-in
// baseline. This is the DEAR retro "Tier 0" recommendation: make the slow
// drift of the codebase visible and ratchet it down over time, without
// blocking work on the large pile of pre-existing findings.
//
// Six scans run today:
//
//	dead-package       Go packages with zero importers (excludes cmd/ and
//	                   package main — those are entry points, not dead).
//	file-size          .go files longer than the line threshold (1000).
//	zero-test          Go packages that ship no _test.go files.
//	doc-path           ARCHITECTURE.md backtick path references that no
//	                   longer resolve to a real file or directory.
//	goroutine-recover  `go func() { ... }()` launches whose body never
//	                   calls recover() — an unguarded goroutine panic
//	                   takes down the whole process.
//	raw-mem-gate       Shell scripts that gate/alert on a raw macOS
//	                   free-page metric (`vm_stat` free pages,
//	                   page_free_count) without deferring to
//	                   `memory_pressure`. macOS parks reclaimable pages off
//	                   the free list, so raw "free" reads near-zero forever —
//	                   a spawn/dispatch gate built on it deadlocks the thing
//	                   it protects. See docs/policies/harness-hygiene and the
//	                   ce-xj1b over-fit class (re-embedded three times).
//
// Each scan emits a set of stable "finding keys". The baseline file records
// accepted keys and append-only provenance for changes to that set. On every
// run the tool diffs current findings against the baseline:
//
//   - A key present now but absent from the baseline is a REGRESSION and
//     fails the build (exit 1).
//   - A key in the baseline but no longer present is FIXED — informational,
//     and a nudge to re-baseline so the ratchet tightens.
//
// Usage:
//
//	structural-health [flags]
//	structural-health --update-baseline   # remove findings that are now fixed
//	structural-health --json              # machine-readable report
//
// Exit codes:
//   - 0: no regressions (or --update-baseline succeeded).
//   - 1: at least one regression versus the baseline.
//   - 2: usage / setup error (bad flags, go list failed, etc.).
//
// The intent is "loud on regression, quiet on the happy path": a clean run
// against the baseline prints a one-line summary per scan and exits 0.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// fileSizeThreshold is the line count above which a .go file is flagged.
// 1000 lines is the DEAR retro recommendation: large enough that genuinely
// cohesive files pass, small enough that god-files surface.
const fileSizeThreshold = 1000

// defaultBaselinePath is where the ratcheting snapshot lives. It sits at the
// repo root so the scan and the baseline travel together.
const defaultBaselinePath = ".structural-health-baseline.json"

// architectureDoc is the documentation file whose path cross-references are
// validated by the doc-path scan.
const architectureDoc = "ARCHITECTURE.md"

// scanNames is the canonical, ordered list of scans. Output and baseline
// iteration follow this order so diffs stay stable.
var scanNames = []string{
	"dead-package",
	"file-size",
	"zero-test",
	"doc-path",
	"goroutine-recover",
	"raw-mem-gate",
}

// skipDirs are directory names pruned from filesystem walks. These hold
// vendored, generated, or VCS content that the scans should not police.
var skipDirs = map[string]bool{
	".git":         true,
	".worktrees":   true,
	"vendor":       true,
	"testdata":     true,
	"node_modules": true,
	"build":        true,
	"bin":          true,
	"dist":         true,
}

// finding is a single scan result. Key is the stable identity used for
// baseline matching; Detail is human-facing context that may change without
// affecting the diff (e.g. a line count).
type finding struct {
	Key    string
	Detail string
}

func main() {
	var (
		root           = flag.String("root", ".", "repository root to scan")
		baselinePath   = flag.String("baseline", defaultBaselinePath, "path to the baseline JSON file (relative to --root unless absolute)")
		updateBaseline = flag.Bool("update-baseline", false, "update the baseline without admitting new finding keys, then exit 0")
		acceptNew      = flag.Bool("accept-new", false, "allow --update-baseline to admit new finding keys")
		acceptScanner  = flag.Bool("accept-scanner-change", false, "allow --update-baseline to migrate stable-key semantics")
		reason         = flag.String("reason", "", "reason for accepting findings or scanner changes")
		reference      = flag.String("reference", "", "durable tracker, PR, URL, or commit reference for an acceptance")
		jsonOut        = flag.Bool("json", false, "emit a machine-readable JSON report")
	)
	flag.Parse()
	request := updateRequest{
		AcceptNew:           *acceptNew,
		AcceptScannerChange: *acceptScanner,
		Reason:              *reason,
		Reference:           *reference,
	}
	if err := validateModeFlags(*updateBaseline, *jsonOut, request); err != nil {
		fail("%v", err)
	}

	absRoot, err := filepath.Abs(*root)
	if err != nil {
		fail("resolve root: %v", err)
	}

	current, err := runScans(absRoot)
	if err != nil {
		fail("%v", err)
	}

	blPath := *baselinePath
	if !filepath.IsAbs(blPath) {
		blPath = filepath.Join(absRoot, blPath)
	}

	if *updateBaseline {
		plan, err := updateBaselineFile(blPath, current, request)
		if err != nil {
			fail("update baseline: %v", err)
		}
		if !plan.Write {
			fmt.Printf("baseline unchanged: %s\n", blPath)
			return
		}
		fmt.Printf("baseline updated: %s\n", blPath)
		total := keyMapCount(plan.Baseline.Findings)
		fmt.Printf("recorded %d findings across %d scans\n", total, len(scanNames))
		fmt.Printf("transition added=%d removed=%d\n", plan.Change.addedCount(), plan.Change.removedCount())
		return
	}

	bl, err := readBaseline(blPath)
	if err != nil {
		fail("read baseline %s: %v\n\nIf this is the first run, generate it with:\n  structural-health --update-baseline", blPath, err)
	}

	report := diff(current, bl)
	if *jsonOut {
		if err := emitJSON(report); err != nil {
			fail("emit json: %v", err)
		}
	} else {
		emitText(report)
	}

	if report.regressionCount() > 0 {
		os.Exit(1)
	}
}

func validateModeFlags(updateBaseline, jsonOut bool, request updateRequest) error {
	if updateBaseline && jsonOut {
		return errors.New("--json cannot be combined with --update-baseline")
	}
	if !updateBaseline && (request.AcceptNew || request.AcceptScannerChange ||
		strings.TrimSpace(request.Reason) != "" || strings.TrimSpace(request.Reference) != "") {
		return errors.New("baseline acceptance flags require --update-baseline")
	}
	return nil
}

// fail prints a message to stderr and exits with the usage/setup code.
func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "structural-health: "+format+"\n", args...)
	os.Exit(2)
}

// runScans executes every scan and returns findings keyed by scan name.
func runScans(root string) (map[string][]finding, error) {
	pkgs, err := listPackages(root)
	if err != nil {
		return nil, fmt.Errorf("list packages: %w", err)
	}

	out := map[string][]finding{}
	out["dead-package"] = scanDeadPackages(pkgs)
	out["zero-test"] = scanZeroTest(pkgs)

	fileSize, err := scanFileSize(root)
	if err != nil {
		return nil, fmt.Errorf("file-size scan: %w", err)
	}
	out["file-size"] = fileSize

	out["doc-path"] = scanDocPaths(root)

	gor, err := scanGoroutineRecover(root)
	if err != nil {
		return nil, fmt.Errorf("goroutine-recover scan: %w", err)
	}
	out["goroutine-recover"] = gor

	rawMem, err := scanRawMemGate(root)
	if err != nil {
		return nil, fmt.Errorf("raw-mem-gate scan: %w", err)
	}
	out["raw-mem-gate"] = rawMem

	for _, fs := range out {
		sort.Slice(fs, func(i, j int) bool { return fs[i].Key < fs[j].Key })
	}
	return out, nil
}

// goPackage mirrors the subset of `go list -json` output the scans need.
type goPackage struct {
	ImportPath   string
	Name         string
	Dir          string
	GoFiles      []string
	TestGoFiles  []string
	XTestGoFiles []string
	Imports      []string
	TestImports  []string
	XTestImports []string
	Standard     bool
}

// listPackages runs `go list -e -json ./...` from root and decodes the
// stream of package objects. The -e flag keeps partial results flowing even
// when some package has a load error, so a single broken package can't blind
// the whole scan.
func listPackages(root string) ([]goPackage, error) {
	cmd := exec.Command("go", "list", "-e", "-json", "./...")
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}

	var pkgs []goPackage
	dec := json.NewDecoder(&stdout)
	for dec.More() {
		var p goPackage
		if err := dec.Decode(&p); err != nil {
			return nil, fmt.Errorf("decode go list output: %w", err)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

// scanDeadPackages flags module packages that nothing imports. cmd/ trees and
// package main are entry points reached from outside the import graph, so
// they are never considered dead. A package imported only by tests still
// counts as live (TestImports/XTestImports feed the imported set).
func scanDeadPackages(pkgs []goPackage) []finding {
	imported := map[string]bool{}
	for _, p := range pkgs {
		for _, imp := range p.Imports {
			imported[imp] = true
		}
		for _, imp := range p.TestImports {
			imported[imp] = true
		}
		for _, imp := range p.XTestImports {
			imported[imp] = true
		}
	}

	var out []finding
	for _, p := range pkgs {
		if p.Standard || p.Name == "main" {
			continue
		}
		if isUnderCmd(p.ImportPath) {
			continue
		}
		if len(p.GoFiles) == 0 {
			continue // nothing but tests / no buildable source
		}
		if imported[p.ImportPath] {
			continue
		}
		out = append(out, finding{Key: p.ImportPath, Detail: "no importers"})
	}
	return out
}

// scanZeroTest flags module packages that have buildable Go source but ship
// no test files at all (neither in-package nor external _test.go).
func scanZeroTest(pkgs []goPackage) []finding {
	var out []finding
	for _, p := range pkgs {
		if p.Standard || len(p.GoFiles) == 0 {
			continue
		}
		if len(p.TestGoFiles) > 0 || len(p.XTestGoFiles) > 0 {
			continue
		}
		out = append(out, finding{Key: p.ImportPath, Detail: "no _test.go files"})
	}
	return out
}

// isUnderCmd reports whether an import path lives in a cmd/ subtree.
func isUnderCmd(importPath string) bool {
	return strings.Contains(importPath, "/cmd/") || strings.HasSuffix(importPath, "/cmd")
}

// scanFileSize walks the tree and flags .go files longer than the threshold.
// Keys are repo-relative paths so the baseline is location-independent.
func scanFileSize(root string) ([]finding, error) {
	var out []finding
	err := walkGoFiles(root, func(path, rel string) error {
		n, err := countLines(path)
		if err != nil {
			return err
		}
		if n > fileSizeThreshold {
			out = append(out, finding{Key: rel, Detail: fmt.Sprintf("%d lines", n)})
		}
		return nil
	})
	return out, err
}

// countLines returns the number of newline-delimited lines in a file.
func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		n++
	}
	return n, sc.Err()
}

// scanDocPaths checks that path-like backtick references in ARCHITECTURE.md
// resolve to a real file or directory. A backtick token is treated as a path
// when it contains a slash — that excludes prose tokens like `Agent` or
// `--fields` while catching `internal/ops/` and `agm/cmd/agm/`.
func scanDocPaths(root string) []finding {
	docPath := filepath.Join(root, architectureDoc)
	data, err := os.ReadFile(docPath)
	if err != nil {
		// Missing ARCHITECTURE.md is itself a structural-health finding.
		return []finding{{Key: architectureDoc + " (missing)", Detail: err.Error()}}
	}

	var out []finding
	seen := map[string]bool{}
	for _, ref := range backtickPaths(string(data)) {
		if seen[ref] {
			continue
		}
		seen[ref] = true
		clean := strings.TrimSuffix(ref, "/")
		if _, err := os.Stat(filepath.Join(root, clean)); err != nil {
			out = append(out, finding{Key: ref, Detail: "no such file or directory"})
		}
	}
	return out
}

// backtickPaths extracts the contents of inline-code spans that look like
// repository paths (they contain a slash). Code fences (```) are skipped so
// embedded diagrams don't leak tokens.
func backtickPaths(md string) []string {
	var refs []string
	for line := range strings.SplitSeq(md, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			continue
		}
		parts := strings.Split(line, "`")
		// Odd indices are the contents of inline-code spans.
		for i := 1; i < len(parts); i += 2 {
			tok := parts[i]
			if isPathLike(tok) {
				refs = append(refs, tok)
			}
		}
	}
	return refs
}

// isPathLike reports whether a backtick token should be validated as a path.
func isPathLike(tok string) bool {
	if tok == "" || !strings.Contains(tok, "/") {
		return false
	}
	// Reject URLs, glob patterns, template placeholders, and command
	// snippets — they're not local paths and would produce noise rather
	// than signal.
	if strings.ContainsAny(tok, " *?{}") {
		return false
	}
	if strings.Contains(tok, "://") {
		return false
	}
	if strings.HasPrefix(tok, "/") || strings.HasPrefix(tok, "~") {
		return false // not a repo-relative reference (absolute / home-dir path)
	}
	return true
}

// scanGoroutineRecover flags `go func() { ... }()` launches whose function
// literal body never calls recover(). An unrecovered panic in a goroutine
// crashes the entire process, so each such launch is a latent availability
// bug. Goroutines that call a named function (`go worker()`) are not
// inspected — their recovery, if any, lives in the callee.
func scanGoroutineRecover(root string) ([]finding, error) {
	var out []finding
	err := walkGoFiles(root, func(path, rel string) error {
		if strings.HasSuffix(rel, "_test.go") {
			return nil // test goroutines crashing the test binary is acceptable
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			// Unparseable file: let go build/vet be the source of truth for
			// syntax errors; skip rather than fail the structural scan.
			return nil //nolint:nilerr // intentional skip; build/vet reports syntax errors
		}
		ast.Inspect(file, func(n ast.Node) bool {
			gostmt, ok := n.(*ast.GoStmt)
			if !ok {
				return true
			}
			lit, ok := gostmt.Call.Fun.(*ast.FuncLit)
			if !ok {
				return true // `go namedFunc(...)` — not inspected here
			}
			if !bodyCallsRecover(lit.Body) {
				pos := fset.Position(gostmt.Pos())
				out = append(out, finding{
					Key:    fmt.Sprintf("%s:%d", rel, pos.Line),
					Detail: "go func without recover()",
				})
			}
			return true
		})
		return nil
	})
	return out, err
}

// bodyCallsRecover reports whether a block contains any call to the builtin
// recover (typically inside a deferred closure). This is a deliberate
// heuristic: it accepts both `defer func() { recover() }()` and a direct
// recover() call, and tolerates false negatives for recovery delegated to a
// helper function — the baseline absorbs those until they're addressed.
func bodyCallsRecover(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "recover" {
			found = true
			return false
		}
		return true
	})
	return found
}

// walkGoFiles invokes fn for every non-skipped .go file under root, passing
// both the absolute path and the repo-relative path.
func walkGoFiles(root string, fn func(path, rel string) error) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		return fn(path, filepath.ToSlash(rel))
	})
}

// walkShellFiles invokes fn for every non-skipped .sh/.bash file under root,
// passing both the absolute path and the repo-relative path. Shell is where
// the raw-mem-gate over-fit lives: the ce-xj1b class was hand-typed back into
// untracked watchdog scripts, not the (carefully written) Go probes.
func walkShellFiles(root string, fn func(path, rel string) error) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".sh") && !strings.HasSuffix(path, ".bash") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		return fn(path, filepath.ToSlash(rel))
	})
}

// rawFreeMemoryIdioms are lowercased substrings that indicate a script is
// reading a raw macOS free-page count. On macOS these numbers are near-zero
// permanently (reclaimable pages are parked off the free list), so gating or
// alerting on them deadlocks/false-alarms the very thing they protect — the
// ce-xj1b over-fit class that has been re-embedded three times. `vm_stat`
// alone is intentionally NOT listed: plenty of scripts pipe vm_stat for
// legitimate reporting. We flag only the specific free-page idioms.
var rawFreeMemoryIdioms = []string{
	"pages free",      // `vm_stat | grep "Pages free"`
	"page_free_count", // `sysctl vm.page_free_count`
	"pages_free",
}

// memoryPressureEscapeHatch is the correct macOS free-headroom source. A shell
// script that consults it is doing the right thing, so its presence anywhere
// in the file suppresses a raw-mem-gate finding. This is the deliberate
// low-noise seam: the fix for a flagged script is to route the decision
// through `memory_pressure -Q` (as cmd/vroom-governor and the Go
// circuitbreaker already do).
var memoryPressureEscapeHatch = []string{
	"memory_pressure",
	"pressure_level",
}

// scanRawMemGate flags shell scripts that read a raw free-page memory metric
// without also deferring to memory_pressure. Keys are repo-relative paths so
// the baseline is location-independent. Detail records the first offending
// idiom and line so the fix is obvious.
//
// This is the deterministic enforcement of the harness-hygiene doctrine's
// "hard requirements need hard checks": the RAM-gate over-fit is caught at
// authoring time (a red diff) instead of at 3am as a pipeline stall. It is
// also the reference lint that ce-2vbg asks to run over the (currently
// untracked) host watchdogs once they are brought under version control.
func scanRawMemGate(root string) ([]finding, error) {
	var out []finding
	err := walkShellFiles(root, func(path, rel string) error {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		var (
			idiomFound string
			idiomLine  int
		)
		sc := bufio.NewScanner(f)
		for lineNum := 1; sc.Scan(); lineNum++ {
			line := strings.ToLower(sc.Text())
			for _, hatch := range memoryPressureEscapeHatch {
				if strings.Contains(line, hatch) {
					return nil // careful script: routes through memory_pressure
				}
			}
			if idiomFound == "" {
				for _, idiom := range rawFreeMemoryIdioms {
					if strings.Contains(line, idiom) {
						idiomFound, idiomLine = idiom, lineNum
						break
					}
				}
			}
		}
		if err := sc.Err(); err != nil {
			return err
		}
		if idiomFound != "" {
			out = append(out, finding{
				Key: rel,
				Detail: fmt.Sprintf(
					"gates on raw free-page memory (%q, line %d) without memory_pressure — macOS reports free≈0 permanently (ce-xj1b)",
					idiomFound, idiomLine),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// scanReport holds the per-scan diff between current findings and baseline.
type scanReport struct {
	Scan        string   `json:"scan"`
	Regressions []string `json:"regressions"`
	Fixed       []string `json:"fixed"`
	Current     int      `json:"current"`
	Baseline    int      `json:"baseline"`
	// details maps a key to its human-facing detail for text output.
	details map[string]string
}

// report is the full diff across every scan.
type report struct {
	Scans []scanReport `json:"scans"`
}

func (r report) regressionCount() int {
	n := 0
	for _, s := range r.Scans {
		n += len(s.Regressions)
	}
	return n
}

// diff compares current findings against the baseline, scan by scan.
func diff(current map[string][]finding, bl baseline) report {
	var rep report
	for _, name := range scanNames {
		findings := current[name]
		base := map[string]bool{}
		for _, k := range bl.Findings[name] {
			base[k] = true
		}
		cur := map[string]string{}
		for _, f := range findings {
			cur[f.Key] = f.Detail
		}

		var regressions, fixed []string
		for _, f := range findings {
			if !base[f.Key] {
				regressions = append(regressions, f.Key)
			}
		}
		for k := range base {
			if _, ok := cur[k]; !ok {
				fixed = append(fixed, k)
			}
		}
		sort.Strings(regressions)
		sort.Strings(fixed)

		rep.Scans = append(rep.Scans, scanReport{
			Scan:        name,
			Regressions: regressions,
			Fixed:       fixed,
			Current:     len(findings),
			Baseline:    len(bl.Findings[name]),
			details:     cur,
		})
	}
	return rep
}

// emitText prints a human-readable report and a final verdict.
func emitText(rep report) {
	regressions := rep.regressionCount()
	for _, s := range rep.Scans {
		status := "ok"
		if len(s.Regressions) > 0 {
			status = "REGRESSED"
		}
		fmt.Printf("[%-9s] %-17s current=%d baseline=%d", status, s.Scan, s.Current, s.Baseline)
		if len(s.Fixed) > 0 {
			fmt.Printf(" fixed=%d", len(s.Fixed))
		}
		fmt.Println()
		for _, k := range s.Regressions {
			if d := s.details[k]; d != "" {
				fmt.Printf("    + %s (%s)\n", k, d)
			} else {
				fmt.Printf("    + %s\n", k)
			}
		}
	}

	fmt.Println()
	switch {
	case regressions > 0:
		fmt.Printf("FAIL: %d new finding(s) versus baseline.\n", regressions)
		fmt.Println("Fix the regressions above. If they are intentional, admit them with:")
		fmt.Println("  go run ./cmd/structural-health --update-baseline --accept-new \\")
		fmt.Println("    --reason \"<why>\" --reference \"<bead-or-pr>\"")
	default:
		fmt.Println("PASS: no regressions versus baseline.")
		if fixedTotal(rep) > 0 {
			fmt.Printf("Note: %d baselined finding(s) are now fixed. Tighten the ratchet with:\n", fixedTotal(rep))
			fmt.Println("  go run ./cmd/structural-health --update-baseline")
		}
	}
}

func fixedTotal(rep report) int {
	n := 0
	for _, s := range rep.Scans {
		n += len(s.Fixed)
	}
	return n
}

// emitJSON writes the report as indented JSON to stdout.
func emitJSON(rep report) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}
