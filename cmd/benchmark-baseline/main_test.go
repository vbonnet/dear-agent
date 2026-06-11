package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_WritesBaselineJSON(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "baseline.json")

	code := run([]string{
		"--output", out,
		"--jaeger-url", "http://localhost:0", // unreachable; Jaeger query must fail gracefully
	})
	if code != 0 {
		t.Fatalf("run exited %d", code)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	var b Baseline
	if err := json.Unmarshal(data, &b); err != nil {
		t.Fatalf("parse output: %v", err)
	}

	if b.Version != "1" {
		t.Errorf("version=%q, want 1", b.Version)
	}
	if b.Benchmark == nil {
		t.Fatal("benchmark is nil")
	}
	if b.Benchmark.Summary.Total == 0 {
		t.Error("benchmark.summary.total is 0; expected fixture tasks")
	}
	// Stub executor marks every task as errored (not solved), so solve_rate == 0.
	if b.Benchmark.Summary.SolveRate != 0 {
		t.Errorf("solve_rate=%f, want 0 for stub executor", b.Benchmark.Summary.SolveRate)
	}
	if b.Operational == nil {
		t.Fatal("operational is nil")
	}
	// With an unreachable Jaeger, the note should mention unavailability.
	found := false
	for _, n := range b.Operational.Notes {
		if strings.Contains(n, "no traces found") || strings.Contains(n, "not yet wired") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a note about missing traces; got notes=%v", b.Operational.Notes)
	}
}

func TestRun_StdoutMode(t *testing.T) {
	// Redirect stdout so we can parse the JSON written to -.
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	code := run([]string{
		"--output", "-",
		"--jaeger-url", "http://localhost:0",
	})

	w.Close()
	os.Stdout = old

	if code != 0 {
		t.Fatalf("run exited %d", code)
	}

	var buf strings.Builder
	tmp := make([]byte, 4096)
	for {
		n, err2 := r.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if err2 != nil {
			break
		}
	}

	var b Baseline
	if err := json.Unmarshal([]byte(buf.String()), &b); err != nil {
		t.Fatalf("parse stdout JSON: %v\noutput: %s", err, buf.String())
	}
	if b.Benchmark == nil {
		t.Fatal("benchmark is nil in stdout output")
	}
}

func TestPrintSummary_NonemptyBaseline(t *testing.T) {
	b := &Baseline{
		Version: "1",
		GitSHA:  "abc123",
		Notes:   []string{"test note"},
	}
	// Should not panic.
	printSummary(os.Stderr, b)
}
