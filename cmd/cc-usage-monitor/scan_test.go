package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClassifySessionType(t *testing.T) {
	cases := []struct {
		name string
		want SessionType
	}{
		{"vroom-orchestrator", SessionSupervisor},
		{"vroom-meta-orchestrator", SessionSupervisor},
		{"vroom-overseer", SessionSupervisor},
		{"some-supervisor-thing", SessionSupervisor},
		{"vroom-meta-o", SessionSupervisor},
		// supervisor wins even when "worker" is also present
		{"vroom-orchestrator-worker-1", SessionSupervisor},
		{"ce-bkxa-worker", SessionWorker},
		{"worker-ce-11fi", SessionWorker},
		{"/Users/x/.agm/sandboxes/abc-upper", SessionWorker},
		{"random-project", SessionUnknown},
		{"", SessionUnknown},
	}
	for _, c := range cases {
		if got := ClassifySessionType(c.name); got != c.want {
			t.Errorf("ClassifySessionType(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestClassifyBest_CwdWinsOverHint(t *testing.T) {
	if got := classifyBest("/home/vroom-overseer", "random"); got != SessionSupervisor {
		t.Errorf("cwd should classify supervisor, got %q", got)
	}
	// cwd unknown -> fall back to dir hint
	if got := classifyBest("/tmp/nothing", "ce-x-worker"); got != SessionWorker {
		t.Errorf("hint fallback should classify worker, got %q", got)
	}
}

func TestParseLine_AssistantUsageComputesCost(t *testing.T) {
	line := []byte(`{"type":"assistant","timestamp":"2026-06-21T10:00:00Z","cwd":"/x/worker-ce-1","message":{"model":"claude-opus-4-8","usage":{"input_tokens":1000000,"output_tokens":1000000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`)
	e, ok := parseLine(line, "hint")
	if !ok {
		t.Fatal("expected ok")
	}
	if e.Type != SessionWorker {
		t.Errorf("type = %q, want worker", e.Type)
	}
	if e.Input != 1_000_000 || e.Output != 1_000_000 {
		t.Errorf("tokens = %d/%d", e.Input, e.Output)
	}
	// Opus 4.8: $15/1M input + $75/1M output = $90 for 1M each.
	if got := round2(e.CostUSD); got != 90.00 {
		t.Errorf("cost = %v, want 90.00", got)
	}
	if e.Timestamp.IsZero() {
		t.Error("timestamp should parse")
	}
}

func TestParseLine_HonoursExplicitCostUSD(t *testing.T) {
	line := []byte(`{"type":"assistant","costUSD":1.23,"message":{"model":"claude-opus-4-8","usage":{"input_tokens":10,"output_tokens":10}}}`)
	e, ok := parseLine(line, "hint")
	if !ok {
		t.Fatal("expected ok")
	}
	if e.CostUSD != 1.23 {
		t.Errorf("cost = %v, want 1.23 (explicit costUSD)", e.CostUSD)
	}
}

func TestParseLine_SkipsNonUsageLines(t *testing.T) {
	for _, line := range []string{
		``,
		`   `,
		`not json`,
		`{"type":"user","message":{"role":"user","content":"hi"}}`,
		`{"type":"summary","summary":"x"}`,
	} {
		if _, ok := parseLine([]byte(line), "hint"); ok {
			t.Errorf("expected skip for %q", line)
		}
	}
}

func TestParseLine_UnknownModelZeroCost(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"model":"mystery-model","usage":{"input_tokens":100,"output_tokens":100}}}`)
	e, ok := parseLine(line, "hint")
	if !ok {
		t.Fatal("expected ok")
	}
	if e.CostUSD != 0 {
		t.Errorf("unknown model cost = %v, want 0", e.CostUSD)
	}
}

func TestScanReader_AggregatesByTypeAndModel(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"assistant","cwd":"/x/worker-1","message":{"model":"claude-opus-4-8","usage":{"input_tokens":1000000,"output_tokens":0}}}`,
		`{"type":"assistant","cwd":"/x/vroom-orchestrator","message":{"model":"claude-opus-4-8","usage":{"input_tokens":0,"output_tokens":1000000}}}`,
		`{"type":"user","message":{"content":"skip"}}`,
	}, "\n")

	report := newReport()
	var entries []Entry
	if err := ScanReader(strings.NewReader(input), "hint", report, &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if report.Total.Messages != 2 {
		t.Errorf("messages = %d, want 2", report.Total.Messages)
	}
	if w := report.ByType[SessionWorker]; w == nil || w.Input != 1_000_000 {
		t.Errorf("worker input wrong: %+v", w)
	}
	if s := report.ByType[SessionSupervisor]; s == nil || s.Output != 1_000_000 {
		t.Errorf("supervisor output wrong: %+v", s)
	}
	// $15 (worker input) + $75 (supervisor output) = $90.
	if got := round2(report.Total.CostUSD); got != 90.00 {
		t.Errorf("total cost = %v, want 90.00", got)
	}
	if m := report.ByModel["claude-opus-4-8"]; m == nil || m.Messages != 2 {
		t.Errorf("by-model wrong: %+v", m)
	}
}

