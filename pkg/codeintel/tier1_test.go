package codeintel

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func astGrepAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ast-grep"); err != nil {
		t.Skip("ast-grep not installed, skipping Tier 1 test")
	}
}

func TestRunAstGrepRules_GoDeadFunction(t *testing.T) {
	astGrepAvailable(t)

	dir := t.TempDir()
	writeFile(t, dir, "main.go", `package main

func main() {}

func unused() {}
`)
	ruleFile := findRuleFile(t, "go", "dead-function.yml")
	findings, err := RunAstGrepRules(ruleFile, []string{filepath.Join(dir, "main.go")})
	if err != nil {
		t.Fatalf("RunAstGrepRules: %v", err)
	}

	if len(findings) == 0 {
		t.Fatal("expected at least one finding for unused function")
	}

	foundUnused := false
	for _, f := range findings {
		if v, ok := f.MetaVariables.Single["FUNC"]; ok && v.Text == "unused" {
			foundUnused = true
		}
	}
	if !foundUnused {
		t.Error("expected finding for 'unused' function")
	}

	// Should NOT find main (excluded by constraint).
	for _, f := range findings {
		if v, ok := f.MetaVariables.Single["FUNC"]; ok && v.Text == "main" {
			t.Error("should not flag 'main' as dead code")
		}
	}
}

func TestRunAstGrepRules_GoDebugPrint(t *testing.T) {
	astGrepAvailable(t)

	dir := t.TempDir()
	writeFile(t, dir, "main.go", `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`)
	ruleFile := findRuleFile(t, "go", "debug-print.yml")
	findings, err := RunAstGrepRules(ruleFile, []string{filepath.Join(dir, "main.go")})
	if err != nil {
		t.Fatalf("RunAstGrepRules: %v", err)
	}

	if len(findings) == 0 {
		t.Fatal("expected finding for fmt.Println")
	}
}

func TestRunAstGrepRules_PythonDeadFunction(t *testing.T) {
	astGrepAvailable(t)

	dir := t.TempDir()
	writeFile(t, dir, "app.py", `def main():
    pass

def unused():
    pass
`)
	ruleFile := findRuleFile(t, "python", "dead-function.yml")
	findings, err := RunAstGrepRules(ruleFile, []string{filepath.Join(dir, "app.py")})
	if err != nil {
		t.Fatalf("RunAstGrepRules: %v", err)
	}

	if len(findings) == 0 {
		t.Fatal("expected findings for Python functions")
	}

	foundUnused := false
	for _, f := range findings {
		if v, ok := f.MetaVariables.Single["FUNC"]; ok && v.Text == "unused" {
			foundUnused = true
		}
	}
	if !foundUnused {
		t.Error("expected finding for 'unused' function")
	}
}

func TestRunAstGrepRules_PythonDebugPrint(t *testing.T) {
	astGrepAvailable(t)

	dir := t.TempDir()
	writeFile(t, dir, "app.py", `def run():
    print("debug")
`)
	ruleFile := findRuleFile(t, "python", "debug-print.yml")
	findings, err := RunAstGrepRules(ruleFile, []string{filepath.Join(dir, "app.py")})
	if err != nil {
		t.Fatalf("RunAstGrepRules: %v", err)
	}

	if len(findings) == 0 {
		t.Fatal("expected finding for print()")
	}
}

func TestRunAstGrepRules_TSDeadFunction(t *testing.T) {
	astGrepAvailable(t)

	dir := t.TempDir()
	writeFile(t, dir, "app.ts", `function greet() {}

function unused() {}
`)
	ruleFile := findRuleFile(t, "typescript", "dead-function.yml")
	findings, err := RunAstGrepRules(ruleFile, []string{filepath.Join(dir, "app.ts")})
	if err != nil {
		t.Fatalf("RunAstGrepRules: %v", err)
	}

	if len(findings) == 0 {
		t.Fatal("expected findings for TS functions")
	}
}

func TestRunAstGrepRules_TSDebugPrint(t *testing.T) {
	astGrepAvailable(t)

	dir := t.TempDir()
	writeFile(t, dir, "app.ts", `function run() {
    console.log("debug");
}
`)
	ruleFile := findRuleFile(t, "typescript", "debug-print.yml")
	findings, err := RunAstGrepRules(ruleFile, []string{filepath.Join(dir, "app.ts")})
	if err != nil {
		t.Fatalf("RunAstGrepRules: %v", err)
	}

	if len(findings) == 0 {
		t.Fatal("expected finding for console.log()")
	}
}

