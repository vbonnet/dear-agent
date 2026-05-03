package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const helloWorkflow = `schema_version: "1"
name: hello
version: 0.1.0
nodes:
  - id: greet
    kind: bash
    bash:
      cmd: 'echo hello'
`

func TestRun_NoArgsShowsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, strings.NewReader(""), &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestRun_BadFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--nope"}, strings.NewReader(""), &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestRun_StartupFailureOnMissingWorkflow(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"/no/such/file.yaml"}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
}

func TestRun_REPLExitsCleanly(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "wf.yaml")
	if err := os.WriteFile(p, []byte(helloWorkflow), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{p}, strings.NewReader("exit\n"), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "workflow-dev:") {
		t.Errorf("missing banner:\n%s", stdout.String())
	}
}

func TestRun_RunVerb(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "wf.yaml")
	if err := os.WriteFile(p, []byte(helloWorkflow), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{p}, strings.NewReader("r\nexit\n"), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stdout.String(), "run (mock)") {
		t.Errorf("missing run output:\n%s", stdout.String())
	}
}
