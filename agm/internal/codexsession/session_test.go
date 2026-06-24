package codexsession

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindByID(t *testing.T) {
	home := t.TempDir()
	sessionID := "019ef2af-97e0-7443-9f07-03e40636740c"
	sessionDir := filepath.Join(home, ".codex", "sessions", "2026", "06", "22")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(sessionDir, "rollout-2026-06-22T21-14-14-"+sessionID+".jsonl")
	line := `{"type":"session_meta","payload":{"session_id":"` + sessionID + `","id":"` + sessionID + `","cwd":"/Users/vbonnet/src/dear-agent"}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	got, err := FindByID(home, sessionID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.SessionID != sessionID {
		t.Fatalf("SessionID = %q, want %q", got.SessionID, sessionID)
	}
	if got.CWD != "/Users/vbonnet/src/dear-agent" {
		t.Fatalf("CWD = %q", got.CWD)
	}
	if got.Path != path {
		t.Fatalf("Path = %q, want %q", got.Path, path)
	}
	if got.Archived {
		t.Fatal("active transcript reported as archived")
	}
}

func TestFindByIDArchived(t *testing.T) {
	home := t.TempDir()
	sessionID := "archived-codex-session"
	archiveDir := filepath.Join(home, ".codex", "archived_sessions")
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(archiveDir, "rollout-"+sessionID+".jsonl")
	line := `{"type":"session_meta","payload":{"session_id":"` + sessionID + `","cwd":"/tmp/work"}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	got, err := FindByID(home, sessionID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if !got.Archived {
		t.Fatal("archived transcript not marked archived")
	}
}
