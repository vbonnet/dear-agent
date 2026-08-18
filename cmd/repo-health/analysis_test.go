package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

// parseFunc parses src and returns the first function declaration plus the
// fileset, for the AST-metric tests.
func parseFunc(t *testing.T, src string) (*token.FileSet, *ast.FuncDecl) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok {
			return fset, fn
		}
	}
	t.Fatal("no func decl found")
	return nil, nil
}

func TestCyclomatic(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{"straight line", `package p
func f() { a := 1; _ = a }`, 1},
		{"one if", `package p
func f(x int) { if x > 0 { } }`, 2},
		{"if and for", `package p
func f(x int) { if x > 0 { } ; for i := 0; i < x; i++ { } }`, 3},
		{"logical ops", `package p
func f(a, b bool) { if a && b || a { } }`, 4}, // 1 base + if + && + ||
		{"switch cases", `package p
func f(x int) { switch x { case 1: case 2: } }`, 3},
	}
	for _, c := range cases {
		fset, fn := parseFunc(t, c.src)
		_ = fset
		if got := cyclomatic(fn); got != c.want {
			t.Errorf("%s: cyclomatic = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestFuncLineSpan(t *testing.T) {
	src := `package p
func f() {
	a := 1
	_ = a
}`
	fset, fn := parseFunc(t, src)
	if got := funcLineSpan(fset, fn); got != 4 { // lines 2..5
		t.Errorf("funcLineSpan = %d, want 4", got)
	}
}

func TestParseGoFilesExcludesAndCounts(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "package p\nfunc A() {}\n")
	write("a_test.go", "package p\nfunc TestA() {}\n")
	write("vendor/v.go", "package v\nfunc V() {}\n")       // excluded dir
	write(".worktrees/w/x.go", "package w\nfunc W() {}\n") // excluded dir
	write("broken.go", "package p\nfunc (\n")              // unparseable

	sources, skipped := parseGoFiles(root)
	if len(sources) != 2 {
		t.Errorf("parsed %d sources, want 2 (a.go, a_test.go); got %v", len(sources), sourcePaths(sources))
	}
	if len(skipped) != 1 {
		t.Errorf("skipped %d, want 1 (broken.go); got %v", len(skipped), skipped)
	}
	var tests, prod int
	for _, s := range sources {
		if s.isTest {
			tests++
		} else {
			prod++
		}
	}
	if tests != 1 || prod != 1 {
		t.Errorf("got %d test / %d prod, want 1/1", tests, prod)
	}
}

func TestParseGoFilesHonorsRepositoryIgnoreAndGeneratedPolicy(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cmd := gittest.Command(t, root, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	write := func(rel string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package p\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("visible.go")
	write("ignored/hidden.go")
	write("dist/generated.go")

	sources, skipped := parseGoFiles(root)
	if len(skipped) != 0 {
		t.Fatalf("unexpected skipped files: %v", skipped)
	}
	if len(sources) != 1 || filepath.Base(sources[0].path) != "visible.go" {
		t.Fatalf("parsed sources = %v, want visible.go only", sourcePaths(sources))
	}
}

func sourcePaths(ss []goSource) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.path
	}
	return out
}

func TestExtractJSON(t *testing.T) {
	cases := map[string]string{
		`{"a":1}`:                  `{"a":1}`,
		"warning: x\n{\"a\":1}":    `{"a":1}`,
		"no json here":             ``,
		"prefix {\"Issues\":[]} z": `{"Issues":[]}`,
	}
	for in, want := range cases {
		if got := extractJSON(in); got != want {
			t.Errorf("extractJSON(%q) = %q, want %q", in, got, want)
		}
	}
}
