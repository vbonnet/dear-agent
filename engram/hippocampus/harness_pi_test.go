package hippocampus

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPiAdapterDiscoversExactProjectAndReadsConversationText(t *testing.T) {
	root := t.TempDir()
	project := t.TempDir()
	nested := filepath.Join(root, "--project--")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(nested, "pi.jsonl")
	data := `{"type":"session","id":"pi-hippocampus","cwd":"` + project + `","timestamp":"2026-07-21T00:00:00Z"}` + "\n" +
		`{"type":"message","message":{"role":"user","content":[{"type":"text","text":"question"}]}}` + "\n" +
		`{"type":"message","message":{"role":"assistant","content":[{"type":"thinking","thinking":"private"},{"type":"text","text":"answer"}]}}` + "\n" +
		`{"type":"message","message":{"role":"toolResult","content":[{"type":"text","text":"tool secret"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := NewPiAdapter(root)
	sessions, err := adapter.DiscoverSessions(t.Context(), project, time.Time{})
	if err != nil || len(sessions) != 1 || sessions[0].ID != "pi-hippocampus" {
		t.Fatalf("DiscoverSessions = %#v, %v", sessions, err)
	}
	text, err := adapter.ReadTranscript(t.Context(), sessions[0])
	if err != nil {
		t.Fatal(err)
	}
	if text != "user: question\nassistant: answer" || strings.Contains(text, "private") || strings.Contains(text, "tool secret") {
		t.Fatalf("Pi transcript = %q", text)
	}
}

func TestPiAdapterHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := NewPiAdapter(t.TempDir()).DiscoverSessions(ctx, "", time.Time{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
