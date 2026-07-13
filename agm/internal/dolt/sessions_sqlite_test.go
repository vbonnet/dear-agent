package dolt

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestSQLiteGetSessionByUUID_ClaudeUUID(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "sqlite-uuid-session",
		Name:          "sqlite-uuid-session",
		Harness:       "claude-code",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context:       manifest.Context{Project: t.TempDir()},
		Claude:        manifest.Claude{UUID: "claude-conversation-uuid"},
		Tmux:          manifest.Tmux{SessionName: "sqlite-uuid-session"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}

	found, err := adapter.GetSessionByUUID(m.Claude.UUID)
	if err != nil {
		t.Fatalf("GetSessionByUUID() error: %v", err)
	}
	if found == nil || found.SessionID != m.SessionID {
		t.Fatalf("GetSessionByUUID() = %#v, want session %q", found, m.SessionID)
	}

	codex := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "sqlite-codex-session",
		Name:          "sqlite-codex-session",
		Harness:       "codex-cli",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context:       manifest.Context{Project: t.TempDir()},
		Codex:         &manifest.Codex{SessionID: "codex-conversation-uuid"},
		Tmux:          manifest.Tmux{SessionName: "sqlite-codex-session"},
	}
	if err := adapter.CreateSession(codex); err != nil {
		t.Fatalf("CreateSession(codex) error: %v", err)
	}

	found, err = adapter.GetSessionByUUID(codex.Codex.SessionID)
	if err != nil {
		t.Fatalf("GetSessionByUUID(codex) error: %v", err)
	}
	if found == nil || found.SessionID != codex.SessionID {
		t.Fatalf("GetSessionByUUID(codex) = %#v, want session %q", found, codex.SessionID)
	}
}
