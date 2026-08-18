package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

type recordingSessionIdentityRenamer struct {
	result dolt.RenameSessionIdentityResult
	err    error
	calls  int
}

func (r *recordingSessionIdentityRenamer) RenameSessionIdentity(context.Context, string, string, string, string, string) (dolt.RenameSessionIdentityResult, error) {
	r.calls++
	return r.result, r.err
}

func TestSessionRenameSerializesWithResumeByStableID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const sessionID = "stable-session-id"
	lockHeld := make(chan struct{})
	release := make(chan struct{})
	lockerDone := make(chan error, 1)
	go func() {
		lockerDone <- ops.WithSessionLock(sessionID, func() error {
			close(lockHeld)
			<-release
			return nil
		})
	}()
	<-lockHeld

	transactionStarted := make(chan struct{})
	renameReachedLock := make(chan struct{})
	renameDone := make(chan error, 1)
	var closeRelease sync.Once
	t.Cleanup(func() {
		closeRelease.Do(func() { close(release) })
	})
	go func() {
		renameDone <- runSessionRenameTransactionWithLock(sessionID, func(gotSessionID string, transaction func() error) error {
			if gotSessionID != sessionID {
				return fmt.Errorf("rename lock key = %q, want %q", gotSessionID, sessionID)
			}
			close(renameReachedLock)
			return ops.WithSessionLock(gotSessionID, transaction)
		}, func() error {
			close(transactionStarted)
			return nil
		})
	}()
	<-renameReachedLock

	select {
	case <-transactionStarted:
		t.Fatal("rename transaction entered while the resume lock was held")
	case <-time.After(100 * time.Millisecond):
	}
	closeRelease.Do(func() { close(release) })
	if err := <-lockerDone; err != nil {
		t.Fatalf("holding session lock: %v", err)
	}
	select {
	case err := <-renameDone:
		if err != nil {
			t.Fatalf("rename transaction after lock release: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("rename transaction did not acquire the released session lock")
	}
}

func TestPersistRenamedSessionIdentityRollsBackTmuxAfterStorageConflict(t *testing.T) {
	conflictErr := errors.New("session identity changed concurrently")
	store := &recordingSessionIdentityRenamer{result: dolt.RenameSessionIdentityResult{TmuxRollbackSafe: true}, err: conflictErr}
	m := &manifest.Manifest{
		SessionID: "stable-id",
		Name:      "old-name",
		Tmux:      manifest.Tmux{SessionName: "old-tmux", SessionRevision: "observed-revision"},
	}
	rollbackCalls := 0
	err := persistRenamedSessionIdentity(t.Context(), store, m, "new-name", true, func(_ context.Context, oldName, newName string) error {
		rollbackCalls++
		if oldName != "new-name" || newName != "old-tmux" {
			t.Fatalf("rollback rename = (%q, %q), want (new-name, old-tmux)", oldName, newName)
		}
		return nil
	})
	if !errors.Is(err, conflictErr) {
		t.Fatalf("persistRenamedSessionIdentity() error = %v, want storage conflict", err)
	}
	if store.calls != 1 || rollbackCalls != 1 {
		t.Fatalf("calls = (store=%d rollback=%d), want (1, 1)", store.calls, rollbackCalls)
	}
	if m.Name != "old-name" || m.Tmux.SessionName != "old-tmux" {
		t.Fatalf("manifest mutated after conflict: name=%q tmux=%q", m.Name, m.Tmux.SessionName)
	}
}

func TestPersistRenamedSessionIdentityJoinsTmuxRollbackFailure(t *testing.T) {
	store := &recordingSessionIdentityRenamer{result: dolt.RenameSessionIdentityResult{TmuxRollbackSafe: true}, err: errors.New("storage conflict")}
	m := &manifest.Manifest{SessionID: "stable-id", Name: "old-name", Tmux: manifest.Tmux{SessionName: "old-tmux"}}
	err := persistRenamedSessionIdentity(t.Context(), store, m, "new-name", true, func(context.Context, string, string) error {
		return errors.New("tmux rollback failed")
	})
	if err == nil || !strings.Contains(err.Error(), "storage conflict") || !strings.Contains(err.Error(), "tmux rollback failed") {
		t.Fatalf("persistRenamedSessionIdentity() error = %v, want joined storage and rollback errors", err)
	}
}

func TestPersistRenamedSessionIdentityRollsBackAfterCallerCancellation(t *testing.T) {
	store := &recordingSessionIdentityRenamer{result: dolt.RenameSessionIdentityResult{TmuxRollbackSafe: true}, err: context.Canceled}
	m := &manifest.Manifest{SessionID: "stable-id", Name: "old-name", Tmux: manifest.Tmux{SessionName: "old-tmux"}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	rollbackCalled := false
	err := persistRenamedSessionIdentity(ctx, store, m, "new-name", true, func(rollbackCtx context.Context, _, _ string) error {
		if rollbackCtx.Err() != nil {
			t.Fatalf("rollback context inherited caller cancellation: %v", rollbackCtx.Err())
		}
		rollbackCalled = true
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("persistRenamedSessionIdentity() error = %v, want context cancellation", err)
	}
	if !rollbackCalled {
		t.Fatal("tmux rollback was not called after caller cancellation")
	}
}

func TestPersistRenamedSessionIdentityMutatesManifestOnlyAfterStorageSuccess(t *testing.T) {
	store := &recordingSessionIdentityRenamer{}
	m := &manifest.Manifest{SessionID: "stable-id", Name: "old-name", Tmux: manifest.Tmux{SessionName: "old-tmux"}}
	err := persistRenamedSessionIdentity(t.Context(), store, m, "new-name", false, func(context.Context, string, string) error {
		t.Fatal("rollback called after successful storage update")
		return nil
	})
	if err != nil {
		t.Fatalf("persistRenamedSessionIdentity() error = %v", err)
	}
	if m.Name != "new-name" || m.Tmux.SessionName != "new-name" {
		t.Fatalf("manifest identity = (%q, %q), want new-name", m.Name, m.Tmux.SessionName)
	}
}

func TestPersistRenamedSessionIdentityPreservesCommittedRenameAfterCleanupFailure(t *testing.T) {
	cleanupErr := errors.New("release target-name reservation")
	store := &recordingSessionIdentityRenamer{
		result: dolt.RenameSessionIdentityResult{StorageCommitted: true},
		err:    cleanupErr,
	}
	m := &manifest.Manifest{SessionID: "stable-id", Name: "old-name", Tmux: manifest.Tmux{SessionName: "old-tmux"}}
	err := persistRenamedSessionIdentity(t.Context(), store, m, "new-name", true, func(context.Context, string, string) error {
		t.Fatal("tmux rollback called after the durable rename committed")
		return nil
	})
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("persistRenamedSessionIdentity() error = %v, want cleanup error", err)
	}
	if m.Name != "new-name" || m.Tmux.SessionName != "new-name" {
		t.Fatalf("manifest identity = (%q, %q), want committed new-name", m.Name, m.Tmux.SessionName)
	}
}

func TestPersistRenamedSessionIdentityPreservesTmuxWhenStorageIsUncertain(t *testing.T) {
	store := &recordingSessionIdentityRenamer{err: errors.New("storage outcome uncertain")}
	m := &manifest.Manifest{SessionID: "stable-id", Name: "old-name", Tmux: manifest.Tmux{SessionName: "old-tmux"}}
	err := persistRenamedSessionIdentity(t.Context(), store, m, "new-name", true, func(context.Context, string, string) error {
		t.Fatal("tmux rollback called without proof that storage stayed unchanged")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "rollback skipped") {
		t.Fatalf("persistRenamedSessionIdentity() error = %v, want uncertain-storage preservation error", err)
	}
}

func TestClassifyTmuxRenameResult(t *testing.T) {
	primary := errors.New("tmux client lost reply")
	tests := []struct {
		name        string
		oldName     string
		newName     string
		currentName string
		owned       bool
		renameErr   error
		wantMoved   bool
		wantError   bool
	}{
		{name: "server applied rename", oldName: "old", newName: "new", currentName: "new", owned: true, renameErr: primary, wantMoved: true},
		{name: "server did not apply rename", oldName: "old", newName: "new", currentName: "old", owned: true, renameErr: primary, wantError: true},
		{name: "same normalized identity remains", oldName: "same", newName: "same", currentName: "same", owned: true, renameErr: primary, wantMoved: true},
		{name: "replacement occupies target", oldName: "old", newName: "new", currentName: "new", renameErr: primary, wantError: true},
		{name: "claimed identity moved elsewhere", oldName: "old", newName: "new", currentName: "other", owned: true, renameErr: primary, wantError: true},
		{name: "success response without identity move", oldName: "old", newName: "new", currentName: "old", owned: true, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := classifyTmuxRenameResult(tmuxRenameOutcome{}, tt.oldName, tt.newName, tt.currentName, tt.owned, tt.renameErr)
			if result.Moved != tt.wantMoved || (err != nil) != tt.wantError {
				t.Fatalf("classifyTmuxRenameResult() = (moved=%v, err=%v), want (moved=%v, error=%v)", result.Moved, err, tt.wantMoved, tt.wantError)
			}
		})
	}
}

func TestMoveAndRestoreTmuxSessionForRenamePreservesClaimedIdentity(t *testing.T) {
	requireCodexResumeTmuxIntegration(t)
	socketPath := setupRegressionSocket(t)
	if err := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", "rename-source").Run(); err != nil {
		t.Fatalf("create source session: %v", err)
	}
	outcome, err := moveTmuxSessionForRename(t.Context(), "rename-source", "rename-target")
	if err != nil || !outcome.Moved || !outcome.Identity.Valid() {
		t.Fatalf("moveTmuxSessionForRename() = (outcome=%+v err=%v)", outcome, err)
	}
	t.Cleanup(func() {
		_ = tmux.ClearSessionRenameIdentityContext(context.Background(), outcome.Identity)
		tmux.KillSession("rename-source")
		tmux.KillSession("rename-target")
	})
	if err := restoreTmuxSessionNameAfterRename(t.Context(), outcome.Identity, "rename-target", "rename-source"); err != nil {
		t.Fatalf("restoreTmuxSessionNameAfterRename() error: %v", err)
	}
	name, owned, err := tmux.InspectSessionRenameIdentityContext(t.Context(), outcome.Identity)
	if err != nil || !owned || name != "rename-source" {
		t.Fatalf("restored claimed identity = (name=%q owned=%v err=%v)", name, owned, err)
	}
}
