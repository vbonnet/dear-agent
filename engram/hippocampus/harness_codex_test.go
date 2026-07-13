package hippocampus

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCodexCLIAdapterDiscoverAndRead(t *testing.T) {
	root := t.TempDir()
	project := t.TempDir()
	transcript := filepath.Join(root, "sessions", "2026", "07", "09", "rollout.jsonl")
	if err := os.MkdirAll(filepath.Dir(transcript), 0o755); err != nil {
		t.Fatal(err)
	}
	content := strings.Join([]string{
		`{"type":"session_meta","payload":{"id":"codex-1","cwd":` + quoteJSON(project) + `,"timestamp":"2026-07-09T01:02:03Z"}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"question"}]}}`,
		`{"type":"response_item","payload":{"type":"function_call","role":"assistant","content":[]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}}`,
	}, "\n")
	if err := os.WriteFile(transcript, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	adapter := NewCodexCLIAdapter(root)
	sessions, err := adapter.DiscoverSessions(context.Background(), project, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].ID != "codex-1" {
		t.Fatalf("sessions = %#v", sessions)
	}
	got, err := adapter.ReadTranscript(context.Background(), sessions[0])
	if err != nil {
		t.Fatal(err)
	}
	if got != "user: question\nassistant: answer" {
		t.Fatalf("transcript = %q", got)
	}
}

func TestCodexCLIAdapterFiltersProjectAndSince(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "archived_sessions", "rollout.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"old","cwd":"/project","timestamp":"2020-01-01T00:00:00Z"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	adapter := NewCodexCLIAdapter(root)
	got, err := adapter.DiscoverSessions(context.Background(), "/other", time.Time{})
	if err != nil || len(got) != 0 {
		t.Fatalf("project filter: sessions=%v err=%v", got, err)
	}
	got, err = adapter.DiscoverSessions(context.Background(), "/project", time.Now().Add(-time.Hour))
	if err != nil || len(got) != 0 {
		t.Fatalf("since filter: sessions=%v err=%v", got, err)
	}
}

func quoteJSON(value string) string {
	return `"` + strings.ReplaceAll(value, `\`, `\\`) + `"`
}
