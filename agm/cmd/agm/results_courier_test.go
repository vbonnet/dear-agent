package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeJSONLLine(t *testing.T, path string, typ, text string) {
	t.Helper()
	entry := map[string]any{"type": typ}
	if typ == "assistant" {
		entry["message"] = map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
		}
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestScanClaudeProjects_FirstRunSeedsWithoutReporting(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "projects", strings.ReplaceAll(home, "/", "-")+"-src-dear-agent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "session1.jsonl")
	writeJSONLLine(t, path, "user", "")
	writeJSONLLine(t, path, "assistant", "PR opened: https://github.com/vbonnet/dear-agent/pull/945")

	// Backdate mtime past the idle grace so the file is eligible this tick.
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	st := courierState{Files: map[string]courierFileState{}}
	events, err := scanClaudeProjects(home, 45*time.Second, &st)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("first run should seed silently, got %d events", len(events))
	}
	if _, ok := st.Files[path]; !ok {
		t.Fatal("expected watermark recorded for the seeded file")
	}
}

func TestScanClaudeProjects_ReportsNewCompletionAfterSeed(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "projects", strings.ReplaceAll(home, "/", "-")+"-src-dear-agent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "session1.jsonl")
	writeJSONLLine(t, path, "user", "")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	st := courierState{Files: map[string]courierFileState{}}
	if _, err := scanClaudeProjects(home, 45*time.Second, &st); err != nil {
		t.Fatalf("seed scan: %v", err)
	}
	if len(st.Files) != 1 {
		t.Fatalf("expected one seeded file, got %d", len(st.Files))
	}

	// New completion appended after the seed.
	writeJSONLLine(t, path, "assistant", "Shrink the 98GB Colima VM footprint: done, freed 61GB")
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	events, err := scanClaudeProjects(home, 45*time.Second, &st)
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected exactly one new completion, got %d: %+v", len(events), events)
	}
	if events[0].Project != "dear-agent" {
		t.Errorf("project = %q, want dear-agent", events[0].Project)
	}
	if events[0].Headline != "Shrink the 98GB Colima VM footprint: done, freed 61GB" {
		t.Errorf("headline = %q", events[0].Headline)
	}

	// A third scan with no new lines must not re-report the same completion.
	again, err := scanClaudeProjects(home, 45*time.Second, &st)
	if err != nil {
		t.Fatalf("third scan: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("expected no repeat report, got %d", len(again))
	}
}

func TestScanClaudeProjects_SkipsStillStreamingFile(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "projects", strings.ReplaceAll(home, "/", "-")+"-src-dear-agent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "session1.jsonl")
	writeJSONLLine(t, path, "assistant", "still going")
	// mtime is "now" (default from write) — well within idle grace.

	st := courierState{Files: map[string]courierFileState{}}
	events, err := scanClaudeProjects(home, 45*time.Second, &st)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no events for an actively-streaming file, got %d", len(events))
	}
	if _, known := st.Files[path]; known {
		t.Fatal("a file within idle grace should not get a watermark yet")
	}
}

func TestLastAssistantText_IgnoresToolOnlyTurns(t *testing.T) {
	lines := []string{
		`{"type":"user"}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"final answer"}]}}`,
	}
	got := lastAssistantText(lines, 0)
	if got != "final answer" {
		t.Errorf("got %q, want %q", got, "final answer")
	}
}

func TestLastAssistantText_NoQualifyingLinesReturnsEmpty(t *testing.T) {
	lines := []string{
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash"}]}}`,
	}
	if got := lastAssistantText(lines, 0); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestProjectLabel(t *testing.T) {
	home := "/Users/vbonnet"
	cases := []struct{ dir, want string }{
		{"-Users-vbonnet-src-dear-agent", "dear-agent"},
		{"-Users-vbonnet-worktrees-dear-agent-results-courier", "dear-agent-results-courier"},
		{"-Users-vbonnet", "home"},
		{"-some-other-unrelated-slug", "some-other-unrelated-slug"}, // no home prefix: raw fallback
	}
	for _, c := range cases {
		if got := projectLabel(home, c.dir); got != c.want {
			t.Errorf("projectLabel(%q) = %q, want %q", c.dir, got, c.want)
		}
	}
}

func TestFormatDigest(t *testing.T) {
	events := []resultsCourierEvent{
		{Project: "dear-agent", Headline: "PR #944 merged"},
		{Project: "dear-agent", Headline: "PR #945 opened"},
	}
	title, body := formatDigest(events)
	if title != "2 sessions finished" {
		t.Errorf("title = %q", title)
	}
	if body != "dear-agent: PR #944 merged | dear-agent: PR #945 opened" {
		t.Errorf("body = %q", body)
	}
}

func TestFormatDigest_Singular(t *testing.T) {
	title, _ := formatDigest([]resultsCourierEvent{{Project: "x", Headline: "y"}})
	if title != "1 session finished" {
		t.Errorf("title = %q, want singular", title)
	}
}

func TestCourierStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "state.json")

	st := courierState{Files: map[string]courierFileState{
		"/a/b.jsonl": {Size: 123, Line: 7},
	}}
	if err := saveCourierState(path, st); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := loadCourierState(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Files["/a/b.jsonl"] != (courierFileState{Size: 123, Line: 7}) {
		t.Errorf("got %+v", loaded.Files["/a/b.jsonl"])
	}
}

func TestLoadCourierState_MissingFileReturnsEmpty(t *testing.T) {
	st, err := loadCourierState(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("missing state file should not error: %v", err)
	}
	if len(st.Files) != 0 {
		t.Errorf("expected zero-value state, got %+v", st)
	}
}

func TestTruncateHeadline(t *testing.T) {
	if got := truncateHeadline("hello   world\nfoo", 100); got != "hello world foo" {
		t.Errorf("got %q", got)
	}
	if got := truncateHeadline("abcdefgh", 4); got != "abcd…" {
		t.Errorf("got %q", got)
	}
}
