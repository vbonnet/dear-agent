package main

import (
	"reflect"
	"sort"
	"testing"
	"time"
)

// pkgFixture returns a small synthetic module: three packages
// (a, b, c), where b imports a and c is independent. b has tests,
// c has tests, a has none.
func pkgFixture() []*goListPackage {
	mainMod := &struct {
		Path string
		Main bool
	}{Path: "example.com/m", Main: true}
	return []*goListPackage{
		{
			ImportPath: "example.com/m/a",
			Dir:        "/repo/a",
			GoFiles:    []string{"a.go"},
			Module:     mainMod,
		},
		{
			ImportPath:  "example.com/m/b",
			Dir:         "/repo/b",
			GoFiles:     []string{"b.go"},
			TestGoFiles: []string{"b_test.go"},
			Imports:     []string{"example.com/m/a"},
			Deps:        []string{"example.com/m/a"},
			Module:      mainMod,
		},
		{
			ImportPath:  "example.com/m/c",
			Dir:         "/repo/c",
			GoFiles:     []string{"c.go"},
			TestGoFiles: []string{"c_test.go"},
			Module:      mainMod,
		},
	}
}

func TestDecide_NoChangesReturnsEmpty(t *testing.T) {
	pkgs := pkgFixture()
	d := decide(options{root: "/repo"}, nil, pkgs)
	if d.forceFull {
		t.Fatal("empty diff should not force full run")
	}
	if len(d.changedPkgs) != 0 {
		t.Fatalf("expected no changed packages, got %v", d.changedPkgs)
	}
	if got := selectPackages(pkgs, d, false); len(got) != 0 {
		t.Fatalf("expected no selected packages, got %v", got)
	}
}

func TestDecide_LeafChangeAffectsDependent(t *testing.T) {
	pkgs := pkgFixture()
	d := decide(options{root: "/repo"}, []string{"a/a.go"}, pkgs)
	if d.forceFull {
		t.Fatal("a/a.go change should not force full run")
	}
	if _, ok := d.changedPkgs["example.com/m/a"]; !ok {
		t.Fatalf("expected a to be in changed packages, got %v", d.changedPkgs)
	}

	got := selectPackages(pkgs, d, false)
	sort.Strings(got)
	want := []string{"example.com/m/b"}
	if !sliceEqual(got, want) {
		t.Fatalf("selectPackages: got %v, want %v", got, want)
	}
}

func TestDecide_IndependentChangeDoesNotAffectOthers(t *testing.T) {
	pkgs := pkgFixture()
	d := decide(options{root: "/repo"}, []string{"c/c.go"}, pkgs)
	got := selectPackages(pkgs, d, false)
	want := []string{"example.com/m/c"}
	if !sliceEqual(got, want) {
		t.Fatalf("selectPackages: got %v, want %v", got, want)
	}
}

func TestDecide_GoModForcesFullRun(t *testing.T) {
	pkgs := pkgFixture()
	d := decide(options{root: "/repo"}, []string{"go.mod"}, pkgs)
	if !d.forceFull {
		t.Fatal("go.mod change must force full run")
	}
	got := selectPackages(pkgs, d, false)
	// Both test-bearing packages should be returned; the non-test
	// package a should be excluded.
	want := []string{"example.com/m/b", "example.com/m/c"}
	sort.Strings(got)
	if !sliceEqual(got, want) {
		t.Fatalf("selectPackages: got %v, want %v", got, want)
	}
}

func TestDecide_GoWorkSumForcesFullRun(t *testing.T) {
	d := decide(options{root: "/repo"}, []string{"go.work.sum"}, pkgFixture())
	if !d.forceFull {
		t.Fatal("go.work.sum change must force full run")
	}
}

func TestDecide_CIWorkflowChangeForcesFullRun(t *testing.T) {
	d := decide(options{root: "/repo"}, []string{".github/workflows/ci.yml"}, pkgFixture())
	if !d.forceFull {
		t.Fatal("CI workflow change must force full run")
	}
}

func TestDecide_TestAffectedSelfChangeForcesFullRun(t *testing.T) {
	d := decide(options{root: "/repo"}, []string{"cmd/test-affected/main.go"}, pkgFixture())
	if !d.forceFull {
		t.Fatal("changing the selector itself must force full run (we can't trust its own output)")
	}
}

func TestDecide_TestdataChangeMapsToParentPackage(t *testing.T) {
	pkgs := pkgFixture()
	d := decide(options{root: "/repo"}, []string{"b/testdata/golden/case1.json"}, pkgs)
	if _, ok := d.changedPkgs["example.com/m/b"]; !ok {
		t.Fatalf("expected b to be picked up via testdata walk-up, got %v", d.changedPkgs)
	}
	got := selectPackages(pkgs, d, false)
	want := []string{"example.com/m/b"}
	if !sliceEqual(got, want) {
		t.Fatalf("selectPackages: got %v, want %v", got, want)
	}
}

