package main

import (
	"errors"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

type reconcileLifecycleStoreStub struct {
	sessions       []*manifest.Manifest
	updated        []*manifest.Manifest
	reactivated    []*manifest.Manifest
	reactivateErr  error
	reactivateDone bool
}

func (s *reconcileLifecycleStoreStub) ListSessions(*dolt.SessionFilter) ([]*manifest.Manifest, error) {
	return s.sessions, nil
}

func (s *reconcileLifecycleStoreStub) UpdateSession(session *manifest.Manifest) error {
	s.updated = append(s.updated, session)
	return nil
}

func (s *reconcileLifecycleStoreStub) ReactivateSession(session *manifest.Manifest) (dolt.ReactivateSessionResult, error) {
	s.reactivated = append(s.reactivated, session)
	return dolt.ReactivateSessionResult{StorageCommitted: s.reactivateDone}, s.reactivateErr
}

func TestSetLifecycleRoutesArchivedReactivationThroughNameAdmission(t *testing.T) {
	archived := &manifest.Manifest{
		SessionID: "archived-session",
		Name:      "reused-name",
		Lifecycle: manifest.LifecycleArchived,
	}
	store := &reconcileLifecycleStoreStub{
		sessions:       []*manifest.Manifest{archived},
		reactivateDone: true,
	}

	if err := setLifecycle(store, archived.SessionID, ""); err != nil {
		t.Fatalf("setLifecycle() error: %v", err)
	}
	if len(store.reactivated) != 1 || store.reactivated[0] != archived {
		t.Fatalf("reactivated sessions = %#v, want archived session", store.reactivated)
	}
	if len(store.updated) != 0 {
		t.Fatalf("direct lifecycle updates = %#v, want none", store.updated)
	}
}

func TestSetLifecyclePropagatesArchivedNameConflict(t *testing.T) {
	archived := &manifest.Manifest{
		SessionID: "archived-session",
		Name:      "reused-name",
		Lifecycle: manifest.LifecycleArchived,
	}
	conflict := &dolt.SessionNameConflictError{Name: archived.Name}
	store := &reconcileLifecycleStoreStub{
		sessions:      []*manifest.Manifest{archived},
		reactivateErr: conflict,
	}

	err := setLifecycle(store, archived.SessionID, "")
	if !errors.Is(err, conflict) {
		t.Fatalf("setLifecycle() error = %v, want name conflict", err)
	}
	if len(store.updated) != 0 {
		t.Fatalf("direct lifecycle updates = %#v, want none", store.updated)
	}
}

func TestSetLifecycleUpdatesNonArchivedLifecycleDirectly(t *testing.T) {
	reaping := &manifest.Manifest{
		SessionID: "reaping-session",
		Name:      "reserved-name",
		Lifecycle: manifest.LifecycleReaping,
	}
	store := &reconcileLifecycleStoreStub{sessions: []*manifest.Manifest{reaping}}

	if err := setLifecycle(store, reaping.SessionID, ""); err != nil {
		t.Fatalf("setLifecycle() error: %v", err)
	}
	if len(store.reactivated) != 0 {
		t.Fatalf("reactivated sessions = %#v, want none", store.reactivated)
	}
	if len(store.updated) != 1 || store.updated[0].Lifecycle != "" {
		t.Fatalf("direct lifecycle updates = %#v, want active reaping session", store.updated)
	}
}
