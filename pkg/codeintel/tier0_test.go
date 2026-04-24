package codeintel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckDeadCode_FindsUnusedFunction(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n")
	writeFile(t, dir, "main.go", `package main

func main() {
	helper()
}

func helper() {}

func unused() {}
`)
	writeFile(t, dir, "other.go", `package main

func anotherHelper() {
	helper()
}
`)

	langs := []LanguageSpec{BuiltinSpecs["go"]}
	result := CheckDeadCode(dir, langs, []string{"main.go"})

	if result.Passed {
		t.Fatal("expected dead code check to fail (unused function)")
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "unused()") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'unused' in details, got: %v", result.Details)
	}
}

func TestCheckDeadCode_SkipsEntryPoints(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n")
	writeFile(t, dir, "main.go", `package main

func main() {}

func init() {}
`)

	langs := []LanguageSpec{BuiltinSpecs["go"]}
	result := CheckDeadCode(dir, langs, []string{"main.go"})

	if !result.Passed {
		t.Errorf("expected pass (main/init are entry points), got details: %v", result.Details)
	}
}

func TestCheckDeadCode_SkipsTestFunctions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n")
	writeFile(t, dir, "foo_test.go", `package main

import "testing"

func TestFoo(t *testing.T) {}
func BenchmarkFoo(b *testing.B) {}
func ExampleFoo() {}
`)

	langs := []LanguageSpec{BuiltinSpecs["go"]}
	result := CheckDeadCode(dir, langs, []string{"foo_test.go"})

	if !result.Passed {
		t.Errorf("expected pass (test funcs skipped), got details: %v", result.Details)
	}
}

func TestCheckDeadCode_NoChangedFiles(t *testing.T) {
	dir := t.TempDir()
	langs := []LanguageSpec{BuiltinSpecs["go"]}
	result := CheckDeadCode(dir, langs, nil)

	if !result.Passed {
		t.Error("expected pass with no changed files")
	}
}

func TestCheckDeadCode_Python(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pyproject.toml", "[project]\nname=\"test\"\n")
	writeFile(t, dir, "app.py", `def main():
    used()

def used():
    pass

def orphan():
    pass
`)
	writeFile(t, dir, "other.py", `from app import used
used()
`)

	langs := []LanguageSpec{BuiltinSpecs["python"]}
	result := CheckDeadCode(dir, langs, []string{"app.py"})

	if result.Passed {
		t.Fatal("expected dead code check to fail (orphan function)")
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "orphan()") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'orphan' in details, got: %v", result.Details)
	}
}

func TestCheckDebugPrints_FindsPrints(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n")
	writeFile(t, dir, "main.go", `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`)

	langs := []LanguageSpec{BuiltinSpecs["go"]}
	result := CheckDebugPrints(dir, langs, []string{"main.go"})

	if result.Passed {
		t.Fatal("expected debug prints to be found")
	}
	if result.Severity != "warning" {
		t.Errorf("expected severity=warning, got %s", result.Severity)
	}
}

func TestCheckDebugPrints_NoPrints(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n")
	writeFile(t, dir, "main.go", `package main

func main() {
	_ = 1 + 2
}
`)

	langs := []LanguageSpec{BuiltinSpecs["go"]}
	result := CheckDebugPrints(dir, langs, []string{"main.go"})

	if !result.Passed {
		t.Errorf("expected pass, got details: %v", result.Details)
	}
}

