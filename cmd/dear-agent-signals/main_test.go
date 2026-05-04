package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/aggregator"
)

// captureStdout swaps os.Stdout for a pipe, runs fn, restores, and
// returns the captured bytes. Used to make the CLI's printf-based
// output testable without restructuring the renderers.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()
	fn()
	_ = w.Close()
	<-done
	return buf.String()
}

func TestRunUnknownSubcommand(t *testing.T) {
	t.Parallel()
	if code := run(context.Background(), []string{"flarp"}); code != 2 {
		t.Errorf("run(flarp) = %d, want 2", code)
	}
}

func TestRunNoArgs(t *testing.T) {
	t.Parallel()
	if code := run(context.Background(), nil); code != 2 {
		t.Errorf("run() = %d, want 2", code)
	}
}

func TestRunHelp(t *testing.T) {
	out := captureStdout(t, func() {
		if code := run(context.Background(), []string{"--help"}); code != 0 {
			t.Errorf("run(--help) = %d, want 0", code)
		}
	})
	if !strings.Contains(out, "Usage:") {
		t.Errorf("--help output missing 'Usage:': %s", out)
	}
}

func TestReportEmpty(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "signals.db")
	// Open + close to create an empty DB with the schema.
	store, err := aggregator.OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Close()

	args := []string{"report", "--db", dbPath}
	if code := run(context.Background(), args); code != 0 {
		t.Errorf("report on empty db = %d, want 0", code)
	}
}

func TestReportRendersStoredSignals(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "signals.db")
	store, err := aggregator.OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)
	sigs := []aggregator.Signal{
		{ID: "1", Kind: aggregator.KindLintTrend, Subject: "a.go", Value: 7, CollectedAt: at},
		{ID: "2", Kind: aggregator.KindGitActivity, Subject: "/repo", Value: 4, CollectedAt: at},
	}
	if err := store.Insert(context.Background(), sigs); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()

	out := captureStdout(t, func() {
		args := []string{"report", "--db", dbPath, "--json"}
		if code := run(context.Background(), args); code != 0 {
			t.Fatalf("report = %d, want 0", code)
		}
	})

	var parsed map[string][]aggregator.Signal
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("parse JSON output: %v\n%s", err, out)
	}
	if len(parsed["lint_trend"]) != 1 || parsed["lint_trend"][0].Subject != "a.go" {
		t.Errorf("missing lint_trend signal in output: %s", out)
	}
	if len(parsed["git_activity"]) != 1 {
		t.Errorf("missing git_activity signal in output: %s", out)
	}
}

func TestReportScore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "signals.db")
	store, err := aggregator.OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)
	sigs := []aggregator.Signal{
		{ID: "s", Kind: aggregator.KindSecurityAlerts, Subject: "GO-1", Value: 1, CollectedAt: at},
	}
	if err := store.Insert(context.Background(), sigs); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()

	out := captureStdout(t, func() {
		args := []string{"report", "--db", dbPath, "--score", "--json"}
		if code := run(context.Background(), args); code != 0 {
			t.Fatalf("report --score = %d, want 0", code)
		}
	})
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, out)
	}
	if _, ok := parsed["scores"]; !ok {
		t.Errorf("missing 'scores' in output: %s", out)
	}
	if _, ok := parsed["total"]; !ok {
		t.Errorf("missing 'total' in output: %s", out)
	}
}

func TestReportInvalidKind(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "signals.db")
	if store, err := aggregator.OpenSQLiteStore(dbPath); err == nil {
		_ = store.Close()
	}
	args := []string{"report", "--db", dbPath, "--kind", "wat"}
	if code := run(context.Background(), args); code != 1 {
		t.Errorf("report invalid kind = %d, want 1", code)
	}
}

func TestCollectExitCode(t *testing.T) {
	t.Parallel()
	r := aggregator.Report{Errors: map[string]error{"x": nil}}
	if code := collectExitCode(r); code != 0 {
		t.Errorf("nil error map should be 0, got %d", code)
	}
	r2 := aggregator.Report{Errors: map[string]error{"x": io.EOF}}
	if code := collectExitCode(r2); code != 1 {
		t.Errorf("non-nil error should be 1, got %d", code)
	}
}
