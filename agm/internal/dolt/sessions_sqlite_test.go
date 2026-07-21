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

func TestSQLiteSessionLifecycle_RoundTripsReapingTombstone(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "sqlite-reaping-session",
		Name:          "sqlite-reaping-session",
		Harness:       "agy",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "sqlite-reaping-session"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	m.Lifecycle = manifest.LifecycleReaping
	if err := adapter.UpdateSession(m); err != nil {
		t.Fatalf("UpdateSession() error: %v", err)
	}

	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	if stored.Lifecycle != manifest.LifecycleReaping {
		t.Fatalf("Lifecycle = %q, want %q", stored.Lifecycle, manifest.LifecycleReaping)
	}
}

func TestSQLiteUpdateTmuxSessionNamePreservesOtherColumns(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	m := &manifest.Manifest{
		SchemaVersion:  manifest.SchemaVersion,
		SessionID:      "sqlite-tmux-name-session",
		Name:           "sqlite-tmux-name-session",
		Harness:        "codex-cli",
		Model:          "gpt-5.6-codex",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Context:        manifest.Context{Project: t.TempDir(), Notes: "concurrent notes"},
		Claude:         manifest.Claude{UUID: "concurrent-hook-uuid"},
		Tmux:           manifest.Tmux{SessionName: "historical.name"},
		PermissionMode: "plan",
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	if err := adapter.UpdateTmuxSessionName(t.Context(), m.SessionID, "canonical-name"); err != nil {
		t.Fatalf("UpdateTmuxSessionName() error: %v", err)
	}

	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	if stored.Tmux.SessionName != "canonical-name" {
		t.Fatalf("Tmux.SessionName = %q, want canonical-name", stored.Tmux.SessionName)
	}
	if stored.Claude.UUID != m.Claude.UUID || stored.Context.Notes != m.Context.Notes || stored.Model != m.Model || stored.PermissionMode != m.PermissionMode {
		t.Fatalf("narrow tmux update changed other columns: %#v", stored)
	}
}

func TestSQLiteTmuxSessionNameChangeOwnsAndRestoresExactWrite(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	previousUpdatedAt := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "tmux-cas-session",
		Name:          "tmux-cas-session",
		Workspace:     adapter.Workspace(),
		CreatedAt:     previousUpdatedAt,
		UpdatedAt:     previousUpdatedAt,
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "legacy-name"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	stored.Context.Notes = "concurrent metadata retained"
	if err := adapter.UpdateSession(stored); err != nil {
		t.Fatalf("UpdateSession() error: %v", err)
	}
	change, err := adapter.BeginTmuxSessionNameChange(t.Context(), m.SessionID, "canonical-name")
	if err != nil || change == nil {
		t.Fatalf("BeginTmuxSessionNameChange() = (%v, %v), want non-nil change", change, err)
	}
	restored, err := adapter.RestoreTmuxSessionNameChange(t.Context(), *change)
	if err != nil || !restored {
		t.Fatalf("RestoreTmuxSessionNameChange() = (%v, %v), want (true, nil)", restored, err)
	}
	final, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after restore error: %v", err)
	}
	if final.Tmux.SessionName != stored.Tmux.SessionName || final.Context.Notes != "concurrent metadata retained" {
		t.Fatalf("restored state = (%q, %q), want (%q, concurrent metadata retained)", final.Tmux.SessionName, final.Context.Notes, stored.Tmux.SessionName)
	}
}

func TestSQLiteTmuxSessionNameCompensationRejectsNewerMetadata(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "tmux-cas-newer-session",
		Name:          "tmux-cas-newer-session",
		Workspace:     adapter.Workspace(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "legacy-name"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	change, err := adapter.BeginTmuxSessionNameChange(t.Context(), m.SessionID, "canonical-name")
	if err != nil || change == nil {
		t.Fatalf("BeginTmuxSessionNameChange() = (%v, %v), want non-nil change", change, err)
	}
	latest, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	latest.Context.Notes = "newer writer"
	if err := adapter.UpdateSession(latest); err != nil {
		t.Fatalf("UpdateSession() error: %v", err)
	}
	restored, err := adapter.RestoreTmuxSessionNameChange(t.Context(), *change)
	if err != nil || restored {
		t.Fatalf("RestoreTmuxSessionNameChange() = (%v, %v), want (false, nil)", restored, err)
	}
	final, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after rejected restore error: %v", err)
	}
	if final.Tmux.SessionName != "canonical-name" || final.Context.Notes != "newer writer" {
		t.Fatalf("newer state was overwritten: name=%q notes=%q", final.Tmux.SessionName, final.Context.Notes)
	}
}

func TestSQLiteUpdateSessionRoundTripsModel(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "sqlite-model-session",
		Name:          "sqlite-model-session",
		Harness:       "agy",
		Model:         "Gemini 3.5 Flash (Medium)",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "sqlite-model-session"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	m.Model = ""
	if err := adapter.UpdateSession(m); err != nil {
		t.Fatalf("UpdateSession() error: %v", err)
	}

	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	if stored.Model != "" {
		t.Fatalf("Model = %q, want cleared unknown provenance", stored.Model)
	}
}

func TestSQLiteCreateSessionDefaultsModelOnlyForClaude(t *testing.T) {
	for _, tc := range []struct {
		name        string
		harness     string
		wantHarness string
		wantModel   string
	}{
		{name: "legacy manifest", wantHarness: "claude-code", wantModel: "claude-sonnet-4-5"},
		{name: "Claude Code", harness: "claude-code", wantHarness: "claude-code", wantModel: "claude-sonnet-4-5"},
		{name: "Antigravity", harness: "agy", wantHarness: "agy"},
		{name: "Codex", harness: "codex-cli", wantHarness: "codex-cli"},
		{name: "OpenCode", harness: "opencode-cli", wantHarness: "opencode-cli"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
			if err != nil {
				t.Fatalf("NewSQLiteAdapter() error: %v", err)
			}
			t.Cleanup(func() { _ = adapter.Close() })

			m := &manifest.Manifest{
				SchemaVersion: manifest.SchemaVersion,
				SessionID:     "sqlite-create-model-session",
				Name:          "sqlite-create-model-session",
				Harness:       tc.harness,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
				Context:       manifest.Context{Project: t.TempDir()},
				Tmux:          manifest.Tmux{SessionName: "sqlite-create-model-session"},
			}
			if err := adapter.CreateSession(m); err != nil {
				t.Fatalf("CreateSession() error: %v", err)
			}

			stored, err := adapter.GetSession(m.SessionID)
			if err != nil {
				t.Fatalf("GetSession() error: %v", err)
			}
			if stored.Harness != tc.wantHarness || stored.Model != tc.wantModel {
				t.Fatalf("Harness/Model = %q/%q, want %q/%q", stored.Harness, stored.Model, tc.wantHarness, tc.wantModel)
			}
		})
	}
}