func TestDecide_RootLevelPackageIsCaughtByWalkUp(t *testing.T) {
	// Synthetic package at the repo root: Dir == "/repo", which
	// filepath.Rel(opts.root, p.Dir) renders as ".".
	rootPkg := &goListPackage{
		ImportPath:  "example.com/m",
		Dir:         "/repo",
		GoFiles:     []string{"root.go"},
		TestGoFiles: []string{"root_test.go"},
		Module: &struct {
			Path string
			Main bool
		}{Path: "example.com/m", Main: true},
	}
	pkgs := append([]*goListPackage{rootPkg}, pkgFixture()...)
	d := decide(options{root: "/repo"}, []string{"root.go"}, pkgs)
	if d.forceFull {
		t.Fatal("root.go change must not force full")
	}
	if _, ok := d.changedPkgs["example.com/m"]; !ok {
		t.Fatalf("walk-up should have reached the root package; got %v", d.changedPkgs)
	}
}

func TestDecide_UnknownPathIsIgnored(t *testing.T) {
	pkgs := pkgFixture()
	d := decide(options{root: "/repo"}, []string{"docs/some-note.md"}, pkgs)
	if d.forceFull {
		t.Fatal("docs change should not force full")
	}
	if len(d.changedPkgs) != 0 {
		t.Fatalf("expected no changed packages, got %v", d.changedPkgs)
	}
}

func TestSelectPackages_EmitAllIncludesNonTestPackages(t *testing.T) {
	pkgs := pkgFixture()
	d := decide(options{root: "/repo"}, []string{"a/a.go"}, pkgs)
	got := selectPackages(pkgs, d, true)
	sort.Strings(got)
	// emitAll should include a (the changed package itself) and b (its
	// dependent). c is unaffected.
	want := []string{"example.com/m/a", "example.com/m/b"}
	if !sliceEqual(got, want) {
		t.Fatalf("selectPackages emitAll: got %v, want %v", got, want)
	}
}

func TestSelectPackages_ExcludesThirdPartyAndStdlib(t *testing.T) {
	thirdParty := &goListPackage{
		ImportPath:  "github.com/external/dep",
		TestGoFiles: []string{"x_test.go"},
		Module: &struct {
			Path string
			Main bool
		}{Path: "github.com/external/dep", Main: false},
	}
	stdlib := &goListPackage{
		ImportPath:  "fmt",
		TestGoFiles: []string{"y_test.go"},
		Standard:    true,
	}
	pkgs := append(pkgFixture(), thirdParty, stdlib)
	d := decide(options{root: "/repo"}, []string{"go.mod"}, pkgs)
	got := selectPackages(pkgs, d, false)
	sort.Strings(got)
	want := []string{"example.com/m/b", "example.com/m/c"}
	if !sliceEqual(got, want) {
		t.Fatalf("selectPackages must exclude third-party and stdlib: got %v, want %v", got, want)
	}
}

func TestIsForceFullPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"go.mod", true},
		{"go.sum", true},
		{"Makefile", true},
		{".github/workflows/ci.yml", true},
		{".github/workflows/agm-e2e-install.yml", true},
		{"cmd/test-affected/main.go", true},
		{"agm/cmd/agm/main.go", false},
		{"docs/adr/ADR-001-monorepo-consolidation.md", false},
		{"README.md", false},
	}
	for _, tc := range cases {
		if got := isForceFullPath(tc.path); got != tc.want {
			t.Errorf("isForceFullPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestGoTestArgsPassesRequiredTimeout(t *testing.T) {
	got := goTestArgs(options{tags: "integration,contract"}, []string{"example.com/m/b", "example.com/m/c"})
	want := []string{
		"test",
		"-race",
		"-count=1",
		"-timeout=20m0s",
		"-tags=integration,contract",
		"example.com/m/b",
		"example.com/m/c",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("goTestArgs() = %v, want %v", got, want)
	}
}

func TestGoTestCommandBudgetPreservesStartupGrace(t *testing.T) {
	const minimumStartupGrace = 10 * time.Minute
	if goTestStartupGrace < minimumStartupGrace {
		t.Fatalf(
			"go test startup grace %s must be at least %s",
			goTestStartupGrace,
			minimumStartupGrace,
		)
	}
	if goTestCommandTimeout != goTestPackageTimeout+goTestStartupGrace {
		t.Fatalf(
			"aggregate go test timeout %s must equal package timeout %s plus startup grace %s",
			goTestCommandTimeout,
			goTestPackageTimeout,
			goTestStartupGrace,
		)
	}
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
