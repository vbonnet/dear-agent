package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRun_BoundedDurationEmitsTrail exercises the full main.run path:
// flags parsed, mesh built, run for a tiny bounded duration, trail written
// to stdout. It is the "does the runner actually work end-to-end" smoke.
func TestRun_BoundedDurationEmitsTrail(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{
		"--interval", "5ms",
		"--duration", "60ms",
		"--disk", "0.95", // forces an Overseer escalation
		"--proposals", "1",
		"--tasks", "1",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run returned err = %v (stderr=%q)", err, stderr.String())
	}

	// Trail emerges as JSONL. Every line must parse, and we should see at
	// least one record from each of the three supervisor roles.
	seenRoles := map[string]bool{}
	for _, line := range bytes.Split(bytes.TrimSpace(stdout.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatalf("non-JSON line in stdout trail: %v (line=%q)", err, line)
		}
		if role, ok := rec["role"].(string); ok {
			seenRoles[role] = true
		}
	}
	for _, want := range []string{"meta-orchestrator", "orchestrator", "overseer"} {
		if !seenRoles[want] {
			t.Errorf("no trail records from role %q in stdout (seen=%v)", want, seenRoles)
		}
	}
}

func TestRun_WritesTrailToFile(t *testing.T) {
	dir := t.TempDir()
	trailFile := filepath.Join(dir, "trail.jsonl")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{
		"--interval", "5ms",
		"--duration", "40ms",
		"--trail", trailFile,
		"--proposals", "1",
		"--tasks", "1",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run: %v (stderr=%q)", err, stderr.String())
	}

	data, err := os.ReadFile(trailFile)
	if err != nil {
		t.Fatalf("read trail file: %v", err)
	}
	if !bytes.Contains(data, []byte("supervisor.")) {
		t.Errorf("trail file missing expected records:\n%s", data)
	}
}

func TestRun_FlagParseErrorSurfacedAsError(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--no-such-flag"}, stdout, stderr)
	if err == nil {
		t.Error("run returned nil for unknown flag")
	}
	if !strings.Contains(stderr.String(), "flag") {
		t.Errorf("stderr should mention the bad flag; got %q", stderr.String())
	}
}
