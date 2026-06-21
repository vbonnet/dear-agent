package ops

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// newStoppedManifest builds a stopped (no tmux) session record for outcome tests.
func newStoppedManifest(id, name string) *manifest.Manifest {
	now := time.Now()
	return &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     id,
		Name:          name,
		Harness:       "claude-code",
		CreatedAt:     now.Add(-24 * time.Hour),
		UpdatedAt:     now.Add(-1 * time.Hour),
		State:         "DONE",
		Tmux:          manifest.Tmux{SessionName: name},
		Context:       manifest.Context{Project: "/tmp/test"},
	}
}

// TestSessionOutcome_StampedOnArchive: an archive with no explicit outcome
// defaults to "completed", and the value is persisted on the record.
func TestSessionOutcome_StampedOnArchive(t *testing.T) {
	storage := dolt.NewMockAdapter()
	tmux := session.NewMockTmux()
	_ = storage.CreateSession(newStoppedManifest("sid-1", "worker-1"))

	ctx := &OpContext{Storage: storage, Tmux: tmux}
	result, err := ArchiveSession(ctx, &ArchiveSessionRequest{Identifier: "sid-1", Force: true})
	if err != nil {
		t.Fatalf("ArchiveSession failed: %v", err)
	}
	if result.Outcome != manifest.OutcomeCompleted {
		t.Errorf("result outcome = %q, want %q", result.Outcome, manifest.OutcomeCompleted)
	}

	updated, _ := storage.GetSession("sid-1")
	if updated.Outcome != manifest.OutcomeCompleted {
		t.Errorf("persisted outcome = %q, want %q", updated.Outcome, manifest.OutcomeCompleted)
	}
}

// TestSessionOutcome_ExplicitValueStamped: an explicit outcome (e.g. the value
// gc passes) is preserved rather than overwritten by the default.
func TestSessionOutcome_ExplicitValueStamped(t *testing.T) {
	cases := []manifest.SessionOutcome{
		manifest.OutcomeCrashed,
		manifest.OutcomeKilled,
		manifest.OutcomeGCStale,
	}
	for _, want := range cases {
		t.Run(string(want), func(t *testing.T) {
			storage := dolt.NewMockAdapter()
			tmux := session.NewMockTmux()
			_ = storage.CreateSession(newStoppedManifest("sid-x", "worker-x"))

			ctx := &OpContext{Storage: storage, Tmux: tmux}
			if _, err := ArchiveSession(ctx, &ArchiveSessionRequest{
				Identifier: "sid-x",
				Force:      true,
				Outcome:    want,
			}); err != nil {
				t.Fatalf("ArchiveSession failed: %v", err)
			}

			updated, _ := storage.GetSession("sid-x")
			if updated.Outcome != want {
				t.Errorf("persisted outcome = %q, want %q", updated.Outcome, want)
			}
		})
	}
}

// TestSessionGC_StampsGCStaleOutcome: a session reaped by GC is stamped
// "gc-stale" so it is distinguishable in the archive pile.
func TestSessionGC_StampsGCStaleOutcome(t *testing.T) {
	storage := dolt.NewMockAdapter()
	tmux := session.NewMockTmux()
	m := newStoppedManifest("sid-gc", "stale-worker")
	m.CreatedAt = time.Now().Add(-90 * 24 * time.Hour)
	m.UpdatedAt = time.Now().Add(-60 * 24 * time.Hour)
	_ = storage.CreateSession(m)

	ctx := &OpContext{Storage: storage, Tmux: tmux}
	res, err := GC(ctx, &GCRequest{OlderThan: 24 * time.Hour, Force: true})
	if err != nil {
		t.Fatalf("GC failed: %v", err)
	}
	if res.Archived != 1 {
		t.Fatalf("GC archived = %d, want 1 (skipped=%d, errors=%d)", res.Archived, res.Skipped, res.Errors)
	}

	updated, _ := storage.GetSession("sid-gc")
	if updated.Outcome != manifest.OutcomeGCStale {
		t.Errorf("gc-archived outcome = %q, want %q", updated.Outcome, manifest.OutcomeGCStale)
	}
}

// TestSessionList_ShowsOutcomeForArchived: ListSessions surfaces the outcome on
// the summary so the archive view can render it.
func TestSessionList_ShowsOutcomeForArchived(t *testing.T) {
	storage := dolt.NewMockAdapter()
	tmux := session.NewMockTmux()

	archived := newStoppedManifest("sid-arch", "killed-worker")
	archived.Lifecycle = manifest.LifecycleArchived
	archived.Outcome = manifest.OutcomeKilled
	_ = storage.CreateSession(archived)

	ctx := &OpContext{Storage: storage, Tmux: tmux}
	result, err := ListSessions(ctx, &ListSessionsRequest{Status: "all"})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}

	var found *SessionSummary
	for i := range result.Sessions {
		if result.Sessions[i].ID == "sid-arch" {
			found = &result.Sessions[i]
			break
		}
	}
	if found == nil {
		t.Fatal("archived session not returned by ListSessions")
	}
	if found.Status != "archived" {
		t.Errorf("status = %q, want archived", found.Status)
	}
	if found.Outcome != string(manifest.OutcomeKilled) {
		t.Errorf("summary outcome = %q, want %q", found.Outcome, manifest.OutcomeKilled)
	}
}
