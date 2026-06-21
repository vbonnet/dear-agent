package ops

import (
	"os/exec"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	inttmux "github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// TestGC_NameCollisionDoesNotArchiveLiveWorker is a regression test for ce-cxjb.1.
//
// Two manifests share the same Name ("dup-worker") but have distinct SessionIDs:
//   - a LIVE worker with an empty Tmux.SessionName (so its tmux pane is matched
//     via the sanitized-Name fallback), and
//   - a STOPPED shadow whose explicit tmux name does not exist.
//
// The shadow is ordered AFTER the live worker so that the old Name-keyed status
// map ("dup-worker" → "stopped") would overwrite the live worker's "active"
// entry, leaving the live worker unprotected and GC-eligible — exactly the bug
// that archived a live worker. Keying the active-tmux set by SessionID (UUID)
// gives each session a distinct entry, so the collision can no longer shadow a
// live session.
func TestGC_NameCollisionDoesNotArchiveLiveWorker(t *testing.T) {
	old := time.Now().Add(-48 * time.Hour)

	live := newManifest("id-live", "dup-worker", "~/project")
	live.Tmux.SessionName = "" // forces the sanitized-Name fallback ("dup-worker")
	live.State = manifest.StateDone
	live.UpdatedAt = old
	live.StateUpdatedAt = old

	shadow := newManifest("id-shadow", "dup-worker", "~/project")
	shadow.Tmux.SessionName = "nonexistent-pane" // does not exist in tmux
	shadow.State = manifest.StateDone
	shadow.UpdatedAt = old
	shadow.StateUpdatedAt = old

	// Live worker comes first, shadow second — the order that triggered the
	// last-writer-wins shadowing under the old Name-keyed map.
	sessions := []*manifest.Manifest{live, shadow}

	// Only the live worker's fallback tmux pane ("dup-worker") actually exists.
	ctx := testCtx(sessions, "dup-worker")

	result, err := GC(ctx, &GCRequest{Force: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var liveEntry *GCSessionEntry
	for i := range result.Sessions {
		if result.Sessions[i].SessionID == "id-live" {
			liveEntry = &result.Sessions[i]
		}
	}
	if liveEntry == nil {
		t.Fatal("live worker missing from GC result")
	}
	if liveEntry.Action != "skipped" {
		t.Errorf("live worker should be skipped, got action=%s", liveEntry.Action)
	}
	if liveEntry.Reason != GCSkipActiveTmux {
		t.Errorf("live worker skip reason should be %s, got %s", GCSkipActiveTmux, liveEntry.Reason)
	}

	// The live worker must not have been archived in storage.
	m, err := ctx.Storage.GetSession("id-live")
	if err != nil {
		t.Fatalf("fetching live worker: %v", err)
	}
	if m.Lifecycle == manifest.LifecycleArchived {
		t.Error("live worker was archived despite an active tmux pane")
	}
}

// TestCheckActiveTmuxBlock_EmptySessionNameFallback is a regression test for
// ce-cxjb.1 fix (b). A live detached pane whose manifest has an empty
// Tmux.SessionName must still block archive: checkActiveTmuxBlock has to resolve
// the tmux name through the same sanitized-Name fallback that status computation
// uses, rather than returning nil the moment the explicit field is empty.
//
// Requires a real tmux binary; skipped otherwise.
func TestCheckActiveTmuxBlock_EmptySessionNameFallback(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	const paneName = "agm-cecxjb1-livepane"
	socketPath := inttmux.GetSocketPath()
	// Best-effort cleanup of any leftover from a previous run.
	_ = exec.Command("tmux", "-S", socketPath, "kill-session", "-t", paneName).Run()
	if err := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", paneName, "sleep", "30").Run(); err != nil {
		t.Skipf("could not create tmux session on AGM socket %s (no server?): %v", socketPath, err)
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-S", socketPath, "kill-session", "-t", paneName).Run()
	})

	// Manifest models a live worker that never recorded its explicit tmux name.
	m := newManifest("id-live-pane", paneName, "~/project")
	m.Tmux.SessionName = "" // the bypass condition

	// Sanity: the fallback resolves to the live pane's actual name.
	if got := session.TmuxSessionName(m); got != paneName {
		t.Fatalf("TmuxSessionName fallback = %q, want %q", got, paneName)
	}

	// Without force, the active pane must block archive.
	if err := checkActiveTmuxBlock(m, false); err == nil {
		t.Error("checkActiveTmuxBlock returned nil for a live pane with empty Tmux.SessionName; expected a block")
	}
	// Force still overrides the block.
	if err := checkActiveTmuxBlock(m, true); err != nil {
		t.Errorf("checkActiveTmuxBlock with force=true should return nil, got %v", err)
	}

	// And the session must not be GC-eligible: status computed via the real tmux
	// (by UUID) marks it active, so gcSkipReason reports GCSkipActiveTmux.
	statuses := session.ComputeStatusBatchByID([]*manifest.Manifest{m}, session.NewRealTmux())
	activeTmuxByID := map[string]bool{}
	for id, status := range statuses {
		if status == "active" {
			activeTmuxByID[id] = true
		}
	}
	reason, skipped := gcSkipReason(m, nil, activeTmuxByID, 0, time.Now())
	if !skipped || reason != GCSkipActiveTmux {
		t.Errorf("live pane should be GC-skipped with %s, got skipped=%v reason=%s", GCSkipActiveTmux, skipped, reason)
	}
}
