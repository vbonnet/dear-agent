package profile

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// fixedClock returns a Store whose clock advances by one second per call,
// starting from base, so timestamps are deterministic yet monotonic.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore(t.TempDir())
	var tick int64
	s.now = func() time.Time {
		tick++
		return time.Unix(1_700_000_000+tick, 0).UTC()
	}
	return s
}

func TestNewStoreDefaultPath(t *testing.T) {
	s := NewStore("")
	if s.baseDir == "" {
		t.Fatal("expected a default baseDir")
	}
	if filepath.Base(s.baseDir) != "profiles" {
		t.Fatalf("expected default to end in profiles, got %q", s.baseDir)
	}
}

func TestSaveStaticIsImmutable(t *testing.T) {
	s := newTestStore(t)
	p := StaticProfile{AgentID: "ce-ywii", Role: "implementer"}

	if err := s.SaveStatic(p); err != nil {
		t.Fatalf("first SaveStatic: %v", err)
	}
	// SpawnedAt should have been stamped.
	got, err := s.LoadStatic("ce-ywii")
	if err != nil {
		t.Fatalf("LoadStatic: %v", err)
	}
	if got.SpawnedAt.IsZero() {
		t.Fatal("SpawnedAt was not stamped")
	}
	if got.Role != "implementer" {
		t.Fatalf("role not persisted, got %q", got.Role)
	}

	// Second write must be rejected and must not mutate the stored profile.
	p2 := StaticProfile{AgentID: "ce-ywii", Role: "TAMPERED"}
	err = s.SaveStatic(p2)
	if !errors.Is(err, ErrStaticExists) {
		t.Fatalf("expected ErrStaticExists, got %v", err)
	}
	reloaded, err := s.LoadStatic("ce-ywii")
	if err != nil {
		t.Fatalf("LoadStatic after rejected overwrite: %v", err)
	}
	if reloaded.Role != "implementer" {
		t.Fatalf("static profile was mutated: role=%q", reloaded.Role)
	}
}

func TestSaveStaticPreservesExplicitSpawnedAt(t *testing.T) {
	s := newTestStore(t)
	want := time.Unix(42, 0).UTC()
	if err := s.SaveStatic(StaticProfile{AgentID: "a", SpawnedAt: want}); err != nil {
		t.Fatalf("SaveStatic: %v", err)
	}
	got, err := s.LoadStatic("a")
	if err != nil {
		t.Fatalf("LoadStatic: %v", err)
	}
	if !got.SpawnedAt.Equal(want) {
		t.Fatalf("SpawnedAt overwritten: got %v want %v", got.SpawnedAt, want)
	}
}

func TestLoadStaticNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.LoadStatic("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestHasStatic(t *testing.T) {
	s := newTestStore(t)
	if s.HasStatic("a") {
		t.Fatal("HasStatic true before save")
	}
	if err := s.SaveStatic(StaticProfile{AgentID: "a"}); err != nil {
		t.Fatalf("SaveStatic: %v", err)
	}
	if !s.HasStatic("a") {
		t.Fatal("HasStatic false after save")
	}
}

func TestSaveAndLoadDynamic(t *testing.T) {
	s := newTestStore(t)
	d := DynamicProfile{AgentID: "a", SessionID: "s1", TaskFocus: "wiring"}
	if err := s.SaveDynamic(d); err != nil {
		t.Fatalf("SaveDynamic: %v", err)
	}
	got, err := s.LoadDynamic("a", "s1")
	if err != nil {
		t.Fatalf("LoadDynamic: %v", err)
	}
	if got.TaskFocus != "wiring" {
		t.Fatalf("TaskFocus not persisted: %q", got.TaskFocus)
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt not stamped")
	}
}

func TestSaveDynamicOverwrites(t *testing.T) {
	s := newTestStore(t)
	if err := s.SaveDynamic(DynamicProfile{AgentID: "a", SessionID: "s", TaskFocus: "first"}); err != nil {
		t.Fatalf("SaveDynamic 1: %v", err)
	}
	if err := s.SaveDynamic(DynamicProfile{AgentID: "a", SessionID: "s", TaskFocus: "second"}); err != nil {
		t.Fatalf("SaveDynamic 2: %v", err)
	}
	got, err := s.LoadDynamic("a", "s")
	if err != nil {
		t.Fatalf("LoadDynamic: %v", err)
	}
	if got.TaskFocus != "second" {
		t.Fatalf("dynamic not overwritten: %q", got.TaskFocus)
	}
}

func TestUpdateDynamicCreatesAndMutates(t *testing.T) {
	s := newTestStore(t)
	// Update with no prior profile should create it, keyed correctly.
	got, err := s.UpdateDynamic("a", "s", func(p *DynamicProfile) {
		p.TaskFocus = "step 1"
	})
	if err != nil {
		t.Fatalf("UpdateDynamic create: %v", err)
	}
	if got.AgentID != "a" || got.SessionID != "s" {
		t.Fatalf("identity keys not set: %+v", got)
	}
	first := got.UpdatedAt

	// Second update should see the prior state and refresh UpdatedAt.
	got, err = s.UpdateDynamic("a", "s", func(p *DynamicProfile) {
		if p.TaskFocus != "step 1" {
			t.Fatalf("prior state not loaded: %q", p.TaskFocus)
		}
		p.TaskFocus = "step 2"
	})
	if err != nil {
		t.Fatalf("UpdateDynamic mutate: %v", err)
	}
	if got.TaskFocus != "step 2" {
		t.Fatalf("mutation not applied: %q", got.TaskFocus)
	}
	if !got.UpdatedAt.After(first) {
		t.Fatalf("UpdatedAt not advanced: %v !> %v", got.UpdatedAt, first)
	}
}

