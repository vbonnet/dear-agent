package usage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNew_ExplicitPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")

	tr, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if tr.FilePath() != path {
		t.Errorf("FilePath() = %q, want %q", tr.FilePath(), path)
	}
}

func TestTrackSync_WritesToFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")

	tr, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ev := Event{
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Command:   "agm session list",
		Success:   true,
	}
	if err := tr.TrackSync(ev); err != nil {
		t.Fatalf("TrackSync: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "agm session list") {
		t.Errorf("usage file does not contain command; got %q", string(data))
	}
}

func TestTrackSync_AppendMultiple(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")

	tr, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for _, cmd := range []string{"cmd-a", "cmd-b", "cmd-c"} {
		ev := Event{Command: cmd, Success: true}
		if err := tr.TrackSync(ev); err != nil {
			t.Fatalf("TrackSync(%q): %v", cmd, err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	for _, want := range []string{"cmd-a", "cmd-b", "cmd-c"} {
		if !strings.Contains(content, want) {
			t.Errorf("usage file missing %q; got %q", want, content)
		}
	}
}

func TestTrackSync_ErrorEvent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")

	tr, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ev := Event{
		Command: "agm session start",
		Success: false,
		Error:   "connection refused",
	}
	if err := tr.TrackSync(ev); err != nil {
		t.Fatalf("TrackSync error event: %v", err)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "connection refused") {
		t.Errorf("usage file missing error message; got %q", string(data))
	}
}

func TestTrack_Async_DoesNotBlock(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tr, err := New(filepath.Join(dir, "usage.jsonl"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Track is fire-and-forget; just verify it doesn't panic.
	tr.Track(Event{Command: "async-cmd", Success: true})
}
