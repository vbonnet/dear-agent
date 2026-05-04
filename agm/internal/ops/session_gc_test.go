package ops

import (
	"errors"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func gcManifest(id, name, state string, updatedAt time.Time) *manifest.Manifest {
	m := newManifest(id, name, "~/project")
	m.State = state
	m.UpdatedAt = updatedAt
	m.StateUpdatedAt = updatedAt
	return m
}

func TestGC_SkipsActiveTmuxSessions(t *testing.T) {
	now := time.Now()
	old := now.Add(-48 * time.Hour)

	sessions := []*manifest.Manifest{
		gcManifest("id-active", "active-session", manifest.StateDone, old),
		gcManifest("id-stopped", "stopped-session", manifest.StateDone, old),
	}

	// active-session has a tmux session, stopped-session does not
	ctx := testCtx(sessions, "active-session")

	result, err := GC(ctx, &GCRequest{Force: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Archived != 1 {
		t.Errorf("expected 1 archived, got %d", result.Archived)
	}

	// Verify the correct session was archived
	for _, s := range result.Sessions {
		if s.Name == "active-session" && s.Action != "skipped" {
			t.Errorf("active-session should be skipped, got action=%s", s.Action)
		}
		if s.Name == "active-session" && s.Reason != GCSkipActiveTmux {
			t.Errorf("active-session skip reason should be %s, got %s", GCSkipActiveTmux, s.Reason)
		}
		if s.Name == "stopped-session" && s.Action != "archived" {
			t.Errorf("stopped-session should be archived, got action=%s", s.Action)
		}
	}
}

func TestGC_SkipsActiveManifestStates(t *testing.T) {
	now := time.Now()
	old := now.Add(-48 * time.Hour)

	activeStatesForTest := []string{
		manifest.StateWorking,
		manifest.StatePermissionPrompt,
		manifest.StateCompacting,
		manifest.StateWaitingAgent,
		manifest.StateLooping,
		manifest.StateBackgroundTasks,
		manifest.StateUserPrompt,
		manifest.StateReady,
	}

	for _, state := range activeStatesForTest {
		t.Run(state, func(t *testing.T) {
			sessions := []*manifest.Manifest{
				gcManifest("id-active", "busy-session", state, old),
			}
			ctx := testCtx(sessions) // no tmux sessions

			result, err := GC(ctx, &GCRequest{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Archived != 0 {
				t.Errorf("state %s: expected 0 archived, got %d", state, result.Archived)
			}

			for _, s := range result.Sessions {
				if s.Name == "busy-session" {
					if s.Action != "skipped" {
						t.Errorf("state %s: expected skipped, got %s", state, s.Action)
					}
					if s.Reason != GCSkipActiveState {
						t.Errorf("state %s: expected reason %s, got %s", state, GCSkipActiveState, s.Reason)
					}
				}
			}
		})
	}
}

func TestGC_SkipsProtectedRoles(t *testing.T) {
	now := time.Now()
	old := now.Add(-48 * time.Hour)

	tests := []struct {
		name     string
		sessName string
		roles    []string
		wantSkip bool
	}{
		{"default orchestrator", "my-orchestrator", nil, true},
		{"default meta-orchestrator", "meta-orchestrator-prod", nil, true},
		{"default overseer", "overseer-1", nil, true},
		{"custom role match", "scheduler-main", []string{"scheduler"}, true},
		{"no match", "worker-123", nil, false},
		{"case insensitive", "ORCHESTRATOR-test", nil, true},
		{"empty roles disables custom protection", "scheduler-test", []string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessions := []*manifest.Manifest{
				gcManifest("id-1", tt.sessName, manifest.StateDone, old),
			}
			ctx := testCtx(sessions) // no tmux

			result, err := GC(ctx, &GCRequest{ProtectRoles: tt.roles, Force: true})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantSkip {
				if result.Archived != 0 {
					t.Errorf("expected protected session to be skipped, but %d archived", result.Archived)
				}
				for _, s := range result.Sessions {
					if s.Name == tt.sessName && s.Reason != GCSkipProtectedRole {
						t.Errorf("expected reason %s, got %s", GCSkipProtectedRole, s.Reason)
					}
				}
			} else if result.Archived != 1 {
				t.Errorf("expected unprotected session to be archived, got %d archived", result.Archived)
			}
		})
	}
}

func TestGC_SkipsAlreadyArchived(t *testing.T) {
	now := time.Now()
	old := now.Add(-48 * time.Hour)

	sessions := []*manifest.Manifest{
		gcManifest("id-archived", "old-session", manifest.StateDone, old),
	}
	sessions[0].Lifecycle = manifest.LifecycleArchived

	ctx := testCtx(sessions)

	result, err := GC(ctx, &GCRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Archived != 0 {
		t.Errorf("expected 0 archived, got %d", result.Archived)
	}
	for _, s := range result.Sessions {
		if s.Reason != GCSkipAlreadyArchived {
			t.Errorf("expected reason %s, got %s", GCSkipAlreadyArchived, s.Reason)
		}
	}
}

func TestGC_SkipsReapingSessions(t *testing.T) {
	now := time.Now()
	old := now.Add(-48 * time.Hour)

	sessions := []*manifest.Manifest{
		gcManifest("id-reaping", "reaping-session", manifest.StateDone, old),
	}
	sessions[0].Lifecycle = manifest.LifecycleReaping

	ctx := testCtx(sessions)

	result, err := GC(ctx, &GCRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Archived != 0 {
		t.Errorf("expected 0 archived, got %d", result.Archived)
	}
	for _, s := range result.Sessions {
		if s.Reason != GCSkipReaping {
			t.Errorf("expected reason %s, got %s", GCSkipReaping, s.Reason)
		}
	}
}

func TestGC_OlderThanFilter(t *testing.T) {
	now := time.Now()

	sessions := []*manifest.Manifest{
		gcManifest("id-old", "old-session", manifest.StateDone, now.Add(-10*24*time.Hour)),
		gcManifest("id-new", "new-session", manifest.StateDone, now.Add(-1*time.Hour)),
	}

	ctx := testCtx(sessions)

	result, err := GC(ctx, &GCRequest{
		OlderThan: 7 * 24 * time.Hour, // 7 days
		Force:     true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Archived != 1 {
		t.Errorf("expected 1 archived, got %d", result.Archived)
	}

	for _, s := range result.Sessions {
		if s.Name == "new-session" && s.Action != "skipped" {
			t.Errorf("new-session should be skipped (too recent)")
		}
		if s.Name == "new-session" && s.Reason != GCSkipTooRecent {
			t.Errorf("new-session reason should be %s, got %s", GCSkipTooRecent, s.Reason)
		}
		if s.Name == "old-session" && s.Action != "archived" {
			t.Errorf("old-session should be archived, got %s", s.Action)
		}
	}
}

func TestGC_DryRunDoesNotArchive(t *testing.T) {
	now := time.Now()
	old := now.Add(-48 * time.Hour)

	sessions := []*manifest.Manifest{
		gcManifest("id-1", "eligible", manifest.StateDone, old),
	}

	ctx := testCtx(sessions)
	ctx.DryRun = true

	result, err := GC(ctx, &GCRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.DryRun {
		t.Error("expected DryRun=true in result")
	}
	if result.Archived != 1 {
		t.Errorf("expected 1 archived (dry run), got %d", result.Archived)
	}

	// Verify the session was NOT actually archived in storage
	m, err := ctx.Storage.GetSession("id-1")
	if err != nil {
		t.Fatalf("unexpected error fetching session: %v", err)
	}
	if m.Lifecycle == manifest.LifecycleArchived {
		t.Error("session should NOT be archived in dry-run mode")
	}
}

func TestGC_ActuallyArchivesSessions(t *testing.T) {
	now := time.Now()
	old := now.Add(-48 * time.Hour)

	sessions := []*manifest.Manifest{
		gcManifest("id-1", "eligible", manifest.StateDone, old),
	}

	ctx := testCtx(sessions)

	result, err := GC(ctx, &GCRequest{Force: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Archived != 1 {
		t.Errorf("expected 1 archived, got %d", result.Archived)
	}

	// Verify the session was actually archived in storage
	m, err := ctx.Storage.GetSession("id-1")
	if err != nil {
		t.Fatalf("unexpected error fetching session: %v", err)
	}
	if m.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("session should be archived, got lifecycle=%s", m.Lifecycle)
	}
}

func TestGC_HealthCheckFailure(t *testing.T) {
	ctx := &OpContext{
		Storage: newFailingStorage(),
		Tmux:    newMockTmux(),
	}

	_, err := GC(ctx, &GCRequest{})
	if err == nil {
		t.Fatal("expected error on health check failure")
	}

	var opErr *OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("expected *OpError, got %T", err)
	}
	if opErr.Code != ErrCodeStorageError {
		t.Errorf("expected code %s, got %s", ErrCodeStorageError, opErr.Code)
	}
}

func TestGC_NilTmux(t *testing.T) {
	now := time.Now()
	old := now.Add(-48 * time.Hour)

	sessions := []*manifest.Manifest{
		gcManifest("id-1", "some-session", manifest.StateDone, old),
	}

	// Create context with nil tmux — all sessions treated as stopped
	mock := dolt.NewMockAdapter()
	for _, s := range sessions {
		_ = mock.CreateSession(s)
	}
	ctx := &OpContext{
		Storage: mock,
		Tmux:    nil,
	}

	result, err := GC(ctx, &GCRequest{Force: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Archived != 1 {
		t.Errorf("expected 1 archived with nil tmux, got %d", result.Archived)
	}
}

func TestGC_EmptySessionList(t *testing.T) {
	ctx := testCtx(nil)

	result, err := GC(ctx, &GCRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Scanned != 0 {
		t.Errorf("expected 0 scanned, got %d", result.Scanned)
	}
	if result.Archived != 0 {
		t.Errorf("expected 0 archived, got %d", result.Archived)
	}
}

func TestGC_CombinedSafety(t *testing.T) {
	now := time.Now()
	old := now.Add(-48 * time.Hour)

	sessions := []*manifest.Manifest{
		gcManifest("id-1", "orchestrator-main", manifest.StateDone, old),  // protected role
		gcManifest("id-2", "active-worker", manifest.StateDone, old),     // active tmux
		gcManifest("id-3", "busy-worker", manifest.StateWorking, old),    // active state
		gcManifest("id-4", "new-worker", manifest.StateDone, now),        // too recent (with filter)
		gcManifest("id-5", "eligible-worker", manifest.StateDone, old),   // should be GC'd
		gcManifest("id-6", "also-eligible", manifest.StateOffline, old),  // should be GC'd
	}

	ctx := testCtx(sessions, "active-worker")
	ctx.DryRun = true

	result, err := GC(ctx, &GCRequest{
		OlderThan: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Scanned != 6 {
		t.Errorf("expected 6 scanned, got %d", result.Scanned)
	}
	if result.Archived != 2 {
		t.Errorf("expected 2 archived, got %d (eligible-worker + also-eligible)", result.Archived)
	}

	// Verify skip reasons
	reasons := make(map[string]GCSkipReason)
	for _, s := range result.Sessions {
		if s.Action == "skipped" {
			reasons[s.Name] = s.Reason
		}
	}

	if reasons["orchestrator-main"] != GCSkipProtectedRole {
		t.Errorf("orchestrator-main: expected %s, got %s", GCSkipProtectedRole, reasons["orchestrator-main"])
	}
	if reasons["active-worker"] != GCSkipActiveTmux {
		t.Errorf("active-worker: expected %s, got %s", GCSkipActiveTmux, reasons["active-worker"])
	}
	if reasons["busy-worker"] != GCSkipActiveState {
		t.Errorf("busy-worker: expected %s, got %s", GCSkipActiveState, reasons["busy-worker"])
	}
	if reasons["new-worker"] != GCSkipTooRecent {
		t.Errorf("new-worker: expected %s, got %s", GCSkipTooRecent, reasons["new-worker"])
	}
}

func TestGC_Default24hThresholdArchivesOldSessions(t *testing.T) {
	now := time.Now()

	sessions := []*manifest.Manifest{
		gcManifest("id-old", "stale-session", manifest.StateDone, now.Add(-48*time.Hour)),  // 2 days old — should be archived
		gcManifest("id-new", "recent-session", manifest.StateDone, now.Add(-2*time.Hour)),  // 2 hours old — too recent
		gcManifest("id-edge", "edge-session", manifest.StateDone, now.Add(-25*time.Hour)),  // 25 hours old — should be archived
	}

	ctx := testCtx(sessions) // no tmux sessions

	result, err := GC(ctx, &GCRequest{
		OlderThan: 24 * time.Hour, // Simulates the default CLI behavior
		Force:     true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Archived != 2 {
		t.Errorf("expected 2 archived (stale + edge), got %d", result.Archived)
	}
	if result.Skipped != 1 {
		t.Errorf("expected 1 skipped (recent), got %d", result.Skipped)
	}

	for _, s := range result.Sessions {
		switch s.Name {
		case "stale-session", "edge-session":
			if s.Action != "archived" {
				t.Errorf("%s should be archived, got action=%s", s.Name, s.Action)
			}
		case "recent-session":
			if s.Action != "skipped" || s.Reason != GCSkipTooRecent {
				t.Errorf("recent-session should be skipped (too_recent), got action=%s reason=%s", s.Action, s.Reason)
			}
		}
	}
}

func TestGC_ZeroOlderThanArchivesAll(t *testing.T) {
	now := time.Now()

	sessions := []*manifest.Manifest{
		gcManifest("id-1", "very-recent", manifest.StateDone, now.Add(-1*time.Minute)),
		gcManifest("id-2", "old-session", manifest.StateDone, now.Add(-72*time.Hour)),
	}

	ctx := testCtx(sessions) // no tmux sessions

	// OlderThan=0 means no age filter — all eligible sessions are archived
	result, err := GC(ctx, &GCRequest{
		OlderThan: 0,
		Force:     true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Archived != 2 {
		t.Errorf("expected 2 archived (no age filter), got %d", result.Archived)
	}
}

func TestMatchesProtectedRole(t *testing.T) {
	tests := []struct {
		name   string
		roles  []string
		expect bool
	}{
		{"exact match", []string{"orchestrator"}, true},
		{"substring match", []string{"orchestrator"}, true},
		{"case insensitive", []string{"ORCHESTRATOR"}, true},
		{"no match", []string{"scheduler"}, false},
		{"empty roles", []string{}, false},
		{"multiple roles, one matches", []string{"scheduler", "orchestrator"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesProtectedRole("my-orchestrator-session", tt.roles)
			if got != tt.expect {
				t.Errorf("matchesProtectedRole(%q, %v) = %v, want %v",
					"my-orchestrator-session", tt.roles, got, tt.expect)
			}
		})
	}
}

// TestGC_RecordsGCArchivedTrustEvent verifies that after a GC pass archives
// a session, a gc_archived trust event is written to the trust JSONL file.
func TestGC_RecordsGCArchivedTrustEvent(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()

	now := time.Now()
	old := now.Add(-48 * time.Hour)

	sessions := []*manifest.Manifest{
		gcManifest("id-gc-trust", "gc-trust-session", manifest.StateDone, old),
	}
	ctx := testCtx(sessions) // no active tmux sessions

	result, err := GC(ctx, &GCRequest{Force: true})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if result.Archived != 1 {
		t.Fatalf("expected 1 archived, got %d", result.Archived)
	}

	// A gc_archived trust event must have been recorded.
	events, readErr := readTrustEvents("gc-trust-session")
	if readErr != nil {
		t.Fatalf("readTrustEvents: %v", readErr)
	}
	found := false
	for _, e := range events {
		if e.EventType == TrustEventGCArchived {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected gc_archived trust event; got events: %v", events)
	}
}

// --- test helpers ---

// failingStorageImpl always returns errors, simulating unreachable storage.
type failingStorageImpl struct{}

func newFailingStorage() *failingStorageImpl { return &failingStorageImpl{} }

func (f *failingStorageImpl) Create(*manifest.Manifest) error                                { return errors.New("unreachable") }
func (f *failingStorageImpl) Get(string) (*manifest.Manifest, error)                         { return nil, errors.New("unreachable") }
func (f *failingStorageImpl) Update(*manifest.Manifest) error                                { return errors.New("unreachable") }
func (f *failingStorageImpl) Delete(string) error                                            { return errors.New("unreachable") }
func (f *failingStorageImpl) List(*manifest.Filter) ([]*manifest.Manifest, error)            { return nil, errors.New("unreachable") }
func (f *failingStorageImpl) Close() error                                                   { return nil }
func (f *failingStorageImpl) ApplyMigrations() error                                         { return nil }
func (f *failingStorageImpl) CreateSession(*manifest.Manifest) error                         { return errors.New("unreachable") }
func (f *failingStorageImpl) GetSession(string) (*manifest.Manifest, error)                  { return nil, errors.New("unreachable") }
func (f *failingStorageImpl) UpdateSession(*manifest.Manifest) error                         { return errors.New("unreachable") }
func (f *failingStorageImpl) DeleteSession(string) error                                     { return errors.New("unreachable") }
func (f *failingStorageImpl) ListSessions(*dolt.SessionFilter) ([]*manifest.Manifest, error) { return nil, errors.New("unreachable") }