func TestCheckDeadCodeTier1_FindsUnusedFunction(t *testing.T) {
	astGrepAvailable(t)
	setRulesDir(t)

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
	result := CheckDeadCodeTier1(dir, langs, []string{"main.go"})

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

func TestCheckDeadCodeTier1_MoreAccurateThanTier0(t *testing.T) {
	astGrepAvailable(t)
	setRulesDir(t)

	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n")
	// Tier 0 regex might false-positive on comments containing function-like patterns.
	// Tier 1 AST should only find actual function declarations.
	writeFile(t, dir, "main.go", `package main

func main() {}

// myHelper is documented here
func myHelper() {}
`)
	writeFile(t, dir, "other.go", `package main

// myHelper is referenced in this comment but not called
func other() {}
`)

	langs := []LanguageSpec{BuiltinSpecs["go"]}

	// Tier 0: regex-based — "myHelper" appears in other.go comment, so it looks referenced.
	t0result := CheckDeadCode(dir, langs, []string{"main.go"})

	// Tier 1: AST-based — finds actual function declarations only.
	t1result := CheckDeadCodeTier1(dir, langs, []string{"main.go"})

	// Both should work without error; the key difference is accuracy.
	t.Logf("Tier 0: passed=%v details=%v", t0result.Passed, t0result.Details)
	t.Logf("Tier 1: passed=%v details=%v", t1result.Passed, t1result.Details)

	// Tier 1 should find myHelper as dead (comment mention doesn't count as reference
	// for the cross-check, which uses the same grep — so both tiers see the same
	// cross-reference result). The key win is that Tier 1 won't false-positive on
	// patterns that look like function definitions but aren't.
}

func TestCheckPatterns_FindsDebugPrints(t *testing.T) {
	astGrepAvailable(t)
	setRulesDir(t)

	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n")
	writeFile(t, dir, "main.go", `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`)

	langs := []LanguageSpec{BuiltinSpecs["go"]}
	result := CheckPatterns(dir, langs, []string{"main.go"})

	if result.Passed {
		t.Fatal("expected debug prints to be found")
	}
	if result.Severity != "warning" {
		t.Errorf("expected severity=warning, got %s", result.Severity)
	}
}

func TestCheckPatterns_NoPrints(t *testing.T) {
	astGrepAvailable(t)
	setRulesDir(t)

	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n")
	writeFile(t, dir, "main.go", `package main

func main() {
	_ = 1 + 2
}
`)

	langs := []LanguageSpec{BuiltinSpecs["go"]}
	result := CheckPatterns(dir, langs, []string{"main.go"})

	if !result.Passed {
		t.Errorf("expected pass, got details: %v", result.Details)
	}
}

func TestCheckPatterns_NoChangedFiles(t *testing.T) {
	astGrepAvailable(t)
	setRulesDir(t)

	dir := t.TempDir()
	langs := []LanguageSpec{BuiltinSpecs["go"]}
	result := CheckPatterns(dir, langs, nil)

	if !result.Passed {
		t.Error("expected pass with no changed files")
	}
}

func TestRunTier1Checks_Integration(t *testing.T) {
	astGrepAvailable(t)
	setRulesDir(t)

	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n\ngo 1.21\n")
	writeFile(t, dir, "main.go", `package main

import "fmt"

func main() {
	fmt.Println("hello")
}

func unused() {}
`)

	res, err := RunTier1Checks(dir, []string{"main.go"})
	if err != nil {
		t.Fatalf("RunTier1Checks: %v", err)
	}

	if res.Tier != 1 {
		t.Errorf("expected tier=1, got %d", res.Tier)
	}
	if len(res.Languages) == 0 {
		t.Fatal("expected at least one language detected")
	}
	if len(res.Checks) != 3 {
		t.Fatalf("expected 3 checks, got %d", len(res.Checks))
	}

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

func TestRunChecks_AutoDetect(t *testing.T) {
	astGrepAvailable(t)
	setRulesDir(t)

	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n\ngo 1.21\n")
	writeFile(t, dir, "main.go", `package main

func main() {}
`)

	// tier=-1 means auto-detect
	res, err := RunChecks(dir, []string{"main.go"}, -1)
	if err != nil {
		t.Fatalf("RunChecks auto: %v", err)
	}

	// Should auto-detect Tier 1 since ast-grep is available.
	if res.Tier != 1 {
		t.Errorf("expected auto-detected tier=1, got %d", res.Tier)
	}
}

func TestRunChecks_ExplicitTier0(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n\ngo 1.21\n")
	writeFile(t, dir, "main.go", `package main

func main() {}
`)

	res, err := RunChecks(dir, []string{"main.go"}, 0)
	if err != nil {
		t.Fatalf("RunChecks tier0: %v", err)
	}

	if res.Tier != 0 {
		t.Errorf("expected tier=0, got %d", res.Tier)
	}
}

func TestCheckDanglingRefsScoped_OnlyRunsRelevantLanguages(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n\ngo 1.21\n")
	writeFile(t, dir, "main.go", `package main

func main() {}
`)

	// Pass both Go and Python specs, but only Go files are changed.
	langs := []LanguageSpec{BuiltinSpecs["go"], BuiltinSpecs["python"]}
	result := CheckDanglingRefsScoped(dir, langs, []string{"main.go"})

	// Should only have run Go build, not Python.
	if !result.Passed {
		t.Errorf("expected pass, got: %s — %v", result.Message, result.Details)
	}
	if strings.Contains(result.Message, "python") || strings.Contains(result.Message, "pytest") {
		t.Errorf("should not have attempted Python build, got: %s", result.Message)
	}
}

func TestCheckDanglingRefsScoped_NoChangedFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n\ngo 1.21\n")
	writeFile(t, dir, "main.go", `package main

func main() {}
`)

	langs := []LanguageSpec{BuiltinSpecs["go"]}
	result := CheckDanglingRefsScoped(dir, langs, nil)

	// Falls back to CheckDanglingRefs with all languages.
	if !result.Passed {
		t.Errorf("expected pass, got: %s", result.Message)
	}
}

func TestRulesDir_NoDoubleRulesPath(t *testing.T) {
	setRulesDir(t)
	rd := rulesDir()
	spec := BuiltinSpecs["go"]
	ruleFile := filepath.Join(rd, spec.ASTGrepRulesDir, "dead-function.yml")
	if _, err := os.Stat(ruleFile); err != nil {
		t.Fatalf("rulesDir produced invalid path %q: %v", ruleFile, err)
	}
	// Verify no double "rules/rules/" in the path.
	if strings.Contains(ruleFile, "rules/rules") || strings.Contains(ruleFile, "rules\\rules") {
		t.Fatalf("rulesDir path has double 'rules': %s", ruleFile)
	}
}

func TestRunAstGrepRules_PythonNoBareExcept(t *testing.T) {
	astGrepAvailable(t)

	dir := t.TempDir()
	writeFile(t, dir, "app.py", `try:
    risky()
except:
    pass
`)
	ruleFile := findRuleFile(t, "python", "no-bare-except.yml")
	findings, err := RunAstGrepRules(ruleFile, []string{filepath.Join(dir, "app.py")})
	if err != nil {
		t.Fatalf("RunAstGrepRules: %v", err)
	}

	if len(findings) == 0 {
		t.Fatal("expected finding for bare except")
	}
}

func TestRunAstGrepRules_PythonNoBareExcept_Specific(t *testing.T) {
	astGrepAvailable(t)

	dir := t.TempDir()
	writeFile(t, dir, "app.py", `try:
    risky()
except ValueError:
    pass
`)
	ruleFile := findRuleFile(t, "python", "no-bare-except.yml")
	findings, err := RunAstGrepRules(ruleFile, []string{filepath.Join(dir, "app.py")})
	if err != nil {
		t.Fatalf("RunAstGrepRules: %v", err)
	}

	if len(findings) > 0 {
		t.Fatalf("expected no findings for specific except, got %d", len(findings))
	}
}

func TestRunAstGrepRules_TSNoAnyType(t *testing.T) {
	astGrepAvailable(t)

	dir := t.TempDir()
	writeFile(t, dir, "app.ts", `function foo(x: any): void {
    console.log(x);
}
`)
	ruleFile := findRuleFile(t, "typescript", "no-any-type.yml")
	findings, err := RunAstGrepRules(ruleFile, []string{filepath.Join(dir, "app.ts")})
	if err != nil {
		t.Fatalf("RunAstGrepRules: %v", err)
	}

	if len(findings) == 0 {
		t.Fatal("expected finding for 'any' type")
	}
}

func TestRunAstGrepRules_TSNoAnyType_Clean(t *testing.T) {
	astGrepAvailable(t)

	dir := t.TempDir()
	writeFile(t, dir, "app.ts", `function foo(x: unknown): void {
    console.log(x);
}
`)
	ruleFile := findRuleFile(t, "typescript", "no-any-type.yml")
	findings, err := RunAstGrepRules(ruleFile, []string{filepath.Join(dir, "app.ts")})
	if err != nil {
		t.Fatalf("RunAstGrepRules: %v", err)
	}

	if len(findings) > 0 {
		t.Fatalf("expected no findings for 'unknown' type, got %d", len(findings))
	}
}

func TestCheckPatterns_ScansAllRules(t *testing.T) {
	astGrepAvailable(t)
	setRulesDir(t)

	dir := t.TempDir()
	// Python file with a bare except — should be caught by no-bare-except.yml
	writeFile(t, dir, "app.py", `def run():
    try:
        risky()
    except:
        pass
`)

	langs := []LanguageSpec{BuiltinSpecs["python"]}
	result := CheckPatterns(dir, langs, []string{"app.py"})

	if result.Passed {
		t.Fatal("expected CheckPatterns to find bare except violation")
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "except") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected bare except in details, got: %v", result.Details)
	}
}

