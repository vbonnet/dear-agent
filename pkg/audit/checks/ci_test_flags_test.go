package checks

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

// writeWorkflow creates a minimal GitHub Actions YAML file at
// .github/workflows/<name>.yml inside dir.
func writeWorkflow(t *testing.T, dir, name, content string) {
	t.Helper()
	wfDir := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(wfDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wfDir, name+".yml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// writeTestFile creates a *_test.go file at <root>/<pkg>/<name>_test.go.
func writeTestFile(t *testing.T, root, pkg, name, content string) {
	t.Helper()
	pkgDir := filepath.Join(root, pkg)
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, name+"_test.go"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// newCITestEnv builds an audit.Env with the temp dir as WorkingDir.
func newCITestEnv(t *testing.T) (audit.Env, string) {
	t.Helper()
	dir := t.TempDir()
	return audit.Env{WorkingDir: dir, RepoRoot: dir}, dir
}

// TestCITestFlags_NoWorkflowDir_Passes checks that a repo without a
// .github/workflows directory does not emit findings (not a GitHub repo).
func TestCITestFlags_NoWorkflowDir_Passes(t *testing.T) {
	env, _ := newCITestEnv(t)
	result, err := CITestFlagsCheck{}.Run(context.Background(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != audit.StatusOK {
		t.Errorf("status=%v, want OK", result.Status)
	}
	if len(result.Findings) != 0 {
		t.Errorf("findings=%d, want 0 (no workflow dir)", len(result.Findings))
	}
}

// TestCITestFlags_CIPassesShort_NoFindings checks that a repo whose CI
// passes -short does not emit findings even if test files call testing.Short().
func TestCITestFlags_CIPassesShort_NoFindings(t *testing.T) {
	env, dir := newCITestEnv(t)
	writeWorkflow(t, dir, "ci", "jobs:\n  test:\n    steps:\n      - run: go test -short -race ./...\n")
	writeTestFile(t, dir, "pkg/foo", "foo",
		"package foo\nimport \"testing\"\nfunc TestFoo(t *testing.T) {\n  if testing.Short() { t.Skip() }\n}\n")

	// Override roots to only look in pkg/foo.
	env.Config = map[string]any{"roots": []string{"pkg"}}
	result, err := CITestFlagsCheck{}.Run(context.Background(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Findings) != 0 {
		t.Errorf("findings=%d, want 0 (CI passes -short)", len(result.Findings))
	}
}

// TestCITestFlags_CINoShort_TestFilesHaveShort_EmitsFinding verifies the
// primary detection path: CI omits -short and test files use testing.Short().
func TestCITestFlags_CINoShort_TestFilesHaveShort_EmitsFinding(t *testing.T) {
	env, dir := newCITestEnv(t)
	writeWorkflow(t, dir, "ci", "jobs:\n  test:\n    steps:\n      - run: go test -race ./...\n")
	writeTestFile(t, dir, "pkg/foo", "foo",
		"package foo\nimport \"testing\"\nfunc TestFoo(t *testing.T) {\n  if testing.Short() { t.Skip(\"skip in short mode\") }\n}\n")

	env.Config = map[string]any{"roots": []string{"pkg"}}
	result, err := CITestFlagsCheck{}.Run(context.Background(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("findings=%d, want 1", len(result.Findings))
	}
	f := result.Findings[0]
	if f.Severity != audit.SeverityP3 {
		t.Errorf("severity=%v, want P3", f.Severity)
	}
	if f.Fingerprint == "" {
		t.Error("fingerprint must not be empty")
	}
	// Evidence must contain affected files.
	files, _ := f.Evidence["affected_files"].([]string)
	if len(files) != 1 {
		t.Errorf("affected_files len=%d, want 1", len(files))
	}
}

// TestCITestFlags_CINoShort_NoTestFilesWithShort_NoFindings checks the
// "nothing to report" path: CI omits -short but no test file calls
// testing.Short(), so there is no skew to report.
func TestCITestFlags_CINoShort_NoTestFilesWithShort_NoFindings(t *testing.T) {
	env, dir := newCITestEnv(t)
	writeWorkflow(t, dir, "ci", "jobs:\n  test:\n    steps:\n      - run: go test -race ./...\n")
	writeTestFile(t, dir, "pkg/foo", "foo",
		"package foo\nimport \"testing\"\nfunc TestFoo(t *testing.T) { _ = 1 }\n")

	env.Config = map[string]any{"roots": []string{"pkg"}}
	result, err := CITestFlagsCheck{}.Run(context.Background(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Findings) != 0 {
		t.Errorf("findings=%d, want 0 (no testing.Short() usage)", len(result.Findings))
	}
}

// TestCITestFlags_MultipleAffectedFiles checks that the finding aggregates
// all affected files into a single entry.
func TestCITestFlags_MultipleAffectedFiles(t *testing.T) {
	env, dir := newCITestEnv(t)
	writeWorkflow(t, dir, "ci", "jobs:\n  test:\n    steps:\n      - run: go test ./...\n")
	const shortGuard = "package foo\nimport \"testing\"\nfunc TestX(t *testing.T) {\n  if testing.Short() { t.Skip() }\n}\n"
	writeTestFile(t, dir, "pkg/a", "a", shortGuard)
	writeTestFile(t, dir, "pkg/b", "b", shortGuard)
	writeTestFile(t, dir, "pkg/c", "c", shortGuard)

	env.Config = map[string]any{"roots": []string{"pkg"}}
	result, err := CITestFlagsCheck{}.Run(context.Background(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("findings=%d, want 1 (aggregated)", len(result.Findings))
	}
	files, _ := result.Findings[0].Evidence["affected_files"].([]string)
	if len(files) != 3 {
		t.Errorf("affected_files=%d, want 3", len(files))
	}
	// Verify count field matches.
	count, _ := result.Findings[0].Evidence["affected_count"].(int)
	if count != 3 {
		t.Errorf("affected_count=%d, want 3", count)
	}
}

// TestCITestFlagsCheck_Meta verifies the check's identity fields.
func TestCITestFlagsCheck_Meta(t *testing.T) {
	m := CITestFlagsCheck{}.Meta()
	if m.ID != "ci.test_flags" {
		t.Errorf("ID=%q, want ci.test_flags", m.ID)
	}
	if m.Cadence != audit.CadenceWeekly {
		t.Errorf("Cadence=%v, want Weekly", m.Cadence)
	}
	if m.SeverityCeiling != audit.SeverityP3 {
		t.Errorf("SeverityCeiling=%v, want P3", m.SeverityCeiling)
	}
}
