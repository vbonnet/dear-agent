package dolt

import (
	"path/filepath"
	"strings"
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

func TestSQLiteSandboxOwnershipMetadataRoundTripsForArchive(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	createdAt := time.Now().UTC().Truncate(time.Microsecond)
	sandboxBase := filepath.Join(t.TempDir(), ".agm", "sandboxes")
	wantSandbox := &manifest.SandboxConfig{
		Enabled:               true,
		ID:                    "sandbox-roundtrip-session",
		Provider:              "apfs-reflink",
		MergedPath:            filepath.Join(sandboxBase, "sandbox-roundtrip-session", "merged"),
		WorkingDir:            filepath.Join(sandboxBase, "sandbox-roundtrip-session", "merged", "repo0"),
		CreatedAt:             createdAt,
		ExtraAddDirs:          []string{"/real/worktree"},
		BypassCodexHookTrust:  true,
		CodexHookSourceRepo:   "/reviewed/source",
		CodexHookSourceCommit: strings.Repeat("a", 40),
		CodexHookDigest:       strings.Repeat("b", 64),
		CodexHookRoot:         filepath.Join(sandboxBase, "trusted-hooks", strings.Repeat("b", 64)),
	}
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "sandbox-roundtrip-session",
		Name:          "sandbox-roundtrip-session",
		Harness:       "codex-cli",
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
		Context:       manifest.Context{Project: wantSandbox.WorkingDir},
		Tmux:          manifest.Tmux{SessionName: "sandbox-roundtrip-session"},
		Sandbox:       wantSandbox,
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}

	assertSandbox := func(t *testing.T, got, want *manifest.SandboxConfig) {
		t.Helper()
		if got == nil {
			t.Fatal("Sandbox = nil, want persisted ownership metadata")
		}
		if got.Enabled != want.Enabled ||
			got.ID != want.ID ||
			got.Provider != want.Provider ||
			got.MergedPath != want.MergedPath ||
			got.WorkingDir != want.WorkingDir ||
			strings.Join(got.ExtraAddDirs, "\x00") != strings.Join(want.ExtraAddDirs, "\x00") ||
			got.BypassCodexHookTrust != want.BypassCodexHookTrust ||
			got.CodexHookSourceRepo != want.CodexHookSourceRepo ||
			got.CodexHookSourceCommit != want.CodexHookSourceCommit ||
			got.CodexHookDigest != want.CodexHookDigest ||
			got.CodexHookRoot != want.CodexHookRoot ||
			!got.CreatedAt.Equal(want.CreatedAt) {
			t.Fatalf("Sandbox = %#v, want %#v", got, want)
		}
	}

	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after create error: %v", err)
	}
	assertSandbox(t, stored.Sandbox, wantSandbox)

	wantSandbox = &manifest.SandboxConfig{
		Enabled:    true,
		ID:         m.SessionID,
		Provider:   "mock-updated",
		MergedPath: filepath.Join(sandboxBase, m.SessionID, "merged"),
		WorkingDir: filepath.Join(sandboxBase, m.SessionID, "merged", "repo1"),
		CreatedAt:  createdAt.Add(time.Second),
	}
	stored.Sandbox = wantSandbox
	if err := adapter.UpdateSession(stored); err != nil {
		t.Fatalf("UpdateSession() error: %v", err)
	}
	updated, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after update error: %v", err)
	}
	assertSandbox(t, updated.Sandbox, wantSandbox)
}

func TestSQLiteMissingSandboxMetadataDoesNotInferOwnership(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	now := time.Now()
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "legacy-session-without-sandbox-metadata",
		Name:          "legacy-session-without-sandbox-metadata",
		Harness:       "codex-cli",
		CreatedAt:     now,
		UpdatedAt:     now,
		Context: manifest.Context{
			Project: "/Users/example/.agm/sandboxes/unowned/merged/repo0",
		},
		Tmux: manifest.Tmux{SessionName: "legacy-session-without-sandbox-metadata"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	if stored.Sandbox != nil {
		t.Fatalf("Sandbox = %#v, want nil without persisted ownership", stored.Sandbox)
	}
}

func TestSQLiteInvalidSandboxMetadataDoesNotAuthorizeCleanup(t *testing.T) {
	tests := []struct {
		name    string
		sandbox func(sessionID, base string) *manifest.SandboxConfig
	}{
		{
			name: "partial record",
			sandbox: func(sessionID, base string) *manifest.SandboxConfig {
				return &manifest.SandboxConfig{
					Enabled:    true,
					ID:         sessionID,
					MergedPath: filepath.Join(base, sessionID, "merged"),
				}
			},
		},
		{
			name: "mismatched ID",
			sandbox: func(_ string, base string) *manifest.SandboxConfig {
				return &manifest.SandboxConfig{
					Enabled:    true,
					ID:         "other-session",
					Provider:   "mock",
					MergedPath: filepath.Join(base, "other-session", "merged"),
					WorkingDir: filepath.Join(base, "other-session", "merged", "repo0"),
					CreatedAt:  time.Now(),
				}
			},
		},
		{
			name: "working directory outside merged boundary",
			sandbox: func(sessionID, base string) *manifest.SandboxConfig {
				return &manifest.SandboxConfig{
					Enabled:    true,
					ID:         sessionID,
					Provider:   "mock",
					MergedPath: filepath.Join(base, sessionID, "merged"),
					WorkingDir: filepath.Join(base, "unowned"),
					CreatedAt:  time.Now(),
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
			if err != nil {
				t.Fatalf("NewSQLiteAdapter() error: %v", err)
			}
			t.Cleanup(func() { _ = adapter.Close() })

			sessionID := "invalid-sandbox-" + strings.ReplaceAll(tt.name, " ", "-")
			now := time.Now()
			base := filepath.Join(t.TempDir(), ".agm", "sandboxes")
			m := &manifest.Manifest{
				SchemaVersion: manifest.SchemaVersion,
				SessionID:     sessionID,
				Name:          sessionID,
				Harness:       "codex-cli",
				CreatedAt:     now,
				UpdatedAt:     now,
				Context:       manifest.Context{Project: t.TempDir()},
				Sandbox:       tt.sandbox(sessionID, base),
			}
			if err := adapter.CreateSession(m); err != nil {
				t.Fatalf("CreateSession() error: %v", err)
			}
			stored, err := adapter.GetSession(sessionID)
			if err != nil {
				t.Fatalf("GetSession() error: %v", err)
			}
			if stored.Sandbox != nil {
				t.Fatalf("Sandbox = %#v, want nil without complete valid ownership", stored.Sandbox)
			}
		})
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
