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
	cases := map[string]string{
		"ce-bkxa":                  SessionWorker,
		"":                         SessionWorker,
		"feature-work":             SessionWorker,
		"vroom-supervisor":         SessionSupervisor,
		"Overseer-tick":            SessionSupervisor,
		"meta-orchestrator":        SessionSupervisor,
		"the-orchestrator-session": SessionSupervisor,
	}
	for name, want := range cases {
		if got := classifySessionType(name); got != want {
			t.Errorf("classifySessionType(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestPricingEstimateCost(t *testing.T) {
	p := Pricing{InputPerMTok: 15, OutputPerMTok: 75}
	// 1M input @ $15 + 1M output @ $75 = $90.
	if got := p.estimateCost(1_000_000, 1_000_000); got != 90 {
		t.Fatalf("estimateCost = %v, want 90", got)
	}
	if got := p.estimateCost(0, 0); got != 0 {
		t.Fatalf("estimateCost(0,0) = %v, want 0", got)
	}
}

func TestParseLine(t *testing.T) {
	p := Pricing{InputPerMTok: 15, OutputPerMTok: 75}

	t.Run("nested message.usage, no costUSD → estimated", func(t *testing.T) {
		line := []byte(`{"type":"assistant","timestamp":"2026-06-21T10:00:00Z","slug":"vroom-supervisor","message":{"usage":{"input_tokens":1000000,"output_tokens":0}}}`)
		rec, ok := parseLine(line, "fallback", p)
		if !ok {
			t.Fatal("expected ok")
		}
		if rec.SessionType != SessionSupervisor {
			t.Errorf("SessionType = %q, want supervisor", rec.SessionType)
		}
		if rec.CostUSD != 15 {
			t.Errorf("CostUSD = %v, want 15 (estimated)", rec.CostUSD)
		}
		if rec.InputTokens != 1_000_000 {
			t.Errorf("InputTokens = %d", rec.InputTokens)
		}
	})

	t.Run("explicit costUSD wins over estimate", func(t *testing.T) {
		line := []byte(`{"type":"assistant","costUSD":3.5,"message":{"usage":{"input_tokens":1000000,"output_tokens":0}}}`)
		rec, ok := parseLine(line, "fallback", p)
		if !ok {
			t.Fatal("expected ok")
		}
		if rec.CostUSD != 3.5 {
			t.Errorf("CostUSD = %v, want 3.5 (explicit)", rec.CostUSD)
		}
	})

	t.Run("legacy top-level usage", func(t *testing.T) {
		line := []byte(`{"type":"assistant","usage":{"input_tokens":0,"output_tokens":1000000}}`)
		rec, ok := parseLine(line, "worker-x", p)
		if !ok {
			t.Fatal("expected ok")
		}
		if rec.OutputTokens != 1_000_000 || rec.CostUSD != 75 {
			t.Errorf("got out=%d cost=%v, want 1000000 / 75", rec.OutputTokens, rec.CostUSD)
		}
	})

	t.Run("slug empty falls back to name", func(t *testing.T) {
		line := []byte(`{"type":"assistant","message":{"usage":{"input_tokens":5,"output_tokens":5}}}`)
		rec, ok := parseLine(line, "supervisor-fallback", p)
		if !ok {
			t.Fatal("expected ok")
		}
		if rec.SessionType != SessionSupervisor {
			t.Errorf("SessionType = %q, want supervisor (from fallback name)", rec.SessionType)
		}
	})

	t.Run("no usage → skipped", func(t *testing.T) {
		if _, ok := parseLine([]byte(`{"type":"user","message":{"role":"user"}}`), "x", p); ok {
			t.Error("expected user turn to be skipped")
		}
	})

	t.Run("malformed JSON → skipped", func(t *testing.T) {
		if _, ok := parseLine([]byte(`{not json`), "x", p); ok {
			t.Error("expected malformed line to be skipped")
		}
	})
}

func TestBurnProjection(t *testing.T) {
	// $1 in 1h → $24 projected.
	if got := burnProjection(1.0, time.Hour); got != 24 {
		t.Errorf("burnProjection(1,1h) = %v, want 24", got)
	}
	// Zero/negative window must never fabricate spend.
	if got := burnProjection(5.0, 0); got != 0 {
		t.Errorf("burnProjection(5,0) = %v, want 0", got)
	}
}

func TestAggregate(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	window := time.Hour
	recs := []parsedRecord{
		// Inside window: $10 supervisor.
		{SessionType: SessionSupervisor, Timestamp: now.Add(-30 * time.Minute), InputTokens: 100, OutputTokens: 200, CostUSD: 10},
		// Outside window: $5 worker (counts toward totals, not burn).
		{SessionType: SessionWorker, Timestamp: now.Add(-3 * time.Hour), InputTokens: 50, OutputTokens: 60, CostUSD: 5},
		// Inside window: $2 worker.
		{SessionType: SessionWorker, Timestamp: now.Add(-5 * time.Minute), InputTokens: 1, OutputTokens: 2, CostUSD: 2},
	}
	res := aggregate(recs, now, window)

	if res.RecordsCounted != 3 {
		t.Errorf("RecordsCounted = %d, want 3", res.RecordsCounted)
	}
	if res.Total.CostUSD != 17 {
		t.Errorf("Total.CostUSD = %v, want 17", res.Total.CostUSD)
	}
	if res.ByType[SessionSupervisor].CostUSD != 10 {
		t.Errorf("supervisor cost = %v, want 10", res.ByType[SessionSupervisor].CostUSD)
	}
	if res.ByType[SessionWorker].CostUSD != 7 {
		t.Errorf("worker cost = %v, want 7", res.ByType[SessionWorker].CostUSD)
	}
	// Window cost = $10 + $2 = $12 → projected $288/24h.
	if res.WindowCostUSD != 12 {
		t.Errorf("WindowCostUSD = %v, want 12", res.WindowCostUSD)
	}
	if res.Projected24hUSD != 288 {
		t.Errorf("Projected24hUSD = %v, want 288", res.Projected24hUSD)
	}
}

func TestScanProjects(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "-Users-x-worker-session")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	content := strings.Join([]string{
		`{"type":"assistant","message":{"usage":{"input_tokens":10,"output_tokens":20}}}`,
		`{"type":"user","message":{"role":"user"}}`,
		`{"type":"assistant","message":{"usage":{"input_tokens":5,"output_tokens":5}}}`,
	}, "\n")
	if err := os.WriteFile(filepath.Join(proj, "sess.jsonl"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	// A non-jsonl file must be ignored.
	if err := os.WriteFile(filepath.Join(proj, "notes.txt"), []byte("ignore me"), 0o600); err != nil {
		t.Fatal(err)
	}

	recs, files, err := ScanProjects(dir, DefaultPricing)
	if err != nil {
		t.Fatal(err)
	}
	if files != 1 {
		t.Errorf("files = %d, want 1", files)
	}
	if len(recs) != 2 {
		t.Errorf("records = %d, want 2 (user turn skipped)", len(recs))
	}
	for _, r := range recs {
		if r.SessionType != SessionWorker {
			t.Errorf("SessionType = %q, want worker (from dir name)", r.SessionType)
		}
	}
}

func TestScanProjectsMissingDirIsError(t *testing.T) {
	_, _, err := ScanProjects(filepath.Join(t.TempDir(), "does-not-exist"), DefaultPricing)
	if err == nil {
		t.Error("expected error for missing projects dir")
	}
}

func TestRunBurnAlertWritesTrail(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "projects", "-Users-x-vroom-supervisor")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	// Use a recent timestamp so the record lands inside the burn window.
	ts := time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339)
	line := `{"type":"assistant","timestamp":"` + ts + `","costUSD":10,"message":{"usage":{"input_tokens":100,"output_tokens":100}}}`
	if err := os.WriteFile(filepath.Join(proj, "s.jsonl"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	trail := filepath.Join(dir, "trail.jsonl")

	var stdout, stderr bytes.Buffer
	// $10 in a 1h window → $240/24h, well over a $1 threshold.
	code, err := run([]string{
		"--projects", filepath.Join(dir, "projects"),
		"--trail", trail,
		"--burn-threshold", "1",
		"--burn-window", "1h",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if code != exitBurn {
		t.Errorf("exit code = %d, want %d (burn)", code, exitBurn)
	}
	data, err := os.ReadFile(trail)
	if err != nil {
		t.Fatalf("reading trail: %v", err)
	}
	if !strings.Contains(string(data), `"kind":"quota.burn.alert"`) {
		t.Errorf("trail missing quota.burn.alert record:\n%s", data)
	}
	if !strings.Contains(stderr.String(), "WARN quota.burn.alert") {
		t.Errorf("expected WARN log, got: %s", stderr.String())
	}
}

func TestRunNoBurnNoTrail(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "projects", "-Users-x-worker")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"assistant","timestamp":"2020-01-01T00:00:00Z","costUSD":0.01,"message":{"usage":{"input_tokens":1,"output_tokens":1}}}`
	if err := os.WriteFile(filepath.Join(proj, "s.jsonl"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	trail := filepath.Join(dir, "trail.jsonl")

	var stdout, stderr bytes.Buffer
	code, err := run([]string{
		"--projects", filepath.Join(dir, "projects"),
		"--trail", trail,
		"--burn-threshold", "50",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if code != exitOK {
		t.Errorf("exit code = %d, want %d", code, exitOK)
	}
	if _, err := os.Stat(trail); !os.IsNotExist(err) {
		t.Errorf("trail should not be created when under threshold")
	}
}