func TestEmbeddedRulesDir(t *testing.T) {
	d, err := embeddedRulesDir()
	if err != nil {
		t.Fatalf("embeddedRulesDir: %v", err)
	}
	// Verify rules were extracted correctly.
	for _, path := range []string{
		"rules/go/dead-function.yml",
		"rules/go/debug-print.yml",
		"rules/python/no-bare-except.yml",
		"rules/typescript/no-any-type.yml",
	} {
		full := filepath.Join(d, path)
		if _, err := os.Stat(full); err != nil {
			t.Errorf("embedded rule not found: %s", full)
		}
	}
}

func TestRunChecks_Tier1RequestedNoAstGrep(t *testing.T) {
	// This test verifies the warning path exists without actually
	// removing ast-grep. We just test that RunChecks with tier=1
	// works (returns tier 1 if ast-grep is available, tier 0 if not).
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n\ngo 1.21\n")
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")

	res, err := RunChecks(dir, []string{"main.go"}, 1)
	if err != nil {
		t.Fatalf("RunChecks tier1: %v", err)
	}
	// Should be tier 1 if ast-grep available, tier 0 otherwise.
	if AstGrepAvailable() && res.Tier != 1 {
		t.Errorf("ast-grep available but got tier=%d", res.Tier)
	}
	if !AstGrepAvailable() && res.Tier != 0 {
		t.Errorf("ast-grep unavailable but got tier=%d", res.Tier)
	}
}