func TestCheckDebugPrints_Python(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app.py", `import sys

def run():
    print("debug info")
    breakpoint()
`)

	langs := []LanguageSpec{BuiltinSpecs["python"]}
	result := CheckDebugPrints(dir, langs, []string{"app.py"})

	if result.Passed {
		t.Fatal("expected debug prints to be found in Python file")
	}
	if len(result.Details) < 2 {
		t.Errorf("expected at least 2 findings (print + breakpoint), got %d: %v", len(result.Details), result.Details)
	}
}

func TestCheckDebugPrints_ShellFixedPattern(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "deploy.sh", `#!/bin/bash
set -x
echo DEBUG something
echo 'DEBUG test'
`)

	langs := []LanguageSpec{BuiltinSpecs["shell"]}
	result := CheckDebugPrints(dir, langs, []string{"deploy.sh"})

	if result.Passed {
		t.Fatal("expected shell debug patterns to match")
	}
	if len(result.Details) < 2 {
		t.Errorf("expected at least 2 findings, got %d: %v", len(result.Details), result.Details)
	}
}

func TestCheckDebugPrints_NoChangedFiles(t *testing.T) {
	dir := t.TempDir()
	langs := []LanguageSpec{BuiltinSpecs["go"]}
	result := CheckDebugPrints(dir, langs, nil)

	if !result.Passed {
		t.Error("expected pass with no changed files")
	}
}

func TestCheckDanglingRefs_NoBuildCmd(t *testing.T) {
	langs := []LanguageSpec{UnknownLanguage}
	result := CheckDanglingRefs(t.TempDir(), langs)

	if !result.Passed {
		t.Error("expected pass when no build cmd available")
	}
	if !strings.Contains(result.Message, "skipped") {
		t.Errorf("expected skip message, got: %s", result.Message)
	}
}

func TestCheckDanglingRefs_GoBuildPass(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n\ngo 1.21\n")
	writeFile(t, dir, "main.go", `package main

func main() {}
`)

	langs := []LanguageSpec{BuiltinSpecs["go"]}
	result := CheckDanglingRefs(dir, langs)

	if !result.Passed {
		t.Errorf("expected build pass, got: %s — %v", result.Message, result.Details)
	}
}

func TestCheckDanglingRefs_GoBuildFail(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n\ngo 1.21\n")
	writeFile(t, dir, "main.go", `package main

func main() {
	doesNotExist()
}
`)

	langs := []LanguageSpec{BuiltinSpecs["go"]}
	result := CheckDanglingRefs(dir, langs)

	if result.Passed {
		t.Fatal("expected build failure")
	}
	if result.Severity != "error" {
		t.Errorf("expected severity=error, got %s", result.Severity)
	}
}

func TestRunTier0Checks_Integration(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n\ngo 1.21\n")
	writeFile(t, dir, "main.go", `package main

import "fmt"

func main() {
	fmt.Println("hello")
}

func unused() {}
`)

	res, err := RunTier0Checks(dir, []string{"main.go"})
	if err != nil {
		t.Fatalf("RunTier0Checks: %v", err)
	}

	if len(res.Languages) == 0 {
		t.Fatal("expected at least one language detected")
	}
	if len(res.Checks) != 3 {
		t.Fatalf("expected 3 checks, got %d", len(res.Checks))
	}

	// Verify check names.
	names := make(map[string]bool)
	for _, c := range res.Checks {
		names[c.Check] = true
	}
	for _, expected := range []string{"dead_code", "dangling_refs", "debug_prints"} {
		if !names[expected] {
			t.Errorf("missing check: %s", expected)
		}
	}
}

func TestRunTier0Checks_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	res, err := RunTier0Checks(dir, nil)
	if err != nil {
		t.Fatalf("RunTier0Checks: %v", err)
	}
	if len(res.Checks) != 1 || res.Checks[0].Check != "detect_languages" {
		t.Errorf("expected single detect_languages check for empty dir, got: %v", res.Checks)
	}
}

func TestExtractFunctions_Go(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	os.WriteFile(path, []byte(`package main

func main() {}
func helper() {}
func (s *Server) Handle() {}
`), 0o644)

	funcs, err := extractFunctions(path, BuiltinSpecs["go"].FunctionPattern)
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]bool{"main": true, "helper": true, "Handle": true}
	for _, f := range funcs {
		delete(expected, f)
	}
	if len(expected) > 0 {
		t.Errorf("missing functions: %v (got: %v)", expected, funcs)
	}
}

func TestExtractFunctions_TypeScript(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.ts")
	os.WriteFile(path, []byte(`
function greet() {}
export function serve() {}
export const handler = () => {}
`), 0o644)

	funcs, err := extractFunctions(path, BuiltinSpecs["typescript"].FunctionPattern)
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]bool{"greet": true, "serve": true, "handler": true}
	for _, f := range funcs {
		delete(expected, f)
	}
	if len(expected) > 0 {
		t.Errorf("missing functions: %v (got: %v)", expected, funcs)
	}
}

func TestBuildExtMap(t *testing.T) {
	langs := []LanguageSpec{BuiltinSpecs["go"], BuiltinSpecs["python"]}
	m := buildExtMap(langs)

	if _, ok := m[".go"]; !ok {
		t.Error("expected .go in ext map")
	}
	if _, ok := m[".py"]; !ok {
		t.Error("expected .py in ext map")
	}
}
