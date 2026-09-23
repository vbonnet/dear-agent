package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/retrolint"
)

// RLINT-08: When all evaluated retrospectives satisfy guard requirements, the CLI shall exit 0.
func TestRLINT08_Exit0OnAllPassing(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	retrosDir := filepath.Join(repoRoot, "retrospectives")
	if err := os.MkdirAll(retrosDir, 0750); err != nil {
		t.Fatal(err)
	}

	retroPath := filepath.Join(retrosDir, "2026-09-05-valid.md")
	content := `# Valid Retro
Date: 2026-09-05

## Guards
- launchd: com.dear-agent.recovery-loop
- deferred: ce-8v9d3 (Valid rationale)
`
	if err := os.WriteFile(retroPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	args := []string{
		"--repo", repoRoot,
		"--retros-dir", retrosDir,
	}

	exitCode := run(args, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "PASS") {
		t.Fatalf("expected PASS in stdout, got: %s", stdout.String())
	}
}

// RLINT-09: When at least one evaluated retrospective fails guard requirements or names missing artifacts, the CLI shall exit 1.
func TestRLINT09_Exit1OnFailingRetrospective(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	retrosDir := filepath.Join(repoRoot, "retrospectives")
	if err := os.MkdirAll(retrosDir, 0750); err != nil {
		t.Fatal(err)
	}

	retroPath := filepath.Join(retrosDir, "2026-09-05-failing.md")
	content := `# Failing Retro
Date: 2026-09-05

## Guards
- test: non/existent/file_test.go
`
	if err := os.WriteFile(retroPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	args := []string{
		"--repo", repoRoot,
		"--retros-dir", retrosDir,
	}

	exitCode := run(args, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("expected exit 1, got %d (stdout: %s, stderr: %s)", exitCode, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "FAIL") {
		t.Fatalf("expected FAIL in stdout, got: %s", stdout.String())
	}
}

// RLINT-10: When configuration loading fails, invalid flags are supplied, or target directories cannot be read, the CLI shall exit 2 with a usage error.
func TestRLINT10_Exit2OnUsageOrConfigError(t *testing.T) {
	t.Parallel()

	// 1. Unknown flag
	var stdout, stderr bytes.Buffer
	code := run([]string{"--nonexistent-flag"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected exit 2 for bad flag, got %d", code)
	}

	// 2. Unreadable / non-existent retrospectives directory
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"--retros-dir", "/non/existent/path/for/retros"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected exit 2 for non-existent retros directory, got %d", code)
	}

	// 3. Invalid absence-lookback duration
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"--absence-lookback", "invalid-duration"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected exit 2 for invalid duration, got %d", code)
	}
}

// RLINT-11: When JSON mode is enabled, the CLI shall output structured results on stdout.
func TestRLINT11_JSONOutputMode(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	retrosDir := filepath.Join(repoRoot, "retrospectives")
	if err := os.MkdirAll(retrosDir, 0750); err != nil {
		t.Fatal(err)
	}

	retroPath := filepath.Join(retrosDir, "2026-09-05-valid.md")
	content := `# Valid Retro
Date: 2026-09-05

## Guards
- launchd: com.dear-agent.recovery-loop
`
	if err := os.WriteFile(retroPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	args := []string{
		"--repo", repoRoot,
		"--retros-dir", retrosDir,
		"--json",
	}

	exitCode := run(args, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", exitCode, stderr.String())
	}

	var rep retrolint.Report
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("stdout did not contain valid JSON report: %v\nOutput was: %s", err, stdout.String())
	}

	if rep.Status != retrolint.StatusPass {
		t.Fatalf("expected JSON report status PASS, got %s", rep.Status)
	}
	if rep.Evaluated != 1 {
		t.Fatalf("expected 1 evaluated, got %d", rep.Evaluated)
	}
	if len(rep.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(rep.Results))
	}
}
