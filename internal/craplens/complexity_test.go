package craplens

import (
	"go/parser"
	"go/token"
	"testing"
)

// TestComplexityMatchesGocyclo pins the counting rules against gocyclo's, since
// golangci-lint already runs gocyclo on this repository and two signals that
// disagree about the same function teach readers to trust neither.
func TestComplexityMatchesGocyclo(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{name: "straight line", body: "func f() { println(1) }", want: 1},
		{name: "one if", body: "func f(a bool) { if a { println(1) } }", want: 2},
		{name: "if else counts once", body: "func f(a bool) { if a { println(1) } else { println(2) } }", want: 2},
		{name: "for loop", body: "func f() { for i := 0; i < 3; i++ { println(i) } }", want: 2},
		{name: "range", body: "func f(xs []int) { for range xs { println(1) } }", want: 2},
		{name: "logical and", body: "func f(a, b bool) { if a && b { println(1) } }", want: 3},
		{name: "logical or", body: "func f(a, b bool) { if a || b { println(1) } }", want: 3},
		{name: "switch cases", body: "func f(n int) { switch n { case 1: case 2: case 3: } }", want: 4},
		// Verified against the gocyclo binary: it walks the whole declaration
		// and counts branches inside a nested closure toward the enclosing
		// function. Scoring the closure separately instead would report 2 here
		// and break the parity CRAPLENS-05 promises.
		{
			name: "nested closure counts toward the enclosing function",
			body: "func f(a bool) func(int) int { if a { return nil }; return func(n int) int { if n > 1 { return 1 }; if n > 2 { return 2 }; if n > 3 { return 3 }; return 0 } }",
			want: 5,
		},
		{name: "default adds no branch", body: "func f(n int) { switch n { case 1: default: } }", want: 2},
		{name: "select default adds no branch", body: "func f(c chan int) { select { case <-c: default: } }", want: 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseSingleFunc(t, tc.body)
			if got := complexity(fn); got != tc.want {
				t.Errorf("complexity = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestFuncNameQualifiesMethods pins that two same-named methods on different
// receivers stay distinguishable in the report.
func TestFuncNameQualifiesMethods(t *testing.T) {
	tests := map[string]string{
		"func f() {}":                 "f",
		"func (s Server) Start() {}":  "(Server).Start",
		"func (s *Server) Start() {}": "(*Server).Start",
		"func (s *Store[T]) Get() {}": "(*Store).Get",
	}

	for body, want := range tests {
		if got := funcName(parseSingleFunc(t, body)); got != want {
			t.Errorf("funcName(%q) = %q, want %q", body, got, want)
		}
	}
}

// TestDeclaredFuncsMatchesGocycloScope pins WHICH declarations are scored,
// which is a separate question from how each one is counted.
//
// Package-level initializer literals are scored even when nested in a
// composite expression, so injected handlers cannot evade the signal.
func TestDeclaredFuncsMatchesGocycloScope(t *testing.T) {
	src := `package p

type cmd struct{ RunE func(int) error }

func Plain() {}

var Bare = func(n int) int {
	if n > 1 {
		return 1
	}
	return 0
}

var Composite = &cmd{RunE: func(n int) error { return nil }}
`
	file, err := parser.ParseFile(token.NewFileSet(), "p.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	var names []string
	for _, decl := range file.Decls {
		for _, cand := range declaredFuncs(decl) {
			names = append(names, cand.name)
		}
	}

	want := []string{"Plain", "Bare", "Composite"}
	if len(names) != len(want) {
		t.Fatalf("scored %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("scored[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}

// TestDeclaredFuncsDoesNotDoubleCountNestedClosures guards against a real
// regression in the composite-literal walk above: ast.Inspect keeps
// descending into a matched *ast.FuncLit's own body unless told to stop, so
// a closure DEFINED INSIDE a package-level func literal (not merely wrapped
// by a non-function expression) was being surfaced a second time under the
// same variable name — complexity() already walks a function's whole body,
// so its inner closures' branches are counted once as part of it; listing
// the inner closure again as its own same-named candidate would both
// duplicate that branch complexity and render the same name twice in a
// report.
func TestDeclaredFuncsDoesNotDoubleCountNestedClosures(t *testing.T) {
	src := `package p

var Bare = func() {
	inner := func() {
		if true {
		}
	}
	inner()
}
`
	file, err := parser.ParseFile(token.NewFileSet(), "p.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	var candidates []candidate
	for _, decl := range file.Decls {
		candidates = append(candidates, declaredFuncs(decl)...)
	}
	if len(candidates) != 1 {
		t.Fatalf("declaredFuncs returned %d candidates, want exactly 1 (the outer closure only): %+v", len(candidates), candidates)
	}
	if got := complexity(candidates[0].node); got != 2 {
		t.Errorf("complexity(Bare) = %d, want 2 (1 base + 1 for the inner closure's if, already absorbed)", got)
	}
}
