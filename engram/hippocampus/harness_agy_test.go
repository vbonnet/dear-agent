package hippocampus

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAgyAdapterDiscoverAndRead(t *testing.T) {
	root := t.TempDir()
	project := t.TempDir()
	id := "117ff898-a964-4a9f-b460-1be4a8a49b17"
	cacheDir := filepath.Join(root, "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cache, _ := json.Marshal(map[string]string{project: id})
	if err := os.WriteFile(filepath.Join(cacheDir, "last_conversations.json"), cache, 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "brain", id, ".system_generated", "logs", "transcript.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte("{\"type\":\"USER_INPUT\",\"content\":\"question\",\"created_at\":\"2026-07-09T01:02:03Z\"}\n" +
		"{\"type\":\"LIST_DIRECTORY\",\"content\":\"noise\"}\n" +
		"{\"type\":\"PLANNER_RESPONSE\",\"content\":\"answer\"}\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	adapter := NewAgyAdapter(root)
	sessions, err := adapter.DiscoverSessions(context.Background(), project, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].ID != id {
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

func TestAgyAdapterRequiresProjectMappingForFilter(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "brain", "session", ".system_generated", "logs", "transcript.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := NewAgyAdapter(root).DiscoverSessions(context.Background(), t.TempDir(), time.Time{})
	if err != nil || len(got) != 0 {
		t.Fatalf("sessions=%v err=%v", got, err)
	}
}

func TestAgyAdapterMapsOlderConversationFromLogs(t *testing.T) {
	root := t.TempDir()
	project := t.TempDir()
	id := "117ff898-a964-4a9f-b460-1be4a8a49b17"
	logDir := filepath.Join(root, "log")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	log := "Initializing CLI store manager for workspace " + project + "\nCreated conversation " + id + "\n"
	if err := os.WriteFile(filepath.Join(logDir, "agy.log"), []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "brain", id, ".system_generated", "logs", "transcript.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := NewAgyAdapter(root).DiscoverSessions(context.Background(), project, time.Time{})
	if err != nil || len(got) != 1 || got[0].ID != id {
		t.Fatalf("sessions=%v err=%v", got, err)
	}
}
