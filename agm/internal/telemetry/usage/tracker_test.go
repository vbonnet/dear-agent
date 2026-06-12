package usage_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/telemetry/usage"
)

func TestNew_ExplicitPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")
	tr, err := usage.New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if tr.FilePath() != path {
		t.Errorf("FilePath = %q, want %q", tr.FilePath(), path)
	}
}

func TestTrackSync_WritesJSONLine(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")
	tr, err := usage.New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ev := usage.Event{
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Command:   "session new",
		Args:      []string{"--name", "test"},
		Success:   true,
	}
	if err := tr.TrackSync(ev); err != nil {
		t.Fatalf("TrackSync: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got usage.Event
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v (data=%q)", err, data)
	}
	if got.Command != "session new" {
		t.Errorf("Command = %q, want %q", got.Command, "session new")
	}
	if !got.Success {
		t.Error("Success should be true")
	}
}

func TestTrackSync_MultipleEvents(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")
	tr, err := usage.New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for i := range 3 {
		ev := usage.Event{Command: "cmd", Args: []string{string(rune('a' + i))}, Success: true}
		if err := tr.TrackSync(ev); err != nil {
			t.Fatalf("TrackSync %d: %v", i, err)
		}
	}

	data, _ := os.ReadFile(path)
	lines := splitLines(data)
	if len(lines) != 3 {
		t.Errorf("expected 3 JSONL lines, got %d", len(lines))
	}
}

func TestTrackSync_SetsTimestampWhenZero(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")
	tr, err := usage.New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ev := usage.Event{Command: "x", Success: true} // zero Timestamp
	if err := tr.TrackSync(ev); err != nil {
		t.Fatalf("TrackSync: %v", err)
	}

	data, _ := os.ReadFile(path)
	var got usage.Event
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Timestamp.IsZero() {
		t.Error("Timestamp should have been set automatically")
	}
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			line := data[start:i]
			if len(line) > 0 {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	return lines
}
