package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
	"github.com/vbonnet/dear-agent/spec-governance/earslint"
)

func writeSpec(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRun_PassingSpec(t *testing.T) {
	dir := t.TempDir()
	p := writeSpec(t, dir, "SPEC.md", "The system shall log requests.\n")
	var out, errBuf bytes.Buffer
	code := run([]string{p}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("want exit 0, got %d (stderr=%s)", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "1 valid requirements") {
		t.Errorf("unexpected output: %s", out.String())
	}
}

func TestRun_ZeroRequirementsFails(t *testing.T) {
	dir := t.TempDir()
	p := writeSpec(t, dir, "SPEC.md", "# Just prose, no requirements.\n")
	var out, errBuf bytes.Buffer
	code := run([]string{p}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
}

func TestRun_StrictVsNonStrict(t *testing.T) {
	dir := t.TempDir()
	p := writeSpec(t, dir, "SPEC.md",
		"The system shall log requests.\nEventually the thing shall happen somehow.\n")

	var out, errBuf bytes.Buffer
	if code := run([]string{p}, &out, &errBuf); code != 0 {
		t.Errorf("non-strict with a valid requirement should pass, got exit %d", code)
	}

	out.Reset()
	errBuf.Reset()
	if code := run([]string{"--strict", p}, &out, &errBuf); code != 1 {
		t.Errorf("strict should fail on non-conforming requirement, got exit %d", code)
	}
}

func TestRun_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	p := writeSpec(t, dir, "SPEC.md", "The system shall log requests.\n")
	var out, errBuf bytes.Buffer
	code := run([]string{"--json", p}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("want exit 0, got %d", code)
	}
	var results []earslint.Result
	if err := json.Unmarshal(out.Bytes(), &results); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out.String())
	}
	if len(results) != 1 || results[0].ValidRequirements != 1 {
		t.Errorf("unexpected JSON results: %+v", results)
	}
}

func TestRun_DirectoryRecursion(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSpec(t, dir, "SPEC.md", "The system shall start.\n")
	writeSpec(t, sub, "SPEC.md", "The system shall stop.\n")

	var out, errBuf bytes.Buffer
	code := run([]string{dir}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("want exit 0, got %d (stderr=%s)", code, errBuf.String())
	}
	if c := strings.Count(out.String(), "valid requirements"); c != 2 {
		t.Errorf("expected 2 SPEC.md files linted, got %d\n%s", c, out.String())
	}
}

func TestExpandPathsHonorsRepositoryIgnoreAndGeneratedPolicy(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cmd := gittest.Command(t, root, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"visible", "ignored", "dist"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		writeSpec(t, filepath.Join(root, dir), "SPEC.md", "The system shall be deterministic.\n")
	}

	files, err := expandPaths([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(root, "visible", "SPEC.md")}
	if !slices.Equal(files, want) {
		t.Fatalf("expanded files = %v, want %v", files, want)
	}
}

func TestRun_ConfigOverride(t *testing.T) {
	dir := t.TempDir()
	cfg := writeSpec(t, dir, ".earslint.yml",
		"requirement_keyword: must\npatterns:\n  - name: must\n    regex: '(?i)^the\\s+.+\\s+must\\s+.+'\n")
	p := writeSpec(t, dir, "SPEC.md", "The system must persist data.\n")
	var out, errBuf bytes.Buffer
	code := run([]string{"--config", cfg, p}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("want exit 0, got %d (stderr=%s)", code, errBuf.String())
	}
}

func TestRun_NoFilesFound(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{filepath.Join(t.TempDir(), "does-not-exist.md")}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("want exit 2 for missing path, got %d", code)
	}
}
