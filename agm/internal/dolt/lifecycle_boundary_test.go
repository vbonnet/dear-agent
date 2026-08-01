package dolt

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func newLifecycleBoundaryAdapter(t *testing.T) *Adapter {
	t.Helper()
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	return adapter
}

func TestAdapterRejectsInvalidLifecycleAndOutcomeWrites(t *testing.T) {
	adapter := newLifecycleBoundaryAdapter(t)
	for _, test := range []struct {
		name   string
		mutate func(*manifest.Manifest)
	}{
		{name: "create lifecycle", mutate: func(m *manifest.Manifest) { m.Lifecycle = "unknown" }},
		{name: "create outcome", mutate: func(m *manifest.Manifest) { m.Outcome = "unknown" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			session := NewTestManifest(test.name, test.name)
			test.mutate(session)
			if err := adapter.CreateSession(session); err == nil {
				t.Fatalf("CreateSession accepted invalid %s", test.name)
			}
		})
	}

	session := NewTestManifest("valid", "valid")
	if err := adapter.CreateSession(session); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*manifest.Manifest)
	}{
		{name: "update lifecycle", mutate: func(m *manifest.Manifest) { m.Lifecycle = "unknown" }},
		{name: "update outcome", mutate: func(m *manifest.Manifest) { m.Outcome = "unknown" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := *session
			test.mutate(&candidate)
			if err := adapter.UpdateSession(&candidate); err == nil {
				t.Fatalf("UpdateSession accepted invalid %s", test.name)
			}
			stored, err := adapter.GetSession(session.SessionID)
			if err != nil {
				t.Fatalf("GetSession() error: %v", err)
			}
			if stored.Lifecycle != "" || stored.Outcome != manifest.OutcomeUnknown {
				t.Fatalf("failed update changed stored values to lifecycle=%q outcome=%q", stored.Lifecycle, stored.Outcome)
			}
		})
	}
}

func TestAdapterRejectsUnknownPersistedLifecycleStatus(t *testing.T) {
	adapter := newLifecycleBoundaryAdapter(t)
	session := NewTestManifest("stored-status", "stored-status")
	if err := adapter.CreateSession(session); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	if _, err := adapter.Conn().Exec(
		`UPDATE agm_sessions SET status = 'unknown' WHERE id = ? AND workspace = ?`,
		session.SessionID,
		"test",
	); err != nil {
		t.Fatalf("seed invalid stored status: %v", err)
	}
	_, err := adapter.GetSession(session.SessionID)
	if err == nil || !strings.Contains(err.Error(), "invalid stored lifecycle status") {
		t.Fatalf("GetSession() error = %v, want invalid stored lifecycle status", err)
	}
}

func TestAdapterMetadataOutcomeDecodePolicy(t *testing.T) {
	adapter := newLifecycleBoundaryAdapter(t)
	session := NewTestManifest("stored-outcome", "stored-outcome")
	if err := adapter.CreateSession(session); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	for _, test := range []struct {
		name     string
		metadata string
		wantErr  bool
	}{
		{name: "legacy empty", metadata: `{"outcome":""}`},
		{name: "unknown", metadata: `{"outcome":"unknown"}`, wantErr: true},
		{name: "non string", metadata: `{"outcome":42}`, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := adapter.Conn().Exec(
				`UPDATE agm_sessions SET metadata = ? WHERE id = ? AND workspace = ?`,
				test.metadata,
				session.SessionID,
				"test",
			); err != nil {
				t.Fatalf("seed metadata: %v", err)
			}
			stored, err := adapter.GetSession(session.SessionID)
			if test.wantErr {
				if err == nil || !strings.Contains(err.Error(), "metadata outcome") {
					t.Fatalf("GetSession() error = %v, want invalid metadata outcome", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetSession() error: %v", err)
			}
			if stored.Outcome != manifest.OutcomeUnknown {
				t.Fatalf("legacy empty outcome = %q, want empty", stored.Outcome)
			}
		})
	}
}
