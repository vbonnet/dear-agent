package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const twoTracesJSON = `[
  {"trace_id":"good","outcome":"success","spans":[{"pillar":"tool_call","name":"gen_ai.tool.call","attributes":{"gen_ai.tool.name":"ok"}}]},
  {"trace_id":"bad","outcome":"error","spans":[{"pillar":"tool_call","name":"gen_ai.tool.call","error_type":"boom","status_error":true,"attributes":{"gen_ai.tool.name":"writer"}}]}
]`

func TestRun_ExtractsProblematic(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "traces.json")
	if err := os.WriteFile(in, []byte(twoTracesJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "evals")

	var buf bytes.Buffer
	if err := run([]string{"-in", in, "-out", outDir}, &buf); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "generated=1") {
		t.Errorf("output missing generated=1: %q", got)
	}
	if _, err := os.Stat(filepath.Join(outDir, "cases", "bad.json")); err != nil {
		t.Errorf("expected case file for 'bad': %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "cases", "good.json")); err == nil {
		t.Error("clean trace 'good' should not produce a case")
	}
}

func TestRun_DryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "traces.jsonl")
	// JSONL form: one trace per line.
	lines := `{"trace_id":"bad","outcome":"error"}` + "\n"
	if err := os.WriteFile(in, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "evals")

	var buf bytes.Buffer
	if err := run([]string{"-in", in, "-out", outDir, "-dry-run"}, &buf); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(buf.String(), "dry-run") {
		t.Errorf("expected dry-run notice: %q", buf.String())
	}
	if _, err := os.Stat(filepath.Join(outDir, "cases")); err == nil {
		t.Error("dry-run wrote case files")
	}
}

func TestRun_MissingInput(t *testing.T) {
	var buf bytes.Buffer
	if err := run([]string{}, &buf); err == nil {
		t.Fatal("expected error for missing -in")
	}
}

func TestLoadTraces_Directory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.json"),
		[]byte(`{"trace_id":"a","outcome":"error"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.jsonl"),
		[]byte(`{"trace_id":"b","outcome":"error"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A non-trace file must be ignored.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore me"), 0o600); err != nil {
		t.Fatal(err)
	}
	traces, err := loadTraces(dir)
	if err != nil {
		t.Fatalf("loadTraces: %v", err)
	}
	if len(traces) != 2 {
		t.Fatalf("traces = %d, want 2", len(traces))
	}
}