func TestRecordDiscovery(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.RecordDiscovery("a", "s", "found the bug", "in store.go"); err != nil {
		t.Fatalf("RecordDiscovery 1: %v", err)
	}
	got, err := s.RecordDiscovery("a", "s", "fixed it", "")
	if err != nil {
		t.Fatalf("RecordDiscovery 2: %v", err)
	}
	if len(got.Discoveries) != 2 {
		t.Fatalf("expected 2 discoveries, got %d", len(got.Discoveries))
	}
	if got.Discoveries[0].Summary != "found the bug" || got.Discoveries[1].Summary != "fixed it" {
		t.Fatalf("discoveries out of order: %+v", got.Discoveries)
	}
	if got.Discoveries[0].Timestamp.IsZero() {
		t.Fatal("discovery timestamp not set")
	}
}

func TestSetTaskFocus(t *testing.T) {
	s := newTestStore(t)
	got, err := s.SetTaskFocus("a", "s", "refactor")
	if err != nil {
		t.Fatalf("SetTaskFocus: %v", err)
	}
	if got.TaskFocus != "refactor" {
		t.Fatalf("focus not set: %q", got.TaskFocus)
	}
}

func TestLoadProfileHandoff(t *testing.T) {
	s := newTestStore(t)
	if err := s.SaveStatic(StaticProfile{AgentID: "a", Role: "impl"}); err != nil {
		t.Fatalf("SaveStatic: %v", err)
	}
	if _, err := s.RecordDiscovery("a", "s1", "thing", ""); err != nil {
		t.Fatalf("RecordDiscovery: %v", err)
	}

	prof, err := s.LoadProfile("a", "s1")
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if prof.Static.Role != "impl" {
		t.Fatalf("static not loaded: %+v", prof.Static)
	}
	if len(prof.Dynamic.Discoveries) != 1 {
		t.Fatalf("dynamic not loaded: %+v", prof.Dynamic)
	}
}

func TestLoadProfileMissingDynamicIsEmptyNotError(t *testing.T) {
	s := newTestStore(t)
	if err := s.SaveStatic(StaticProfile{AgentID: "a"}); err != nil {
		t.Fatalf("SaveStatic: %v", err)
	}
	prof, err := s.LoadProfile("a", "new-session")
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if prof.Dynamic.SessionID != "new-session" || prof.Dynamic.AgentID != "a" {
		t.Fatalf("empty dynamic not keyed: %+v", prof.Dynamic)
	}
	if len(prof.Dynamic.Discoveries) != 0 {
		t.Fatal("expected no discoveries")
	}
}

func TestLoadProfileMissingStaticErrors(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.LoadProfile("ghost", "s"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListSessions(t *testing.T) {
	s := newTestStore(t)
	if got, err := s.ListSessions("a"); err != nil || len(got) != 0 {
		t.Fatalf("expected empty, got %v err=%v", got, err)
	}
	for _, sess := range []string{"s3", "s1", "s2"} {
		if _, err := s.SetTaskFocus("a", sess, "x"); err != nil {
			t.Fatalf("SetTaskFocus %s: %v", sess, err)
		}
	}
	// A static profile in the same dir must not be counted as a session.
	if err := s.SaveStatic(StaticProfile{AgentID: "a"}); err != nil {
		t.Fatalf("SaveStatic: %v", err)
	}
	got, err := s.ListSessions("a")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	want := []string{"s1", "s2", "s3"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sorted sessions mismatch: got %v want %v", got, want)
		}
	}
}

func TestInvalidIDsRejectedAtStore(t *testing.T) {
	s := newTestStore(t)
	if err := s.SaveStatic(StaticProfile{AgentID: "../evil"}); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("SaveStatic bad id: %v", err)
	}
	if _, err := s.LoadStatic("../evil"); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("LoadStatic bad id: %v", err)
	}
	if _, err := s.LoadDynamic("a", "../evil"); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("LoadDynamic bad session: %v", err)
	}
	if _, err := s.UpdateDynamic("a", "..", nil); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("UpdateDynamic bad session: %v", err)
	}
}

func TestUpdateDynamicNilMutateStillPersists(t *testing.T) {
	s := newTestStore(t)
	got, err := s.UpdateDynamic("a", "s", nil)
	if err != nil {
		t.Fatalf("UpdateDynamic nil mutate: %v", err)
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt not stamped")
	}
	if _, err := s.LoadDynamic("a", "s"); err != nil {
		t.Fatalf("profile not persisted: %v", err)
	}
}
