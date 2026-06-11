package fsguard

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestFileRecorderAppendsJSONL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "nested", "violations.jsonl") // parent created on write
	r := NewFileRecorder(logPath)

	if err := r.Record(Violation{Tool: "Write", Path: "~/src/x", Reason: "blocked"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := r.Record(Violation{Tool: "Bash", Command: "rm ~/src/y", Reason: "blocked"}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	lines := readLines(t, logPath)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %v", len(lines), lines)
	}
	var v Violation
	if err := json.Unmarshal([]byte(lines[0]), &v); err != nil {
		t.Fatalf("line 0 not valid JSON: %v", err)
	}
	if v.Tool != "Write" || v.Path != "~/src/x" {
		t.Errorf("decoded %+v, want Tool=Write Path=~/src/x", v)
	}
	if v.Time == "" {
		t.Error("Record did not stamp a Time")
	}
}

func TestFileRecorderConcurrent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := NewFileRecorder(filepath.Join(dir, "v.jsonl"))

	const n = 50
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			_ = r.Record(Violation{Tool: "Write", Reason: "x"})
		})
	}
	wg.Wait()

	lines := readLines(t, filepath.Join(dir, "v.jsonl"))
	if len(lines) != n {
		t.Fatalf("got %d lines, want %d", len(lines), n)
	}
	for i, l := range lines {
		if !json.Valid([]byte(l)) {
			t.Fatalf("line %d corrupt (interleaved write): %q", i, l)
		}
	}
}

func TestNopRecorder(t *testing.T) {
	t.Parallel()
	if err := (NopRecorder{}).Record(Violation{}); err != nil {
		t.Fatalf("NopRecorder.Record: %v", err)
	}
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer func() { _ = f.Close() }()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if s := strings.TrimSpace(sc.Text()); s != "" {
			lines = append(lines, s)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan log: %v", err)
	}
	return lines
}
