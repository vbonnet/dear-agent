package ops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

func TestArchiveOperationContextPropagatesCancellation(t *testing.T) {
	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()
	got := archiveOperationContext(&OpContext{Context: requestCtx})
	select {
	case <-got.Done():
	default:
		t.Fatal("archive operation context lost request cancellation")
	}
	if fallback := archiveOperationContext(&OpContext{}); fallback == nil {
		t.Fatal("archive operation context fallback is nil")
	}
}

func TestArchiveSessionSerializesWithAPIDeliveryMutationLock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage := dolt.NewMockAdapter()
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "api-archive-lock-id",
		Name:          "api-archive-lock",
		Harness:       "openai",
		IsTest:        true,
		CreatedAt:     time.Now().Add(-time.Hour),
		UpdatedAt:     time.Now().Add(-time.Hour),
		Context:       manifest.Context{Project: t.TempDir()},
	}
	if err := storage.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}

	type archiveOutcome struct {
		result *ArchiveSessionResult
		err    error
	}
	archiveStarted := make(chan struct{})
	archiveDone := make(chan archiveOutcome, 1)
	err := WithAPISessionLockContext(t.Context(), m.SessionID, func() error {
		go func() {
			close(archiveStarted)
			result, archiveErr := ArchiveSession(&OpContext{Context: t.Context(), Storage: storage}, &ArchiveSessionRequest{
				Identifier: m.SessionID,
				Force:      true,
			})
			archiveDone <- archiveOutcome{result: result, err: archiveErr}
		}()
		<-archiveStarted
		select {
		case outcome := <-archiveDone:
			return errors.Join(fmt.Errorf("archive crossed the API delivery mutation lock: result=%#v", outcome.result), outcome.err)
		case <-time.After(100 * time.Millisecond):
		}
		current, getErr := storage.GetSession(m.SessionID)
		if getErr != nil {
			return getErr
		}
		if current.Lifecycle == manifest.LifecycleArchived {
			return fmt.Errorf("archive mutated lifecycle before acquiring the shared lock")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("hold API delivery mutation lock: %v", err)
	}
	select {
	case outcome := <-archiveDone:
		if outcome.err != nil {
			t.Fatalf("ArchiveSession() error: %v", outcome.err)
		}
		if outcome.result == nil || outcome.result.SessionID != m.SessionID {
			t.Fatalf("ArchiveSession() result = %#v", outcome.result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("archive did not continue after API delivery mutation lock released")
	}
	current, err := storage.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	if current.Lifecycle != manifest.LifecycleArchived {
		t.Fatalf("archive lifecycle = %q, want archived", current.Lifecycle)
	}
}

type testExternalArchiver func(context.Context, *manifest.Manifest) []ExternalArchiveOutcome

func (f testExternalArchiver) ArchiveExternalSession(ctx context.Context, m *manifest.Manifest) []ExternalArchiveOutcome {
	return f(ctx, m)
}

func TestArchiveSession_IsolatedStoreSkipsHostCleanup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	adapter, err := dolt.NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "isolated-cleanup-id",
		Name:          "isolated-cleanup-session",
		Harness:       "agy",
		CreatedAt:     time.Now().Add(-time.Hour),
		UpdatedAt:     time.Now().Add(-time.Hour),
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "isolated-cleanup-session"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	pendingDir := filepath.Join(home, ".agm", "pending", m.Name)
	if err := os.MkdirAll(pendingDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(pending) error: %v", err)
	}
	marker := filepath.Join(pendingDir, "message.json")
	if err := os.WriteFile(marker, []byte("pending"), 0o600); err != nil {
		t.Fatalf("WriteFile(marker) error: %v", err)
	}

	ctx := &OpContext{
		Storage: adapter,
		ExternalSessionArchiver: testExternalArchiver(func(context.Context, *manifest.Manifest) []ExternalArchiveOutcome {
			return []ExternalArchiveOutcome{{Provider: "test", Status: ExternalArchiveSkipped}}
		}),
	}
	if _, err := ArchiveSession(ctx, &ArchiveSessionRequest{Identifier: m.SessionID, Force: true}); err != nil {
		t.Fatalf("ArchiveSession() error: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("isolated archive removed host pending marker: %v", err)
	}
}

func TestArchiveSession_TestManifestSkipsHostCleanup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	storage := dolt.NewMockAdapter()
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "test-manifest-cleanup-id",
		Name:          "test-manifest-cleanup-session",
		Harness:       "agy",
		IsTest:        true,
		CreatedAt:     time.Now().Add(-time.Hour),
		UpdatedAt:     time.Now().Add(-time.Hour),
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "test-manifest-cleanup-session"},
	}
	if err := storage.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	pendingDir := filepath.Join(home, ".agm", "pending", m.Name)
	if err := os.MkdirAll(pendingDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(pending) error: %v", err)
	}
	marker := filepath.Join(pendingDir, "message.json")
	if err := os.WriteFile(marker, []byte("pending"), 0o600); err != nil {
		t.Fatalf("WriteFile(marker) error: %v", err)
	}

	ctx := &OpContext{
		Storage: storage,
		ExternalSessionArchiver: testExternalArchiver(func(context.Context, *manifest.Manifest) []ExternalArchiveOutcome {
			return []ExternalArchiveOutcome{{Provider: "test", Status: ExternalArchiveSkipped}}
		}),
	}
	if _, err := ArchiveSession(ctx, &ArchiveSessionRequest{Identifier: m.SessionID, Force: true}); err != nil {
		t.Fatalf("ArchiveSession() error: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("test manifest archive removed host pending marker: %v", err)
	}
}

func TestArchiveSession_StoppedSession(t *testing.T) {
	storage := dolt.NewMockAdapter()
	tmux := session.NewMockTmux()

	now := time.Now()
	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "sid-stopped",
		Name:          "stopped-session",
		Harness:       "claude-code",
		CreatedAt:     now.Add(-24 * time.Hour),
		UpdatedAt:     now.Add(-1 * time.Hour),
		Lifecycle:     "",
		State:         "DONE",
		Tmux:          manifest.Tmux{SessionName: "stopped-session"},
		Context:       manifest.Context{Project: "/tmp/test"},
	}
	storage.CreateSession(m)
	// No tmux session — session is stopped

	ctx := &OpContext{Storage: storage, Tmux: tmux}
	result, err := ArchiveSession(ctx, &ArchiveSessionRequest{
		Identifier: "sid-stopped",
		Force:      true,
	})
	if err != nil {
		t.Fatalf("ArchiveSession failed: %v", err)
	}

	if result.PreviousStatus != "stopped" {
		t.Errorf("expected previous status 'stopped', got %q", result.PreviousStatus)
	}

	// Verify lifecycle is now archived
	updated, _ := storage.GetSession("sid-stopped")
	if updated.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("expected lifecycle 'archived', got %q", updated.Lifecycle)
	}
}

