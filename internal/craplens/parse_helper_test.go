package craplens

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// parseSingleFunc parses a snippet holding exactly one function declaration and
// returns it, so the complexity and naming tests can stay one line each.
func parseSingleFunc(t *testing.T, body string) *ast.FuncDecl {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "snippet.go", "package p\n"+body+"\n", parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %q: %v", body, err)
	}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			return fn
		}
	}
	t.Fatalf("no function declaration in %q", body)
	return nil
}
