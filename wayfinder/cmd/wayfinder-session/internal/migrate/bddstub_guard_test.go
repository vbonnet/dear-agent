package migrate

// Deterministic guards against the reintroduction of "BDD stub theater" — the
// class of defect this package once shipped, where generateFeatureFromS6
// DISCARDED its S6 input and emitted a fixed block of boilerplate Gherkin. That
// fabricated TESTS.feature asserted nothing yet satisfied the orchestrator's
// test-first gate, hiding the absence of real tests.
//
// These guards run under the existing `go test ./...` CI step (and
// `make preflight-tests`), so no new workflow wiring is required. They scan the
// real wayfinder source tree (not the .worktrees copies) and fail loudly if:
//
//  1. any non-test function emits a Gherkin feature literal while ignoring all
//     of its input parameters (an input-discarding "generator"), or
//  2. any checked-in *.feature file is identical (modulo the generation date)
//     to the retired boilerplate template.
//
// If you are adding a *real* EARS→BDD generator, it will reference its input
// and these guards will stay green. If a guard trips, the fix is to make the
// generator actually use its input — not to silence the guard.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// retiredBoilerplateFeature is the exact output of the deleted
// generateFeatureFromS6 stub. It is kept here only so the guard can recognise
// it if it is ever checked in as a .feature file. The "Generated from ... on
// <date>" line is normalised away before comparison.
const retiredBoilerplateFeature = `Feature: Project Implementation

  Generated from S6-design.md on 2026-01-01

  Scenario: Basic functionality works
    Given the system is properly configured
    When a user performs basic operations
    Then the system responds correctly
    And all validations pass

  Scenario: Edge cases are handled
    Given the system encounters edge cases
    When unexpected input is provided
    Then the system handles it gracefully
    And appropriate error messages are shown

  Scenario: Performance requirements are met
    Given the system is under normal load
    When operations are performed
    Then response time is acceptable
    And resource usage is within limits
`

var guardSkipDirs = map[string]bool{
	"vendor":       true,
	"node_modules": true,
	".git":         true,
	".worktrees":   true,
	"testdata":     true,
}

// wayfinderRoot resolves the real wayfinder module subtree from this test
// file's compiled path, so the guard never wanders into .worktrees checkouts.
func wayfinderRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine caller path")
	}
	dir := filepath.Dir(thisFile)
	for {
		if filepath.Base(dir) == "wayfinder" {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate wayfinder root from %s", thisFile)
		}
		dir = parent
	}
}

// walkFiles invokes fn for every file under root with the given suffix,
// skipping guardSkipDirs.
func walkFiles(t *testing.T, root, suffix string, fn func(path string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if guardSkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, suffix) {
			fn(path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
}

// TestGuard_NoInputIgnoringFeatureGenerators fails if any non-test function
// emits a Gherkin feature literal while ignoring all of its input parameters.
// That is the exact shape of the deleted stub and the recurrence we guard.
func TestGuard_NoInputIgnoringFeatureGenerators(t *testing.T) {
	root := wayfinderRoot(t)
	fset := token.NewFileSet()

	var offenders []string
	walkFiles(t, root, ".go", func(path string) {
		if strings.HasSuffix(path, "_test.go") {
			return // guard production code; test fixtures may embed Gherkin
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if emitsGherkin(fn.Body) && ignoresAllInput(fn) {
				pos := fset.Position(fn.Pos())
				offenders = append(offenders, "\t"+pos.String()+": "+fn.Name.Name)
			}
		}
	})

	if len(offenders) > 0 {
		t.Fatalf("found function(s) that emit a hardcoded Gherkin feature while "+
			"ignoring their input — this is BDD stub theater. Make the generator "+
			"actually consume its input, or return a clear not-implemented error "+
			"(see ErrFeatureGenNotImplemented) instead of fabricating a feature:\n%s",
			strings.Join(offenders, "\n"))
	}
}

// emitsGherkin reports whether the body contains a string literal that looks
// like a BDD feature definition.
func emitsGherkin(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		s, err := strconv.Unquote(lit.Value)
		if err != nil {
			s = lit.Value // raw/backtick fallback
		}
		if strings.Contains(s, "Feature:") &&
			(strings.Contains(s, "Scenario:") || strings.Contains(s, "Given ")) {
			found = true
			return false
		}
		return true
	})
	return found
}

// ignoresAllInput reports whether fn declares at least one parameter but
// references none of its named parameters in its body. Blank ("_") and unnamed
// parameters can never be referenced, so a function whose only parameters are
// blank is treated as ignoring all input. A function with no parameters at all
// is not a candidate (it has no input to ignore — e.g. a fixed template helper).
func ignoresAllInput(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil {
		return false
	}
	named := map[string]bool{}
	hasAnyParam := false
	for _, field := range fn.Type.Params.List {
		if len(field.Names) == 0 {
			hasAnyParam = true // unnamed param, e.g. func(string)
			continue
		}
		for _, n := range field.Names {
			hasAnyParam = true
			if n.Name != "_" {
				named[n.Name] = true
			}
		}
	}
	if !hasAnyParam {
		return false
	}

	referenced := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && named[id.Name] {
			referenced = true
			return false
		}
		return true
	})
	return !referenced
}

// TestGuard_NoCheckedInBoilerplateFeatures fails if any checked-in *.feature
// file is the retired boilerplate (modulo its generation-date line).
func TestGuard_NoCheckedInBoilerplateFeatures(t *testing.T) {
	root := wayfinderRoot(t)
	wantNormalized := normalizeFeature(retiredBoilerplateFeature)

	var offenders []string
	walkFiles(t, root, ".feature", func(path string) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if normalizeFeature(string(data)) == wantNormalized {
			offenders = append(offenders, "\t"+path)
		}
	})

	if len(offenders) > 0 {
		t.Fatalf("found checked-in TESTS.feature boilerplate (identical to the "+
			"retired generateFeatureFromS6 output). Replace it with real, "+
			"design-derived scenarios:\n%s", strings.Join(offenders, "\n"))
	}
}

// normalizeFeature drops the volatile generation-date line so boilerplate is
// recognised regardless of the date it was stamped with.
func normalizeFeature(s string) string {
	var kept []string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Generated from S6-design.md on") {
			continue
		}
		kept = append(kept, strings.TrimRight(line, " \t\r"))
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}
