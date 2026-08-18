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
	t.Cleanup(func() {
		if err := adapter.Close(); err != nil {
			t.Logf("failed to close adapter: %v", err)
		}
	})
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

func TestAdapterUpdateRejectsUnknownStoredLifecycleAndOutcome(t *testing.T) {
	for _, test := range []struct {
		name       string
		seed       func(*Adapter, *manifest.Manifest) error
		wantErr    string
		readStored func(*Adapter, *manifest.Manifest) (string, error)
		wantStored string
	}{
		{
			name: "lifecycle",
			seed: func(adapter *Adapter, session *manifest.Manifest) error {
				_, err := adapter.Conn().Exec(
					`UPDATE agm_sessions SET status = 'unknown-lifecycle' WHERE id = ? AND workspace = ?`,
					session.SessionID,
					"test",
				)
				return err
			},
			wantErr: "invalid stored lifecycle status",
			readStored: func(adapter *Adapter, session *manifest.Manifest) (string, error) {
				var status string
				err := adapter.Conn().QueryRow(
					`SELECT status FROM agm_sessions WHERE id = ? AND workspace = ?`,
					session.SessionID,
					"test",
				).Scan(&status)
				return status, err
			},
			wantStored: "unknown-lifecycle",
		},
		{
			name: "outcome",
			seed: func(adapter *Adapter, session *manifest.Manifest) error {
				_, err := adapter.Conn().Exec(
					`UPDATE agm_sessions SET metadata = ? WHERE id = ? AND workspace = ?`,
					`{"outcome":"unknown-outcome","private_note":"must not appear in errors"}`,
					session.SessionID,
					"test",
				)
				return err
			},
			wantErr: "invalid metadata outcome",
			readStored: func(adapter *Adapter, session *manifest.Manifest) (string, error) {
				var metadata string
				err := adapter.Conn().QueryRow(
					`SELECT metadata FROM agm_sessions WHERE id = ? AND workspace = ?`,
					session.SessionID,
					"test",
				).Scan(&metadata)
				return metadata, err
			},
			wantStored: `{"outcome":"unknown-outcome","private_note":"must not appear in errors"}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			adapter := newLifecycleBoundaryAdapter(t)
			session := NewTestManifest("update-"+test.name, "update-"+test.name)
			if err := adapter.CreateSession(session); err != nil {
				t.Fatalf("CreateSession() error: %v", err)
			}
			if err := test.seed(adapter, session); err != nil {
				t.Fatalf("seed invalid stored %s: %v", test.name, err)
			}

			candidate := *session
			candidate.Context.Notes = "valid update must not erase corrupt durable state"
			err := adapter.UpdateSession(&candidate)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("UpdateSession() error = %v, want %q", err, test.wantErr)
			}
			if strings.Contains(err.Error(), "must not appear in errors") {
				t.Fatalf("UpdateSession() error leaked stored metadata: %v", err)
			}
			stored, err := test.readStored(adapter, session)
			if err != nil {
				t.Fatalf("read stored %s: %v", test.name, err)
			}
			if stored != test.wantStored {
				t.Fatalf("stored %s = %q, want %q", test.name, stored, test.wantStored)
			}
		})
	}
}

func TestAdapterUpdateAcceptsLegacyEmptyLifecycleAndOutcome(t *testing.T) {
	adapter := newLifecycleBoundaryAdapter(t)
	session := NewTestManifest("legacy-update", "legacy-update")
	if err := adapter.CreateSession(session); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	if _, err := adapter.Conn().Exec(
		`UPDATE agm_sessions SET status = '', metadata = '{"outcome":""}' WHERE id = ? AND workspace = ?`,
		session.SessionID,
		"test",
	); err != nil {
		t.Fatalf("seed legacy durable values: %v", err)
	}

	candidate := *session
	candidate.Context.Notes = "legacy values remain writable"
	if err := adapter.UpdateSession(&candidate); err != nil {
		t.Fatalf("UpdateSession() rejected legacy durable values: %v", err)
	}
}

func TestListActiveSessionsRejectsUnknownStoredLifecycle(t *testing.T) {
	adapter := newLifecycleBoundaryAdapter(t)
	session := NewTestManifest("unknown-active", "unknown-active")
	if err := adapter.CreateSession(session); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	if _, err := adapter.Conn().Exec(
		`UPDATE agm_sessions SET status = 'unknown-lifecycle' WHERE id = ? AND workspace = ?`,
		session.SessionID,
		"test",
	); err != nil {
		t.Fatalf("seed invalid stored status: %v", err)
	}

	_, err := adapter.ListActiveSessions(t.Context())
	if err == nil || !strings.Contains(err.Error(), "invalid stored lifecycle status") {
		t.Fatalf("ListActiveSessions() error = %v, want invalid stored lifecycle status", err)
	}
}

func TestListActiveSessionsPreservesLegacyAndArchivedBehavior(t *testing.T) {
	adapter := newLifecycleBoundaryAdapter(t)
	legacy := NewTestManifest("legacy-empty", "legacy-empty")
	active := NewTestManifest("legacy-active", "legacy-active")
	archived := NewTestManifest("archived", "archived")
	archived.Lifecycle = manifest.LifecycleArchived
	for _, session := range []*manifest.Manifest{legacy, active, archived} {
		if err := adapter.CreateSession(session); err != nil {
			t.Fatalf("CreateSession(%q) error: %v", session.SessionID, err)
		}
	}
	if _, err := adapter.Conn().Exec(
		`UPDATE agm_sessions SET status = '' WHERE id = ? AND workspace = ?`,
		legacy.SessionID,
		"test",
	); err != nil {
		t.Fatalf("seed empty legacy status: %v", err)
	}

	names, err := adapter.ListActiveSessions(t.Context())
	if err != nil {
		t.Fatalf("ListActiveSessions() error: %v", err)
	}
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		seen[name] = true
	}
	if !seen[legacy.Name] || !seen[active.Name] || seen[archived.Name] {
		t.Fatalf("ListActiveSessions() = %v, want legacy empty and active only", names)
	}
}

func TestLinkSessionParentRejectsUnknownStoredLifecycle(t *testing.T) {
	adapter := newLifecycleBoundaryAdapter(t)
	parent := NewTestManifest("parent", "parent")
	child := NewTestManifest("child", "child")
	for _, session := range []*manifest.Manifest{parent, child} {
		if err := adapter.CreateSession(session); err != nil {
			t.Fatalf("CreateSession(%q) error: %v", session.SessionID, err)
		}
	}
	observed, err := adapter.GetSession(child.SessionID)
	if err != nil {
		t.Fatalf("GetSession(child) error: %v", err)
	}
	if _, err := adapter.Conn().Exec(
		`UPDATE agm_sessions SET status = 'unknown-lifecycle' WHERE id = ? AND workspace = ?`,
		child.SessionID,
		"test",
	); err != nil {
		t.Fatalf("seed invalid child status: %v", err)
	}

	inheritedName := "parent-child"
	err = adapter.LinkSessionParent(t.Context(), child.SessionID, observed.Tmux.SessionRevision, parent.SessionID, &inheritedName)
	if err == nil || !strings.Contains(err.Error(), "invalid stored lifecycle status") {
		t.Fatalf("LinkSessionParent() error = %v, want invalid stored lifecycle status", err)
	}
	var linked int
	if err := adapter.Conn().QueryRow(
		`SELECT COUNT(*) FROM agm_sessions WHERE id = ? AND workspace = ? AND parent_session_id = ?`,
		child.SessionID,
		"test",
		parent.SessionID,
	).Scan(&linked); err != nil {
		t.Fatalf("count linked child rows: %v", err)
	}
	if linked != 0 {
		t.Fatalf("LinkSessionParent() linked corrupt child rows = %d, want 0", linked)
	}
}
