package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

func TestReconcileLifecycleMismatchSerializesWithArchiveCleanup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage := dolt.NewMockAdapter()
	sessionID := "reconcile-archive-lock-id"
	if err := storage.CreateSession(&manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     sessionID,
		Name:          "reconcile-archive-lock",
		Lifecycle:     manifest.LifecycleArchived,
		Tmux:          manifest.Tmux{SessionName: "reconcile-archive-lock"},
		CreatedAt:     time.Now().Add(-time.Hour),
		UpdatedAt:     time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}

	type result struct {
		changed bool
		err     error
	}
	reconcileDone := make(chan result, 1)
	err := ops.WithSessionLock(sessionID, func() error {
		go func() {
			changed, reconcileErr := reconcileLifecycleMismatchWithLocker(
				storage,
				mismatch{Kind: "zombie", SessionID: sessionID},
				ops.WithSessionLock,
				func(string) (bool, error) { return true, nil },
			)
			reconcileDone <- result{changed: changed, err: reconcileErr}
		}()
		select {
		case early := <-reconcileDone:
			return fmt.Errorf(
				"reconcile crossed archive cleanup lock: changed=%t err=%v",
				early.changed,
				early.err,
			)
		case <-time.After(100 * time.Millisecond):
		}
		current, getErr := storage.GetSession(sessionID)
		if getErr != nil {
			return getErr
		}
		if current.Lifecycle != manifest.LifecycleArchived {
			return fmt.Errorf(
				"lifecycle changed while archive cleanup lock was held: %q",
				current.Lifecycle,
			)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("hold archive cleanup lock: %v", err)
	}

	select {
	case got := <-reconcileDone:
		if got.err != nil {
			t.Fatalf("reconcileLifecycleMismatchWithLocker() error: %v", got.err)
		}
		if !got.changed {
			t.Fatal("reconcileLifecycleMismatchWithLocker() changed = false, want true")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reconcile did not continue after archive cleanup lock released")
	}
	current, err := storage.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	if current.Lifecycle != "" {
		t.Fatalf("reconciled lifecycle = %q, want active", current.Lifecycle)
	}
}

func TestReconcileLifecycleMismatchRevalidatesTmuxUnderLock(t *testing.T) {
	tests := []struct {
		name          string
		kind          string
		lifecycle     string
		tmuxExists    bool
		tmuxErr       error
		wantErr       bool
		wantChanged   bool
		wantLifecycle string
	}{
		{
			name:          "zombie remains live",
			kind:          "zombie",
			lifecycle:     manifest.LifecycleArchived,
			tmuxExists:    true,
			wantChanged:   true,
			wantLifecycle: "",
		},
		{
			name:          "zombie disappeared while waiting",
			kind:          "zombie",
			lifecycle:     manifest.LifecycleArchived,
			tmuxExists:    false,
			wantChanged:   false,
			wantLifecycle: manifest.LifecycleArchived,
		},
		{
			name:          "zombie tmux state is unknown",
			kind:          "zombie",
			lifecycle:     manifest.LifecycleArchived,
			tmuxErr:       fmt.Errorf("tmux unavailable"),
			wantErr:       true,
			wantChanged:   false,
			wantLifecycle: manifest.LifecycleArchived,
		},
		{
			name:          "zombie already reactivated",
			kind:          "zombie",
			lifecycle:     "",
			tmuxExists:    true,
			wantChanged:   false,
			wantLifecycle: "",
		},
		{
			name:          "orphan remains absent",
			kind:          "orphan",
			lifecycle:     "",
			tmuxExists:    false,
			wantChanged:   true,
			wantLifecycle: manifest.LifecycleArchived,
		},
		{
			name:          "orphan restarted while waiting",
			kind:          "orphan",
			lifecycle:     "",
			tmuxExists:    true,
			wantChanged:   false,
			wantLifecycle: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := dolt.NewMockAdapter()
			sessionID := "reconcile-revalidate-" + tt.kind
			if err := storage.CreateSession(&manifest.Manifest{
				SchemaVersion: manifest.SchemaVersion,
				SessionID:     sessionID,
				Name:          sessionID,
				Lifecycle:     tt.lifecycle,
				Tmux:          manifest.Tmux{SessionName: sessionID},
			}); err != nil {
				t.Fatalf("CreateSession() error: %v", err)
			}

			changed, err := reconcileLifecycleMismatchWithLocker(
				storage,
				mismatch{Kind: tt.kind, SessionID: sessionID},
				func(gotSessionID string, transaction func() error) error {
					if gotSessionID != sessionID {
						return fmt.Errorf(
							"locked session ID = %q, want %q",
							gotSessionID,
							sessionID,
						)
					}
					return transaction()
				},
				func(gotTmuxName string) (bool, error) {
					if gotTmuxName != sessionID {
						return false, fmt.Errorf(
							"checked tmux name = %q, want %q",
							gotTmuxName,
							sessionID,
						)
					}
					return tt.tmuxExists, tt.tmuxErr
				},
			)
			if tt.wantErr && err == nil {
				t.Fatal("reconcileLifecycleMismatchWithLocker() error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("reconcileLifecycleMismatchWithLocker() error: %v", err)
			}
			if changed != tt.wantChanged {
				t.Fatalf("changed = %t, want %t", changed, tt.wantChanged)
			}
			current, err := storage.GetSession(sessionID)
			if err != nil {
				t.Fatalf("GetSession() error: %v", err)
			}
			if current.Lifecycle != tt.wantLifecycle {
				t.Fatalf(
					"lifecycle = %q, want %q",
					current.Lifecycle,
					tt.wantLifecycle,
				)
			}
		})
	}
}
