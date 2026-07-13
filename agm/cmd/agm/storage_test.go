package main

import (
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestTestStoragePersistsCodexMetadataAcrossAdapters(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agm.db")
	t.Setenv("AGM_DB_PATH", dbPath)

	first, err := getStorage()
	if err != nil {
		t.Fatalf("getStorage first process: %v", err)
	}
	m := &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     "test-session-id",
		Name:          "test-session",
		Harness:       "codex-cli",
		Workspace:     "test",
		IsTest:        true,
		Codex: &manifest.Codex{
			SessionID: "codex-thread-id",
		},
		Tmux: manifest.Tmux{SessionName: "test-session"},
	}
	if err := first.CreateSession(m); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first process: %v", err)
	}

	second, err := getStorage()
	if err != nil {
		t.Fatalf("getStorage second process: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	got, err := second.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession from second process: %v", err)
	}
	if got.Codex == nil || got.Codex.SessionID != m.Codex.SessionID {
		t.Fatalf("Codex metadata = %#v, want thread %q", got.Codex, m.Codex.SessionID)
	}
	if got.Name != m.Name || !got.IsTest {
		t.Fatalf("persisted manifest = %#v, want test session %q", got, m.Name)
	}
}
