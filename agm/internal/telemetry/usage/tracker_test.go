package usage

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNew_ExplicitPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")
	tr, err := New(path)
	if err != nil {
		t.Fatalf("New(%q) error: %v", path, err)
	}
	if tr.FilePath() != path {
		t.Errorf("FilePath() = %q, want %q", tr.FilePath(), path)
	}
}

func TestNew_EmptyPath_UsesDefault(t *testing.T) {
	tr, err := New("")
	if err != nil {
		t.Fatalf("New(\"\") error: %v", err)
	}
	if !strings.HasSuffix(tr.FilePath(), "usage.jsonl") {
		t.Errorf("default FilePath() = %q, want suffix 'usage.jsonl'", tr.FilePath())
	}
}

func TestTrackSync_WritesJSONLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")
	tr, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ev := Event{
		Command: "agm",
		Args:    []string{"session", "list"},
		Success: true,
	}
	if err := tr.TrackSync(ev); err != nil {
		t.Fatalf("TrackSync: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		t.Fatal("expected at least one line in usage file")
	}
	var got Event
	if err := json.Unmarshal(sc.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Command != "agm" {
		t.Errorf("Command = %q, want %q", got.Command, "agm")
	}
	if !got.Success {
		t.Errorf("Success = false, want true")
	}
	if got.Timestamp.IsZero() {
		t.Error("Timestamp should be set by TrackSync")
	}
}

func TestTrackSync_AppendMultiple(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")
	tr, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for range 3 {
		if err := tr.TrackSync(Event{Command: "cmd", Success: true}); err != nil {
			t.Fatalf("TrackSync: %v", err)
		}
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	lines := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines++
	}
	if lines != 3 {
		t.Errorf("expected 3 lines, got %d", lines)
	}
}