func TestArchiveSession_AlreadyArchivedReturnsError(t *testing.T) {
	storage := dolt.NewMockAdapter()
	tmux := session.NewMockTmux()

	now := time.Now()
	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "sid-archived",
		Name:          "archived-session",
		Harness:       "claude-code",
		CreatedAt:     now,
		UpdatedAt:     now,
		Lifecycle:     manifest.LifecycleArchived,
		Tmux:          manifest.Tmux{SessionName: "archived-session"},
	}
	storage.CreateSession(m)

	ctx := &OpContext{Storage: storage, Tmux: tmux}
	_, err := ArchiveSession(ctx, &ArchiveSessionRequest{
		Identifier: "sid-archived",
	})
	if err == nil {
		t.Fatal("expected error for already-archived session")
	}
}

// TestArchiveSession_ReapingSessionCanBeArchived verifies that sessions stuck
// in "reaping" lifecycle (reaper crashed) can be manually archived.
// Regression: sessions stuck as stopped/OFFLINE after reaper failure.
func TestArchiveSession_ReapingSessionCanBeArchived(t *testing.T) {
	storage := dolt.NewMockAdapter()
	tmux := session.NewMockTmux()

	now := time.Now()
	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "sid-reaping",
		Name:          "reaping-session",
		Harness:       "claude-code",
		CreatedAt:     now.Add(-24 * time.Hour),
		UpdatedAt:     now.Add(-1 * time.Hour),
		Lifecycle:     manifest.LifecycleReaping,
		State:         "DONE",
		Tmux:          manifest.Tmux{SessionName: "reaping-session"},
		Context:       manifest.Context{Project: "/tmp/test"},
	}
	storage.CreateSession(m)
	// No tmux session — reaper crashed, session stuck

	ctx := &OpContext{Storage: storage, Tmux: tmux}
	result, err := ArchiveSession(ctx, &ArchiveSessionRequest{
		Identifier: "sid-reaping",
		Force:      true,
	})
	if err != nil {
		t.Fatalf("ArchiveSession failed for reaping session: %v", err)
	}

	if result.PreviousStatus != "stopped" {
		t.Errorf("expected previous status 'stopped', got %q", result.PreviousStatus)
	}

	// Verify lifecycle transitioned to archived
	updated, _ := storage.GetSession("sid-reaping")
	if updated.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("expected lifecycle 'archived', got %q", updated.Lifecycle)
	}
}

