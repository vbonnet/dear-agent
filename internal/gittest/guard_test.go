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

// allowedUnsandboxedGitTests lists the exact direct-Git call sites permitted
// without internal/gittest, each with the reason it is exempt. The list is a
// ratchet: adding a call site is a deliberate, reviewed act, and the guard
// fails on a stale entry so the list cannot rot.
//
// Paths are slash-separated and relative to the repository root.
var allowedUnsandboxedGitTests = map[string]map[int]string{
	"internal/gittest/gittest_test.go": {75: "positive control: proves host hooks fire for an unisolated repository, " +
		"without which the isolation assertion would pass vacuously",
	},
	"agm/cmd/agm/scan_test.go": {118: "read-only capability probe: `git rev-parse --git-dir` decides whether to skip " +
		"tests that deliberately read the INVOKING repository. Sandboxing the probe would answer a " +
		"question about a different repository than the one under test. It creates nothing and " +
		"mutates nothing, so no hook can fire", 234: "same read-only capability probe as line 118",
	},
	"internal/deploy/launchd_install_test.go": {526: "read-only `git ls-files` inventories the repository under test; it creates " +
		"nothing and cannot execute a Git hook",
	},
	"pkg/instructionlint/instructionlint_test.go": {1750: "read-only `git rev-parse --show-toplevel` locates the checked-out fixture root; " +
		"it creates nothing and cannot execute a Git hook",
	},
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
	seenAllowed := map[string]map[int]bool{}

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
		if allowed, ok := allowedUnsandboxedGitTests[rel]; ok {
			for _, line := range lines {
				if _, allowed := allowed[line]; !allowed {
					unexpected[rel] = append(unexpected[rel], line)
					continue
				}
				if seenAllowed[rel] == nil {
					seenAllowed[rel] = map[int]bool{}
				}
				seenAllowed[rel][line] = true
			}
			return nil
		}
		unexpected[rel] = lines
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}

	for rel, allowed := range allowedUnsandboxedGitTests {
		for line := range allowed {
			if !seenAllowed[rel][line] {
				t.Errorf("stale exemption: %s:%d no longer builds the reviewed Git command directly — remove it from "+
					"allowedUnsandboxedGitTests", rel, line)
			}
		}
	}

	for _, rel := range sortedKeys(unexpected) {
		t.Errorf("%s builds a Git command directly at line(s) %v — route it through "+
			"internal/gittest (gittest.Run / gittest.Command / gittest.Env) so it cannot "+
			"execute a host hook, or add a reviewed entry to allowedUnsandboxedGitTests",
			rel, unexpected[rel])
	}
}

// gitCommandLines returns the lines in path that build a Git command directly
// or pass "git" through a local helper that builds one. The latter catches
// wrappers such as run(t, dir, name, args...) which otherwise hide Git behind
// an identifier and inherit host hooks just as surely as a literal command.
func gitCommandLines(path string) ([]int, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	helperProgramArg := gitCommandHelpers(file)
	var lines []int
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if name, ok := call.Fun.(*ast.Ident); ok {
			if argIndex, helper := helperProgramArg[name.Name]; helper && len(call.Args) > argIndex && isGitLiteral(call.Args[argIndex]) {
				lines = append(lines, fset.Position(call.Pos()).Line)
			}
			return true
		}
		if gitProgramArg(call) != nil && isGitLiteral(gitProgramArg(call)) {
			lines = append(lines, fset.Position(call.Pos()).Line)
		}
		return true
	})
	return lines, nil
}

// gitCommandHelpers reports local functions whose named parameter becomes the
// program argument to exec.Command or exec.CommandContext. Callers passing
// "git" to one of these wrappers need the same sandbox as direct commands.
func gitCommandHelpers(file *ast.File) map[string]int {
	helpers := make(map[string]int)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Type.Params == nil {
			continue
		}
		params := make(map[string]int)
		index := 0
		for _, field := range fn.Type.Params.List {
			for _, name := range field.Names {
				params[name.Name] = index
				index++
			}
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			program := gitProgramArgFromCall(n)
			name, ok := program.(*ast.Ident)
			if !ok {
				return true
			}
			if paramIndex, ok := params[name.Name]; ok {
				helpers[fn.Name.Name] = paramIndex
			}
			return true
		})
	}
	return helpers
}

func gitProgramArgFromCall(n ast.Node) ast.Expr {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return nil
	}
	return gitProgramArg(call)
}

// gitProgramArg returns the executable argument for an exec command call.
// LookPath is excluded because it does not execute a hook-capable process.
func gitProgramArg(call *ast.CallExpr) ast.Expr {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "exec" {
		return nil
	}
	switch sel.Sel.Name {
	case "Command":
		if len(call.Args) > 0 {
			return call.Args[0]
		}
	case "CommandContext":
		if len(call.Args) > 1 {
			return call.Args[1]
		}
	}
	return nil
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

func TestGitCommandLines_DetectsGitWrapper(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wrapper_test.go")
	content := `package fixture
import ("os/exec"; "testing")
func run(t testing.TB, name string, args ...string) { _ = exec.Command(name, args...) }
func TestGit(t *testing.T) { run(t, "git", "init") }
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	lines, err := gitCommandLines(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 {
		t.Fatalf("gitCommandLines wrapper = %v, want one call site", lines)
	}
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
