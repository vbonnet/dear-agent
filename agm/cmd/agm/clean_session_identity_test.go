package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

func TestUISessionsFromManifests_DuplicateNamesKeepIdentityAndStatus(t *testing.T) {
	updatedAt := time.Now().Add(-time.Hour)
	live := &manifest.Manifest{
		SessionID: "live-id", Name: "shared-name", UpdatedAt: updatedAt,
		Tmux: manifest.Tmux{SessionName: "live-tmux"},
	}
	stopped := &manifest.Manifest{
		SessionID: "stopped-id", Name: "shared-name", UpdatedAt: updatedAt,
		Tmux: manifest.Tmux{SessionName: "stopped-tmux"},
	}
	archived := &manifest.Manifest{
		SessionID: "archived-id", Name: "shared-name", UpdatedAt: updatedAt,
		Lifecycle: manifest.LifecycleArchived,
	}
	tmux := session.NewMockTmux()
	tmux.Sessions["live-tmux"] = true

	manifests := []*manifest.Manifest{live, stopped, archived}
	got := uiSessionsFromManifests(manifests, tmux)
	for i, want := range []struct {
		id, status string
	}{{"live-id", "active"}, {"stopped-id", "stopped"}, {"archived-id", "archived"}} {
		if got[i].Manifest != manifests[i] || got[i].UpdatedAt != updatedAt {
			t.Errorf("session %d lost its exact manifest or update time", i)
		}
		if got[i].SessionID != want.id || got[i].Status != want.status {
			t.Errorf("session %d = (%q, %q), want (%q, %q)",
				i, got[i].SessionID, got[i].Status, want.id, want.status)
		}
	}
}

func TestCleanupConfirmationLabels_KeepSelectedIDsAndEscapeNames(t *testing.T) {
	sessions := []*ui.Session{
		{Manifest: &manifest.Manifest{SessionID: "archive-a", Name: "shared-name"}},
		{Manifest: &manifest.Manifest{SessionID: "archive-b", Name: "shared-name\nDelete another"}},
	}

	labels := cleanupConfirmationLabels(sessions)
	if len(labels) != 2 {
		t.Fatalf("labels = %d, want 2", len(labels))
	}
	if labels[0] != `"shared-name" [ID: "archive-a"]` {
		t.Errorf("first selected target = %q", labels[0])
	}
	if labels[1] != `"shared-name\nDelete another" [ID: "archive-b"]` {
		t.Errorf("second selected target = %q", labels[1])
	}
	if strings.Contains(labels[1], "\n") {
		t.Fatalf("selected target contains a literal newline: %q", labels[1])
	}
}

func TestCleanupDispatch_DuplicateNamesUseSelectedManifests(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	previousCfg := cfg
	cfg = &config.Config{SessionsDir: filepath.Join(root, "sessions")}
	t.Cleanup(func() { cfg = previousCfg })

	adapter, err := dolt.NewSQLiteAdapter(filepath.Join(root, "agm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	makeSession := func(id string, lifecycle string) *manifest.Manifest {
		m := &manifest.Manifest{
			SchemaVersion: manifest.SchemaVersion,
			SessionID:     id,
			Name:          "shared-name",
			Harness:       "agy",
			Lifecycle:     lifecycle,
			CreatedAt:     time.Now().Add(-time.Hour),
			UpdatedAt:     time.Now().Add(-time.Hour),
			Context:       manifest.Context{Project: root},
			Tmux:          manifest.Tmux{SessionName: id},
		}
		if err := adapter.CreateSession(m); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
		if err := os.MkdirAll(getSessionDir(id), 0o700); err != nil {
			t.Fatalf("make session directory %s: %v", id, err)
		}
		return m
	}
	stopped := makeSession("stopped-id", "")
	archivedA := makeSession("archive-a", manifest.LifecycleArchived)
	archivedB := makeSession("archive-b", manifest.LifecycleArchived)
	listed, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]*manifest.Manifest, len(listed))
	for _, m := range listed {
		byID[m.SessionID] = m
	}

	selected := &ui.CleanupResult{
		ToArchive: []*ui.Session{{Manifest: byID[stopped.SessionID], UpdatedAt: byID[stopped.SessionID].UpdatedAt}},
		ToDelete:  []*ui.Session{{Manifest: byID[archivedB.SessionID], UpdatedAt: byID[archivedB.SessionID].UpdatedAt}},
	}
	checker := &strictCleanupChecker{}
	if applied, _, err := applyCleanupSelection(context.Background(), adapter, selected.ToArchive[0],
		cleanupArchive, 0, checker); err != nil || !applied {
		t.Fatalf("archive selected session: applied=%t, err=%v", applied, err)
	}
	if applied, _, err := applyCleanupSelection(context.Background(), adapter, selected.ToDelete[0],
		cleanupDelete, 0, checker); err != nil || !applied {
		t.Fatalf("delete selected session directory: applied=%t, err=%v", applied, err)
	}

	stored, err := adapter.GetSession(stopped.SessionID)
	if err != nil || stored.Lifecycle != manifest.LifecycleArchived {
		t.Fatalf("selected archive lifecycle = %v, err = %v", stored, err)
	}
	unselected, err := adapter.GetSession(archivedA.SessionID)
	if err != nil || unselected.Lifecycle != manifest.LifecycleArchived {
		t.Fatalf("unselected duplicate lifecycle = %v, err = %v", unselected, err)
	}
	if _, err := os.Stat(getSessionDir(archivedA.SessionID)); err != nil {
		t.Fatalf("unselected duplicate directory changed: %v", err)
	}
	if _, err := os.Stat(getSessionDir(archivedB.SessionID)); !os.IsNotExist(err) {
		t.Fatalf("selected delete directory still exists or stat failed: %v", err)
	}
}