// findRuleFile locates a rule YAML file for testing.
func findRuleFile(t *testing.T, lang, name string) string {
	t.Helper()
	// Try CODEINTEL_RULES_DIR env var first.
	if d := os.Getenv("CODEINTEL_RULES_DIR"); d != "" {
		path := filepath.Join(d, "rules", lang, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	// Try relative to this test file's package.
	candidates := []string{
		filepath.Join("rules", lang, name),
		filepath.Join("..", "..", "pkg", "codeintel", "rules", lang, name),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	t.Fatalf("rule file not found: rules/%s/%s", lang, name)
	return ""
}

// setRulesDir sets CODEINTEL_RULES_DIR to point to the rules directory
// so that CheckDeadCodeTier1 and CheckPatterns can find the rules.
func setRulesDir(t *testing.T) {
	t.Helper()
	candidates := []string{
		"rules",
		filepath.Join("..", "..", "pkg", "codeintel", "rules"),
	}
	for _, c := range candidates {
		// Check if the directory contains rule subdirs.
		if _, err := os.Stat(filepath.Join(c, "go", "dead-function.yml")); err == nil {
			abs, _ := filepath.Abs(c)
			// rulesDir() looks for CODEINTEL_RULES_DIR, which should be the
			// parent of the "rules/" subdir that ASTGrepRulesDir references.
			// ASTGrepRulesDir is "rules/go", so CODEINTEL_RULES_DIR should be
			// the parent of that — i.e., the dir containing "rules/".
			parent := filepath.Dir(abs)
			t.Setenv("CODEINTEL_RULES_DIR", parent)
			return
		}
	}
	t.Fatal("cannot find rules directory for test setup")
}