// TestListSessions_ArchivedSessionNotShownByDefault verifies that archived
// sessions are hidden from default list view.
// Regression: sessions stuck as stopped/OFFLINE appeared in default list.
func TestListSessions_ArchivedSessionNotShownByDefault(t *testing.T) {
	storage := dolt.NewMockAdapter()
	tmux := session.NewMockTmux()

	now := time.Now()
	// Active session
	active := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "sid-active",
		Name:          "active-session",
		Harness:       "claude-code",
		CreatedAt:     now,
		UpdatedAt:     now,
		Lifecycle:     "",
		Tmux:          manifest.Tmux{SessionName: "active-session"},
	}
	// Archived session
	archived := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "sid-archived",
		Name:          "archived-session",
		Harness:       "claude-code",
		CreatedAt:     now,
		UpdatedAt:     now,
		Lifecycle:     manifest.LifecycleArchived,
		Tmux:          manifest.Tmux{SessionName: "archived-session"},
	}
	storage.CreateSession(active)
	storage.CreateSession(archived)
	tmux.Sessions["active-session"] = true

	ctx := &OpContext{Storage: storage, Tmux: tmux}
	result, err := ListSessions(ctx, &ListSessionsRequest{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}

	if len(result.Sessions) != 1 {
		t.Errorf("expected 1 session in default list, got %d", len(result.Sessions))
	}
	if len(result.Sessions) > 0 && result.Sessions[0].Name != "active-session" {
		t.Errorf("expected active-session, got %s", result.Sessions[0].Name)
	}
}

// TestListSessions_StoppedSessionShowsCorrectStatus verifies the display
// of sessions with dead tmux.
func TestListSessions_StoppedSessionShowsCorrectStatus(t *testing.T) {
	storage := dolt.NewMockAdapter()
	tmux := session.NewMockTmux()

	now := time.Now()
	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "sid-stopped",
		Name:          "stopped-session",
		Harness:       "claude-code",
		CreatedAt:     now,
		UpdatedAt:     now,
		Lifecycle:     "",
		Tmux:          manifest.Tmux{SessionName: "stopped-session"},
	}
	storage.CreateSession(m)
	// No tmux session

	ctx := &OpContext{Storage: storage, Tmux: tmux}
	result, err := ListSessions(ctx, &ListSessionsRequest{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}

	if len(result.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(result.Sessions))
	}
	if result.Sessions[0].Status != "stopped" {
		t.Errorf("expected status 'stopped', got %q", result.Sessions[0].Status)
	}
}

