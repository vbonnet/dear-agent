package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFormatReport_NoFindings(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)
	report := formatReport(now, 24*time.Hour, 10, nil, nil)
	if !strings.Contains(report, "2026-06-11") {
		t.Errorf("report missing date: %s", report)
	}
	if !strings.Contains(report, "Spans scanned:** 10") {
		t.Errorf("report missing scan count: %s", report)
	}
	if !strings.Contains(report, "Findings:** 0") {
		t.Errorf("report missing 0 findings: %s", report)
	}
	if !strings.Contains(report, "_none_") {
		t.Errorf("report missing _none_ placeholder: %s", report)
	}
}

func TestFormatReport_WithFindings(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)
	errs := []finding{{TraceID: "aabbccddeeff0011", SpanID: "1122334455667788", Name: "op.fail", Msg: "oops"}}
	slows := []finding{{TraceID: "bbbbbbbbbbbbbbbb", SpanID: "cccccccccccccccc", Name: "op.slow", DurationMs: 9001}}
	report := formatReport(now, 24*time.Hour, 5, errs, slows)
	if !strings.Contains(report, "aabbccdd") {
		t.Errorf("report missing truncated trace ID: %s", report)
	}
	if !strings.Contains(report, "op.fail") {
		t.Errorf("report missing error span name: %s", report)
	}
	if !strings.Contains(report, "9001ms") {
		t.Errorf("report missing slow span duration: %s", report)
	}
}

func TestReadSpans_FiltersOldSpans(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	dir := t.TempDir()
	path := filepath.Join(dir, "spans.jsonl")

	spans := []span{
		{TraceID: "t1", SpanID: "s1", Name: "new", StartTime: now.Add(-1 * time.Hour), StatusCode: "Ok"},
		{TraceID: "t2", SpanID: "s2", Name: "old", StartTime: now.Add(-48 * time.Hour), StatusCode: "Error"},
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(f)
	for _, s := range spans {
		if err := enc.Encode(s); err != nil {
			t.Fatal(err)
		}
	}
	_ = f.Close()

	cutoff := now.Add(-24 * time.Hour)
	got, err := readSpans(path, cutoff)
	if err != nil {
		t.Fatalf("readSpans: %v", err)
	}
	if len(got) != 1 || got[0].TraceID != "t1" {
		t.Errorf("expected 1 in-window span (t1), got %v", got)
	}
}

func TestAppendToFile_CreatesAndAppends(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "RETRO_LOG.md")

	if err := appendToFile(path, "first\n"); err != nil {
		t.Fatalf("appendToFile(first): %v", err)
	}
	if err := appendToFile(path, "second\n"); err != nil {
		t.Fatalf("appendToFile(second): %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "first\nsecond\n" {
		t.Errorf("unexpected content: %q", string(data))
	}
}

func TestShort8(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"abcdefghijkl", "abcdefgh"},
		{"short", "short"},
		{"", ""},
	}
	for _, c := range cases {
		if got := short8(c.in); got != c.want {
			t.Errorf("short8(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
