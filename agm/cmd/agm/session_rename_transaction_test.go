package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

type recordingSessionIdentityRenamer struct {
	err   error
	calls int
}

func (r *recordingSessionIdentityRenamer) RenameSessionIdentity(context.Context, string, string, string, string, string) error {
	r.calls++
	return r.err
}

func TestPersistRenamedSessionIdentityRollsBackTmuxAfterStorageConflict(t *testing.T) {
	conflictErr := errors.New("session identity changed concurrently")
	store := &recordingSessionIdentityRenamer{err: conflictErr}
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
	store := &recordingSessionIdentityRenamer{err: errors.New("storage conflict")}
	m := &manifest.Manifest{SessionID: "stable-id", Name: "old-name", Tmux: manifest.Tmux{SessionName: "old-tmux"}}
	err := persistRenamedSessionIdentity(t.Context(), store, m, "new-name", true, func(context.Context, string, string) error {
		return errors.New("tmux rollback failed")
	})
	if err == nil || !strings.Contains(err.Error(), "storage conflict") || !strings.Contains(err.Error(), "tmux rollback failed") {
		t.Fatalf("persistRenamedSessionIdentity() error = %v, want joined storage and rollback errors", err)
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