// TestFullExitArchiveFlow_StoppedToArchived simulates the full flow:
// session stops → archive → verify no longer appears as stopped/OFFLINE.
// Regression test for: sessions stuck as stopped/OFFLINE after exit.
func TestFullExitArchiveFlow_StoppedToArchived(t *testing.T) {
	storage := dolt.NewMockAdapter()
	tmux := session.NewMockTmux()

	now := time.Now()
	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "sid-test",
		Name:          "test-session",
		Harness:       "claude-code",
		CreatedAt:     now.Add(-24 * time.Hour),
		UpdatedAt:     now.Add(-1 * time.Hour),
		Lifecycle:     "",
		State:         "DONE",
		Tmux:          manifest.Tmux{SessionName: "test-session"},
		Context:       manifest.Context{Project: "/tmp/test"},
	}
	storage.CreateSession(m)

	ctx := &OpContext{Storage: storage, Tmux: tmux}

	// Step 1: Session is active
	tmux.Sessions["test-session"] = true
	list1, _ := ListSessions(ctx, &ListSessionsRequest{})
	if len(list1.Sessions) != 1 || list1.Sessions[0].Status != "active" {
		t.Fatalf("expected active session, got %v", list1.Sessions)
	}

	// Step 2: Tmux dies (simulates Claude exit)
	delete(tmux.Sessions, "test-session")
	list2, _ := ListSessions(ctx, &ListSessionsRequest{})
	if len(list2.Sessions) != 1 || list2.Sessions[0].Status != "stopped" {
		t.Fatalf("expected stopped session, got %v", list2.Sessions)
	}

	// Step 3: Archive the stopped session
	_, err := ArchiveSession(ctx, &ArchiveSessionRequest{
		Identifier: "sid-test",
		Force:      true,
	})
	if err != nil {
		t.Fatalf("ArchiveSession failed: %v", err)
	}

	// Step 4: Session should NOT appear in default list
	list3, _ := ListSessions(ctx, &ListSessionsRequest{})
	if len(list3.Sessions) != 0 {
		t.Errorf("expected 0 sessions after archive, got %d (status: %s)",
			len(list3.Sessions), list3.Sessions[0].Status)
	}

	// Step 5: Session should appear in "all" list as archived
	list4, _ := ListSessions(ctx, &ListSessionsRequest{Status: "all"})
	if len(list4.Sessions) != 1 {
		t.Fatalf("expected 1 session in 'all' list, got %d", len(list4.Sessions))
	}
	if list4.Sessions[0].Status != "archived" {
		t.Errorf("expected status 'archived' in 'all' list, got %q", list4.Sessions[0].Status)
	}
}

// TestArchiveSession_RecordsTrustEvent verifies that a trust event is
// written to the trust JSONL file after a successful archive. When git is
// not available (as in tests), zero commits are found so false_completion
// is expected.
func TestArchiveSession_RecordsTrustEvent(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()

	storage := dolt.NewMockAdapter()
	tmux := session.NewMockTmux()

	now := time.Now()
	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "sid-trust",
		Name:          "trust-test-session",
		Harness:       "claude-code",
		CreatedAt:     now.Add(-2 * time.Hour),
		UpdatedAt:     now.Add(-10 * time.Minute),
		Lifecycle:     "",
		State:         "DONE",
		Tmux:          manifest.Tmux{SessionName: "trust-test-session"},
		Context:       manifest.Context{Project: "/tmp/nonexistent"},
	}
	storage.CreateSession(m)

	ctx := &OpContext{Storage: storage, Tmux: tmux}
	_, err := ArchiveSession(ctx, &ArchiveSessionRequest{
		Identifier: "sid-trust",
		Force:      true,
	})
	if err != nil {
		t.Fatalf("ArchiveSession failed: %v", err)
	}

	// Trust event must have been recorded (success or false_completion).
	events, readErr := readTrustEvents("trust-test-session")
	if readErr != nil {
		t.Fatalf("readTrustEvents: %v", readErr)
	}
	if len(events) == 0 {
		t.Fatal("expected at least one trust event after archive, got none")
	}
	et := events[0].EventType
	if et != TrustEventSuccess && et != TrustEventFalseCompletion {
		t.Errorf("unexpected trust event type %q; want success or false_completion", et)
	}
}
