package gittest_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// allowedUnsandboxedGitTests lists the test files permitted to build a Git
// command without internal/gittest, each with the reason it is exempt. The
// list is a ratchet: adding an entry is a deliberate, reviewed act, and the
// guard fails on a stale entry so the list cannot rot.
//
// Paths are slash-separated and relative to the repository root.
var allowedUnsandboxedGitTests = map[string]string{
	"internal/gittest/gittest_test.go": "positive control: proves host hooks fire for an unisolated repository, " +
		"without which the isolation assertion would pass vacuously",
	"agm/cmd/agm/scan_test.go": "read-only capability probe: `git rev-parse --git-dir` decides whether to skip " +
		"tests that deliberately read the INVOKING repository. Sandboxing the probe would answer a " +
		"question about a different repository than the one under test. It creates nothing and " +
		"mutates nothing, so no hook can fire",
}

// TestNoUnsandboxedGitInTests is the ce-3knl.1 ratchet. A test that shells out
// to Git with an inherited environment picks up the host's global
// core.hooksPath; on 2026-07-10 that ran the real post-merge hook from two
// temporary repositories and deleted two live worktrees. Isolating the
// existing call sites fixes the tests that exist today — this guard is what
// stops the next one from reintroducing the hazard.
func TestNoUnsandboxedGitInTests(t *testing.T) {
	root := repoRoot(t)

	unexpected := map[string][]int{}
	seenAllowed := map[string]bool{}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)

		lines, scanErr := gitCommandLines(path)
		if scanErr != nil {
			return scanErr
		}
		if len(lines) == 0 {
			return nil
		}
		if _, ok := allowedUnsandboxedGitTests[rel]; ok {
			seenAllowed[rel] = true
			return nil
		}
		unexpected[rel] = lines
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}

	for rel := range allowedUnsandboxedGitTests {
		if !seenAllowed[rel] {
			t.Errorf("stale exemption: %s no longer builds a Git command directly — remove it from "+
				"allowedUnsandboxedGitTests", rel)
		}
	}

	for _, rel := range sortedKeys(unexpected) {
		t.Errorf("%s builds a Git command directly at line(s) %v — route it through "+
			"internal/gittest (gittest.Run / gittest.Command / gittest.Env) so it cannot "+
			"execute a host hook, or add a reviewed entry to allowedUnsandboxedGitTests",
			rel, unexpected[rel])
	}
}

// gitCommandLines returns the lines in path that call exec.Command or
// exec.CommandContext with "git" as the program.
func gitCommandLines(path string) ([]int, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	var lines []int
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "exec" {
			return true
		}
		var nameArg ast.Expr
		switch sel.Sel.Name {
		case "Command", "LookPath":
			if len(call.Args) > 0 {
				nameArg = call.Args[0]
			}
		case "CommandContext":
			if len(call.Args) > 1 {
				nameArg = call.Args[1]
			}
		default:
			return true
		}
		// LookPath("git") is a capability probe, not an invocation; it runs
		// nothing and so cannot reach a hook.
		if sel.Sel.Name == "LookPath" {
			return true
		}
		if isGitLiteral(nameArg) {
			lines = append(lines, fset.Position(call.Pos()).Line)
		}
		return true
	})
	return lines, nil
}

func isGitLiteral(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return false
	}
	return value == "git"
}

func skipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", "testdata", "build", "dist":
		return true
	}
	return false
}

// repoRoot walks up from the test's working directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", dir)
		}
		dir = parent
	}
}

func sortedKeys(m map[string][]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
