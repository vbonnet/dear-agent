package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

func TestUpgradeDryRunDoesNotWrite(t *testing.T) {
	src := `name: legacy
version: 0.1.0
nodes:
  - id: r
    kind: ai
    ai:
      model: claude-opus-4-7
      prompt: dig
`
	p := writeTempFile(t, "legacy.yaml", src)

	var stdout, stderr bytes.Buffer
	code := run([]string{"upgrade", p}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	got, _ := os.ReadFile(p)
	if string(got) != src {
		t.Fatalf("dry-run modified the file:\n%s", string(got))
	}
	if !strings.Contains(stdout.String(), "changed") {
		t.Errorf("expected change summary in stdout:\n%s", stdout.String())
	}
}

func TestUpgradeWriteOverwritesInPlace(t *testing.T) {
	src := `name: legacy
version: 0.1.0
nodes:
  - id: r
    kind: ai
    ai:
      model: claude-opus-4-7
      prompt: dig
`
	p := writeTempFile(t, "legacy.yaml", src)

	var stdout, stderr bytes.Buffer
	code := run([]string{"upgrade", "--write", p}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	got, _ := os.ReadFile(p)
	if !strings.Contains(string(got), "role: research") {
		t.Errorf("expected role: research in rewritten file:\n%s", string(got))
	}
	if !strings.Contains(string(got), "schema_version: \"1\"") {
		t.Errorf("expected schema_version in rewritten file:\n%s", string(got))
	}
}

func TestUpgradeNoOpProducesNoOpLine(t *testing.T) {
	src := `schema_version: "1"
name: already
version: 0.1.0
nodes:
  - id: n1
    kind: ai
    ai:
      role: research
      prompt: hi
`
	p := writeTempFile(t, "already.yaml", src)
	var stdout, stderr bytes.Buffer
	code := run([]string{"upgrade", p}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "no-op") {
		t.Errorf("expected no-op prefix:\n%s", stdout.String())
	}
}

func TestFromWayfinderWritesWorkflowFile(t *testing.T) {
	session := `schema_version: "2.0"
project_name: "Test Project"
description: "demo"
roadmap:
  phases:
    - id: SETUP
      name: Planning
      status: completed
    - id: BUILD
      name: BUILD
      status: in-progress
`
	srcPath := writeTempFile(t, "session.yaml", session)
	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "workflow.yaml")

	var stdout, stderr bytes.Buffer
	code := run([]string{"from-wayfinder", "--out", outPath, srcPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read out: %v", err)
	}
	s := string(got)
	if !strings.Contains(s, "name: test-project") {
		t.Errorf("expected workflow name:\n%s", s)
	}
	if !strings.Contains(s, "id: setup") || !strings.Contains(s, "id: build") {
		t.Errorf("expected node ids:\n%s", s)
	}
}

func TestFromWayfinderStdoutWhenOutDash(t *testing.T) {
	session := `project_name: P
roadmap:
  phases:
    - id: ONE
      name: First
`
	srcPath := writeTempFile(t, "session.yaml", session)
	var stdout, stderr bytes.Buffer
	code := run([]string{"from-wayfinder", "--out", "-", srcPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "id: one") {
		t.Errorf("expected stdout to contain workflow YAML:\n%s", stdout.String())
	}
}

func TestUpgradeReportsErrorOnMissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"upgrade", "/nonexistent/path/to.yaml"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
}

func TestUnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"do-the-thing"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestNoArgsShowsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestHelpReturnsZero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
}
