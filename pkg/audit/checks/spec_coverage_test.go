package checks

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

// writeFile is a one-line helper local to spec.coverage tests; the
// existing writeGo helper writes Go source specifically and we need
// to write SPEC.md too.
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// runSpecCoverage runs the check against a fresh temp tree with the
// given config and returns the Findings slice (sorted, as the check
// emits them).
func runSpecCoverage(t *testing.T, dir string, cfg map[string]any) []audit.Finding {
	t.Helper()
	res, err := SpecCoverageCheck{}.Run(context.Background(), audit.Env{
		RepoRoot:   dir,
		WorkingDir: dir,
		Config:     cfg,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != audit.StatusOK {
		t.Errorf("Status = %q, want ok", res.Status)
	}
	return res.Findings
}

// findingPaths extracts the Path field from each finding for ordered
// assertions; the check sorts by Path so the slice is deterministic.
func findingPaths(findings []audit.Finding) []string {
	out := make([]string, len(findings))
	for i, f := range findings {
		out[i] = f.Path
	}
	sort.Strings(out)
	return out
}

func TestSpecCoverageEmitsFindingForBareDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pkg/missing/missing.go", "package missing\n")

	findings := runSpecCoverage(t, dir, nil)
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1; got %+v", len(findings), findings)
	}
	f := findings[0]
	if f.Severity != audit.SeverityP3 {
		t.Errorf("severity = %q, want P3", f.Severity)
	}
	if f.Path != "pkg/missing/SPEC.md" {
		t.Errorf("path = %q, want pkg/missing/SPEC.md", f.Path)
	}
	if !strings.Contains(f.Title, "pkg/missing") {
		t.Errorf("title %q does not name the directory", f.Title)
	}
	if got, _ := f.Evidence["directory"].(string); got != "pkg/missing" {
		t.Errorf("evidence.directory = %q, want pkg/missing", got)
	}
}

func TestSpecCoverageSilentWhenSpecPresent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pkg/covered/covered.go", "package covered\n")
	writeFile(t, dir, "pkg/covered/SPEC.md", "# covered\n")

	if findings := runSpecCoverage(t, dir, nil); len(findings) != 0 {
		t.Errorf("expected 0 findings; got %+v", findings)
	}
}

func TestSpecCoverageSkipsTestOnlyDir(t *testing.T) {
	// A directory with only _test.go files is not a package worth
	// documenting separately — its parent's SPEC covers it.
	dir := t.TempDir()
	writeFile(t, dir, "pkg/fixtures/fixture_test.go", "package fixtures\n")

	if findings := runSpecCoverage(t, dir, nil); len(findings) != 0 {
		t.Errorf("expected 0 findings for test-only dir; got %+v", findings)
	}
}

func TestSpecCoverageSkipsHiddenAndUnderscoreDirs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pkg/_internal/x.go", "package internal\n")
	writeFile(t, dir, "pkg/.cache/y.go", "package cache\n")

	if findings := runSpecCoverage(t, dir, nil); len(findings) != 0 {
		t.Errorf("expected 0 findings for _/. prefix dirs; got %+v", findings)
	}
}

func TestSpecCoverageHonoursSkipConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pkg/vendor/lib.go", "package lib\n")
	writeFile(t, dir, "pkg/keepme/keep.go", "package keepme\n")

	findings := runSpecCoverage(t, dir, nil)
	paths := findingPaths(findings)
	want := []string{"pkg/keepme/SPEC.md"}
	if !equalStrings(paths, want) {
		t.Errorf("paths = %v, want %v", paths, want)
	}
}

func TestSpecCoverageHonoursRootsConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pkg/a/a.go", "package a\n")
	writeFile(t, dir, "internal/b/b.go", "package b\n")
	writeFile(t, dir, "tools/c/c.go", "package c\n")
	writeFile(t, dir, "cmd/d/d.go", "package d\n")
	writeFile(t, dir, "third_party/e/e.go", "package e\n")

	// Default roots include pkg/internal/tools/cmd but not third_party.
	defaults := runSpecCoverage(t, dir, nil)
	if len(defaults) != 4 {
		t.Errorf("default roots: findings = %d, want 4; got %v", len(defaults), findingPaths(defaults))
	}

	// Custom roots restrict to a single tree.
	custom := runSpecCoverage(t, dir, map[string]any{
		"roots": []any{"third_party"},
	})
	if len(custom) != 1 || custom[0].Path != "third_party/e/SPEC.md" {
		t.Errorf("custom roots: unexpected findings %+v", custom)
	}
}

func TestSpecCoverageMissingRootIsSilent(t *testing.T) {
	dir := t.TempDir()
	// No pkg/, no internal/, no tools/, no cmd/. The check should not
	// error and should produce no findings.
	findings := runSpecCoverage(t, dir, nil)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings against empty repo; got %+v", findings)
	}
}

func TestSpecCoverageDoesNotRecurse(t *testing.T) {
	// A nested package (pkg/a/b/) without its own SPEC.md should NOT
	// produce a finding — only direct children of the configured root
	// are inspected. This matches the repo convention that sub-packages
	// inherit documentation from the top-level area.
	dir := t.TempDir()
	writeFile(t, dir, "pkg/a/SPEC.md", "# a\n")
	writeFile(t, dir, "pkg/a/a.go", "package a\n")
	writeFile(t, dir, "pkg/a/b/b.go", "package b\n")

	if findings := runSpecCoverage(t, dir, nil); len(findings) != 0 {
		t.Errorf("expected 0 findings (sub-packages inherit); got %+v", findings)
	}
}

func TestSpecCoverageStableFingerprint(t *testing.T) {
	// The same defect across two runs must produce the same
	// Fingerprint so the store de-dupes. Two runs of the same fixture
	// must agree.
	dir := t.TempDir()
	writeFile(t, dir, "pkg/x/x.go", "package x\n")

	first := runSpecCoverage(t, dir, nil)
	second := runSpecCoverage(t, dir, nil)
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("expected 1 finding each, got %d and %d", len(first), len(second))
	}
	if first[0].Fingerprint != second[0].Fingerprint {
		t.Errorf("fingerprint differs across runs: %q vs %q",
			first[0].Fingerprint, second[0].Fingerprint)
	}
}

func TestSpecCoverageMetaIsValid(t *testing.T) {
	if err := (SpecCoverageCheck{}).Meta().Validate(); err != nil {
		t.Errorf("Meta.Validate: %v", err)
	}
}

func TestSpecCoverageRegisteredInDefault(t *testing.T) {
	if _, ok := audit.Default.Lookup("spec.coverage"); !ok {
		t.Error("spec.coverage not registered in audit.Default")
	}
}

func equalStrings(a, b []string) bool {
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