func TestScanDir_WalksJSONLAndUsesDirHint(t *testing.T) {
	root := t.TempDir()
	// A project dir named like a worker session; lines lack cwd so the dir
	// name must drive classification.
	proj := filepath.Join(root, "ce-bkxa-worker")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"assistant","message":{"model":"claude-opus-4-8","usage":{"input_tokens":100,"output_tokens":50}}}` + "\n"
	if err := os.WriteFile(filepath.Join(proj, "sess.jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	// A non-jsonl file must be ignored.
	if err := os.WriteFile(filepath.Join(proj, "notes.txt"), []byte("ignore"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, entries, err := ScanDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if report.Files != 1 {
		t.Errorf("files = %d, want 1", report.Files)
	}
	if len(entries) != 1 {
		t.Errorf("entries = %d, want 1", len(entries))
	}
	if w := report.ByType[SessionWorker]; w == nil || w.Input != 100 {
		t.Errorf("dir-hint classification failed: %+v", w)
	}
}

func TestProjectedDailyCost(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	entries := []Entry{
		{Timestamp: now.Add(-30 * time.Minute), CostUSD: 1.0}, // in 1h window
		{Timestamp: now.Add(-90 * time.Minute), CostUSD: 5.0}, // outside 1h window
		{Timestamp: time.Time{}, CostUSD: 99.0},               // no timestamp, ignored
	}
	// $1 in the trailing hour -> $24/day.
	if got := round2(ProjectedDailyCost(entries, now, time.Hour)); got != 24.00 {
		t.Errorf("projected = %v, want 24.00", got)
	}
	// 2h window captures $6 over 2h -> $3/h -> $72/day.
	if got := round2(ProjectedDailyCost(entries, now, 2*time.Hour)); got != 72.00 {
		t.Errorf("projected(2h) = %v, want 72.00", got)
	}
	if got := ProjectedDailyCost(entries, now, 0); got != 0 {
		t.Errorf("zero window should be 0, got %v", got)
	}
}

func TestRun_AlertFiresAndWritesTrail(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "projects", "worker-ce-x")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	// 10M output tokens of Opus in the last 30m = $750 -> way over $50/day.
	line := `{"type":"assistant","timestamp":"2026-06-21T11:45:00Z","message":{"model":"claude-opus-4-8","usage":{"input_tokens":0,"output_tokens":10000000}}}` + "\n"
	if err := os.WriteFile(filepath.Join(root, "s.jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	trail := filepath.Join(dir, "trail.jsonl")

	var out, errb bytes.Buffer
	code := run([]string{
		"--root", filepath.Join(dir, "projects"),
		"--trail", trail,
		"--no-emit",
		"--json",
	}, &out, &errb, now)

	if code != exitAlert {
		t.Fatalf("exit = %d, want %d (alert). stderr=%s", code, exitAlert, errb.String())
	}
	data, err := os.ReadFile(trail)
	if err != nil {
		t.Fatalf("trail not written: %v", err)
	}
	if !strings.Contains(string(data), `"quota.burn.alert"`) {
		t.Errorf("trail missing alert kind: %s", data)
	}
	if !strings.Contains(out.String(), `"alerted": true`) {
		t.Errorf("json output missing alerted flag: %s", out.String())
	}
}

func TestRun_NoAlertBelowThreshold(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "projects", "worker-ce-x")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	line := `{"type":"assistant","timestamp":"2026-06-21T11:45:00Z","message":{"model":"claude-opus-4-8","usage":{"input_tokens":10,"output_tokens":10}}}` + "\n"
	if err := os.WriteFile(filepath.Join(root, "s.jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	trail := filepath.Join(dir, "trail.jsonl")

	var out, errb bytes.Buffer
	code := run([]string{
		"--root", filepath.Join(dir, "projects"),
		"--trail", trail,
		"--no-emit",
	}, &out, &errb, now)

	if code != exitOK {
		t.Fatalf("exit = %d, want %d (ok). stderr=%s", code, exitOK, errb.String())
	}
	if _, err := os.Stat(trail); !os.IsNotExist(err) {
		t.Errorf("trail should not be written below threshold")
	}
}
