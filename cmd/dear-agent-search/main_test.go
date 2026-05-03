package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/source"
	sqliteadapter "github.com/vbonnet/dear-agent/pkg/source/sqlite"
)

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		in   string
		want time.Time
	}{
		{"24h", now.Add(-24 * time.Hour)},
		{"30d", now.Add(-30 * 24 * time.Hour)},
		{"2026-04-01", time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		{"2026-04-01T12:00:00Z", time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		got, err := parseSince(c.in, now)
		if err != nil {
			t.Errorf("parseSince(%q): %v", c.in, err)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("parseSince(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	if _, err := parseSince("not a time", now); err == nil {
		t.Error("expected error for invalid input")
	}
}

func TestSplitWorkItem(t *testing.T) {
	cases := []struct{ in, run, node string }{
		{"", "", ""},
		{"run-A", "run-A", ""},
		{"run-A/node1", "run-A", "node1"},
		{"run-A/path/with/slashes", "run-A", "path/with/slashes"},
	}
	for _, c := range cases {
		gotRun, gotNode := splitWorkItem(c.in)
		if gotRun != c.run || gotNode != c.node {
			t.Errorf("splitWorkItem(%q) = (%q,%q), want (%q,%q)", c.in, gotRun, gotNode, c.run, c.node)
		}
	}
}

func TestFormatText_NoResults(t *testing.T) {
	got := formatText("missing", nil)
	if !strings.Contains(got, "no results") {
		t.Errorf("expected 'no results', got %q", got)
	}
}

func TestFormatText_RendersResults(t *testing.T) {
	got := formatText("foo", []result{
		{URI: "u1", Title: "T1", Snippet: "first", RunID: "run-abc12345", NodeID: "n1", RunState: "succeeded"},
		{URI: "u2", Snippet: "second"},
	})
	for _, want := range []string{"[1] T1", "(run run-abc1/n1 succeeded)", "[2] u2", "first", "second"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestRun_EndToEnd seeds two sources and then runs the CLI against the
// same DB. The output must contain both URIs and the work-item annotation
// for the one that was tagged with a run we created.
func TestRun_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "runs.db")

	a, err := sqliteadapter.Open(dbPath)
	if err != nil {
		t.Fatalf("Open adapter: %v", err)
	}
	defer a.Close()
	ctx := context.Background()
	for _, s := range []source.Source{
		{URI: "u1", Title: "Routing", Content: []byte("LLM routing strategies"),
			IndexedAt: time.Now().UTC(),
			Metadata:  source.Metadata{WorkItem: "run-1/research", Cues: []string{"routing"}}},
		{URI: "u2", Title: "Cache", Content: []byte("prompt cache benchmark"),
			IndexedAt: time.Now().UTC().Add(-time.Hour),
			Metadata:  source.Metadata{WorkItem: "run-2/eval", Cues: []string{"cache"}}},
	} {
		if _, err := a.Add(ctx, s); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	// Use Open then close so the CLI sees the file. Re-opening as a
	// fresh process would be more realistic, but run() is in-process.
	_ = a.Close()

	stdout, stderr := captureFile(t)
	defer cleanup(stdout)
	defer cleanup(stderr)

	code := run([]string{"--db", dbPath, "routing"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("run exited %d, stderr=%s", code, readAll(stderr))
	}
	out := readAll(stdout)
	if !strings.Contains(out, "u1") {
		t.Errorf("expected u1 in output, got:\n%s", out)
	}
	if strings.Contains(out, "u2") {
		t.Errorf("did not expect u2 in routing query output, got:\n%s", out)
	}
}

func TestRun_NoArgs_ShowsUsage(t *testing.T) {
	stdout, stderr := captureFile(t)
	defer cleanup(stdout)
	defer cleanup(stderr)
	code := run([]string{}, stdout, stderr)
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

func TestRun_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "runs.db")
	a, err := sqliteadapter.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := a.Add(context.Background(), source.Source{
		URI: "u1", Title: "T", Content: []byte("hello"),
		IndexedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	a.Close()

	stdout, stderr := captureFile(t)
	defer cleanup(stdout)
	defer cleanup(stderr)
	if code := run([]string{"--db", dbPath, "--json", "hello"}, stdout, stderr); code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, readAll(stderr))
	}
	out := readAll(stdout)
	for _, want := range []string{`"results"`, `"uri": "u1"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in JSON output:\n%s", want, out)
		}
	}
}
