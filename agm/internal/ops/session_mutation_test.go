package ops

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// --- ArchiveSession tests ---

func TestArchiveSession_Success(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "my-session", "~/project"),
	}
	ctx := testCtx(sessions, "my-session")

	result, err := ArchiveSession(ctx, &ArchiveSessionRequest{Identifier: "id-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Operation != "archive_session" {
		t.Errorf("expected operation archive_session, got %s", result.Operation)
	}
	if result.SessionID != "id-1" {
		t.Errorf("expected session ID id-1, got %s", result.SessionID)
	}
	if result.Name != "my-session" {
		t.Errorf("expected name my-session, got %s", result.Name)
	}
	if result.PreviousStatus != "active" {
		t.Errorf("expected previous status active, got %s", result.PreviousStatus)
	}
	if result.DryRun {
		t.Error("expected DryRun=false")
	}

	// Verify session is now archived in storage
	updated, err := ctx.Storage.GetSession("id-1")
	if err != nil {
		t.Fatalf("failed to get updated session: %v", err)
	}
	if updated.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("expected lifecycle archived, got %s", updated.Lifecycle)
	}
}

func TestArchiveSession_ByName(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "my-session", "~/project"),
	}
	ctx := testCtx(sessions)

	result, err := ArchiveSession(ctx, &ArchiveSessionRequest{Identifier: "my-session"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SessionID != "id-1" {
		t.Errorf("expected session ID id-1, got %s", result.SessionID)
	}
	if result.PreviousStatus != "stopped" {
		t.Errorf("expected previous status stopped, got %s", result.PreviousStatus)
	}
}

func TestArchiveSession_AlreadyArchived(t *testing.T) {
	m := newManifest("id-1", "archived-session", "~/project")
	m.Lifecycle = manifest.LifecycleArchived
	sessions := []*manifest.Manifest{m}
	ctx := testCtx(sessions)

	_, err := ArchiveSession(ctx, &ArchiveSessionRequest{Identifier: "id-1"})
	if err == nil {
		t.Fatal("expected error for already archived session")
	}
	opErr := &OpError{}
	ok := errors.As(err, &opErr)
	if !ok {
		t.Fatalf("expected *OpError, got %T", err)
	}
	if opErr.Code != ErrCodeSessionArchived {
		t.Errorf("expected code %s, got %s", ErrCodeSessionArchived, opErr.Code)
	}
}

func TestArchiveSession_NotFound(t *testing.T) {
	ctx := testCtx(nil)
	_, err := ArchiveSession(ctx, &ArchiveSessionRequest{Identifier: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for missing session")
	}
	opErr := &OpError{}
	ok := errors.As(err, &opErr)
	if !ok {
		t.Fatalf("expected *OpError, got %T", err)
	}
	if opErr.Code != ErrCodeSessionNotFound {
		t.Errorf("expected code %s, got %s", ErrCodeSessionNotFound, opErr.Code)
	}
}

func TestArchiveSession_EmptyIdentifier(t *testing.T) {
	ctx := testCtx(nil)
	_, err := ArchiveSession(ctx, &ArchiveSessionRequest{Identifier: ""})
	if err == nil {
		t.Fatal("expected error for empty identifier")
	}
}

func TestArchiveSession_NilRequest(t *testing.T) {
	ctx := testCtx(nil)
	_, err := ArchiveSession(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil request")
	}
}

func TestArchiveSession_DryRun(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "my-session", "~/project"),
	}
	ctx := testCtx(sessions, "my-session")
	ctx.DryRun = true

	result, err := ArchiveSession(ctx, &ArchiveSessionRequest{Identifier: "id-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.DryRun {
		t.Error("expected DryRun=true")
	}
	if result.PreviousStatus != "active" {
		t.Errorf("expected previous status active, got %s", result.PreviousStatus)
	}

	// Verify session is NOT archived (dry run)
	updated, err := ctx.Storage.GetSession("id-1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if updated.Lifecycle == manifest.LifecycleArchived {
		t.Error("session should NOT be archived in dry run mode")
	}
}

func TestArchiveSession_ForceBypassesVerification(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "my-session", "~/project"),
	}
	ctx := testCtx(sessions)

	// Force=true should always succeed (verification runs but doesn't block)
	result, err := ArchiveSession(ctx, &ArchiveSessionRequest{
		Identifier: "my-session",
		Force:      true,
	})
	if err != nil {
		t.Fatalf("unexpected error with Force=true: %v", err)
	}
	if result.SessionID != "id-1" {
		t.Errorf("expected session ID id-1, got %s", result.SessionID)
	}

	// Verify session is archived
	updated, err := ctx.Storage.GetSession("id-1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if updated.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("expected lifecycle archived, got %s", updated.Lifecycle)
	}
}

// --- KillSession tests ---

type stubbornKillTmux struct {
	*mockTmux
}

type strictProbeKillTmux struct {
	*mockTmux
	strictErr   error
	strictCalls int
}

type observingKillStorage struct {
	dolt.Storage
	firstRead chan struct{}
	once      sync.Once
}

type vanishingKillStorage struct {
	dolt.Storage
	reads int
}

func (s *observingKillStorage) GetSession(sessionID string) (*manifest.Manifest, error) {
	m, err := s.Storage.GetSession(sessionID)
	s.once.Do(func() { close(s.firstRead) })
	return m, err
}

func (s *vanishingKillStorage) GetSession(sessionID string) (*manifest.Manifest, error) {
	s.reads++
	if s.reads > 1 {
		return nil, nil
	}
	return s.Storage.GetSession(sessionID)
}

func (m *stubbornKillTmux) KillSession(name string) error {
	m.killed = append(m.killed, name)
	return nil
}

func (m *strictProbeKillTmux) HasSessionStrict(context.Context, string) (bool, error) {
	m.strictCalls++
	if m.strictErr != nil {
		return false, m.strictErr
	}
	return m.HasSession("my-session")
}

func TestKillSession_RunningSession(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "my-session", "~/project"),
	}
	ctx := testCtx(sessions, "my-session")

	// Running session without --confirmed-stuck should be refused
	_, err := KillSession(ctx, &KillSessionRequest{Identifier: "id-1"})
	if err == nil {
		t.Fatal("expected error when killing running session without --confirmed-stuck")
	}
	var opErr *OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("expected *OpError, got %T", err)
	}
	if opErr.Code != ErrCodeActiveSessionKill {
		t.Errorf("expected code %s, got %s", ErrCodeActiveSessionKill, opErr.Code)
	}

	// With --confirmed-stuck, it should succeed
	result, err := KillSession(ctx, &KillSessionRequest{Identifier: "id-1", ConfirmedStuck: true})
	if err != nil {
		t.Fatalf("unexpected error with --confirmed-stuck: %v", err)
	}
	if result.Operation != "kill_session" {
		t.Errorf("expected operation kill_session, got %s", result.Operation)
	}
	if result.SessionID != "id-1" {
		t.Errorf("expected session ID id-1, got %s", result.SessionID)
	}
	if !result.WasRunning {
		t.Error("expected WasRunning=true for session with active tmux")
	}
	tm := ctx.Tmux.(*mockTmux)
	if tm.sessions["my-session"] {
		t.Error("successful kill left the tmux session present")
	}
	if len(tm.killed) != 1 || tm.killed[0] != "my-session" {
		t.Fatalf("killed targets = %v, want [my-session]", tm.killed)
	}
}

func TestKillSession_StoppedSession(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "my-session", "~/project"),
	}
	ctx := testCtx(sessions) // no tmux sessions

	result, err := KillSession(ctx, &KillSessionRequest{Identifier: "id-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.WasRunning {
		t.Error("expected WasRunning=false for session without active tmux")
	}
	if killed := ctx.Tmux.(*mockTmux).killed; len(killed) != 0 {
		t.Fatalf("absent tmux target triggered kill calls: %v", killed)
	}
}

func TestKillSession_RemovesExactTmuxTarget(t *testing.T) {
	m := newManifest("id-1", "display-name", "~/project")
	m.Tmux.SessionName = "exact-target"
	tm := newMockTmux("exact-target", "exact-target-child")
	ctx := testCtx([]*manifest.Manifest{m})
	ctx.Tmux = tm

	result, err := KillSession(ctx, &KillSessionRequest{Identifier: "display-name", ConfirmedStuck: true})
	if err != nil {
		t.Fatalf("KillSession() error = %v", err)
	}
	if result.TmuxSessionName != "exact-target" {
		t.Fatalf("TmuxSessionName = %q, want exact-target", result.TmuxSessionName)
	}
	if len(tm.killed) != 1 || tm.killed[0] != "exact-target" {
		t.Fatalf("killed targets = %v, want [exact-target]", tm.killed)
	}
	if tm.sessions["exact-target"] {
		t.Error("exact target still exists")
	}
	if !tm.sessions["exact-target-child"] {
		t.Error("prefix-related non-target session was removed")
	}
}

func TestKillSession_ReloadsCurrentTargetUnderStableIDLock(t *testing.T) {
	sessionID := "kill-lock-" + t.Name()
	initial := newManifest(sessionID, "display-name", "~/project")
	initial.Tmux.SessionName = "old-target"
	initial.UpdatedAt = time.Now().Add(-time.Hour)
	store := dolt.NewMockAdapter()
	if err := store.CreateSession(initial); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	observed := &observingKillStorage{Storage: store, firstRead: make(chan struct{})}
	tm := newMockTmux("old-target", "new-target")
	opCtx := &OpContext{Storage: observed, Tmux: tm}

	lockHeld := make(chan struct{})
	releaseLock := make(chan struct{})
	lockDone := make(chan error, 1)
	go func() {
		lockDone <- WithSessionLockTimeout(sessionID, time.Second, func() error {
			close(lockHeld)
			<-releaseLock
			return nil
		})
	}()
	<-lockHeld

	type killOutcome struct {
		result *KillSessionResult
		err    error
	}
	killDone := make(chan killOutcome, 1)
	go func() {
		result, err := KillSession(opCtx, &KillSessionRequest{Identifier: sessionID, ConfirmedStuck: true})
		killDone <- killOutcome{result: result, err: err}
	}()
	<-observed.firstRead

	current, err := store.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	current.Tmux.SessionName = "new-target"
	if err := store.UpdateSession(current); err != nil {
		t.Fatalf("UpdateSession: %v", err)
	}
	close(releaseLock)
	if err := <-lockDone; err != nil {
		t.Fatalf("lock owner: %v", err)
	}

	outcome := <-killDone
	if outcome.err != nil {
		t.Fatalf("KillSession: %v", outcome.err)
	}
	if outcome.result.TmuxSessionName != "new-target" {
		t.Fatalf("resolved tmux target = %q, want reloaded new-target", outcome.result.TmuxSessionName)
	}
	if tm.sessions["new-target"] {
		t.Fatal("reloaded target remains after successful kill")
	}
	if !tm.sessions["old-target"] {
		t.Fatal("stale pre-lock target was killed")
	}
}

func TestKillSession_ConcurrentDeletionReturnsNotFound(t *testing.T) {
	store := dolt.NewMockAdapter()
	if err := store.CreateSession(newManifest("id-1", "my-session", "~/project")); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	tm := newMockTmux("my-session")
	opCtx := &OpContext{
		Storage: &vanishingKillStorage{Storage: store},
		Tmux:    tm,
	}

	result, err := KillSession(opCtx, &KillSessionRequest{Identifier: "id-1", ConfirmedStuck: true})
	if result != nil {
		t.Fatalf("result = %#v, want nil after concurrent deletion", result)
	}
	var opErr *OpError
	if !errors.As(err, &opErr) || opErr.Code != ErrCodeSessionNotFound {
		t.Fatalf("KillSession() error = %v, want %s", err, ErrCodeSessionNotFound)
	}
	if len(tm.killed) != 0 {
		t.Fatalf("concurrent deletion mutated tmux: killed = %v", tm.killed)
	}
}

func TestKillSession_CanceledRequestDoesNotMutateTmux(t *testing.T) {
	tm := newMockTmux("my-session")
	opCtx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")})
	opCtx.Tmux = tm
	requestCtx, cancel := context.WithCancel(t.Context())
	cancel()
	opCtx.Context = requestCtx

	_, err := KillSession(opCtx, &KillSessionRequest{Identifier: "id-1", ConfirmedStuck: true})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("KillSession() error = %v, want context.Canceled", err)
	}
	if len(tm.killed) != 0 || !tm.sessions["my-session"] {
		t.Fatalf("canceled request mutated tmux: killed=%v sessions=%v", tm.killed, tm.sessions)
	}
}

func TestKillSession_CancelStopsContendedStableIDLock(t *testing.T) {
	sessionID := "kill-cancel-lock-" + t.Name()
	store := dolt.NewMockAdapter()
	if err := store.CreateSession(newManifest(sessionID, "my-session", "~/project")); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	observed := &observingKillStorage{Storage: store, firstRead: make(chan struct{})}
	tm := newMockTmux("my-session")
	requestCtx, cancel := context.WithCancel(t.Context())
	opCtx := &OpContext{Context: requestCtx, Storage: observed, Tmux: tm}

	lockHeld := make(chan struct{})
	releaseLock := make(chan struct{})
	lockDone := make(chan error, 1)
	go func() {
		lockDone <- WithSessionLockTimeout(sessionID, time.Second, func() error {
			close(lockHeld)
			<-releaseLock
			return nil
		})
	}()
	<-lockHeld

	done := make(chan error, 1)
	go func() {
		_, err := KillSession(opCtx, &KillSessionRequest{Identifier: sessionID, ConfirmedStuck: true})
		done <- err
	}()
	<-observed.firstRead
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("KillSession() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled KillSession did not stop waiting for the stable-ID lock")
	}
	close(releaseLock)
	if err := <-lockDone; err != nil {
		t.Fatalf("lock owner: %v", err)
	}
	if len(tm.killed) != 0 || !tm.sessions["my-session"] {
		t.Fatalf("canceled lock wait mutated tmux: killed=%v sessions=%v", tm.killed, tm.sessions)
	}
}

func TestKillSession_PropagatesBackendFailure(t *testing.T) {
	wantErr := errors.New("tmux kill denied")
	tm := newMockTmux("my-session")
	tm.killErr = wantErr
	ctx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")})
	ctx.Tmux = tm

	result, err := KillSession(ctx, &KillSessionRequest{Identifier: "id-1", ConfirmedStuck: true})
	if !errors.Is(err, wantErr) {
		t.Fatalf("KillSession() error = %v, want backend failure %v", err, wantErr)
	}
	if result == nil || result.TmuxSessionName != "my-session" {
		t.Fatalf("result = %#v, want resolved target for failed mutation", result)
	}
}

func TestKillSession_FailsWhenTargetRemains(t *testing.T) {
	tm := &stubbornKillTmux{mockTmux: newMockTmux("my-session")}
	ctx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")})
	ctx.Tmux = tm

	_, err := KillSession(ctx, &KillSessionRequest{Identifier: "id-1", ConfirmedStuck: true})
	if err == nil {
		t.Fatal("KillSession() returned success while the exact tmux target remained")
	}
	if !tm.sessions["my-session"] {
		t.Fatal("stubborn backend fixture unexpectedly removed the target")
	}
}

func TestKillSession_PropagatesProbeFailure(t *testing.T) {
	wantErr := errors.New("tmux socket unavailable")
	tm := newMockTmux("my-session")
	tm.hasErr = wantErr
	ctx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")})
	ctx.Tmux = tm

	_, err := KillSession(ctx, &KillSessionRequest{Identifier: "id-1", ConfirmedStuck: true})
	if !errors.Is(err, wantErr) {
		t.Fatalf("KillSession() error = %v, want probe failure %v", err, wantErr)
	}
	if len(tm.killed) != 0 {
		t.Fatalf("probe failure must not mutate tmux, killed = %v", tm.killed)
	}
}

func TestKillSession_UsesStrictProductionProbe(t *testing.T) {
	wantErr := errors.New("tmux socket permission denied")
	tm := &strictProbeKillTmux{mockTmux: newMockTmux("my-session"), strictErr: wantErr}
	ctx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")})
	ctx.Tmux = tm

	_, err := KillSession(ctx, &KillSessionRequest{Identifier: "id-1", ConfirmedStuck: true})
	if !errors.Is(err, wantErr) {
		t.Fatalf("KillSession() error = %v, want strict probe failure %v", err, wantErr)
	}
	if tm.strictCalls == 0 {
		t.Fatal("KillSession did not use the strict existence capability")
	}
	if len(tm.killed) != 0 {
		t.Fatalf("strict probe failure mutated tmux: killed = %v", tm.killed)
	}
}

func TestKillSession_RequiresTmuxBackend(t *testing.T) {
	ctx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")})
	ctx.Tmux = nil

	_, err := KillSession(ctx, &KillSessionRequest{Identifier: "id-1"})
	if err == nil {
		t.Fatal("KillSession() returned success without a tmux backend")
	}
}

func TestKillSession_ArchivedSession(t *testing.T) {
	m := newManifest("id-1", "archived-session", "~/project")
	m.Lifecycle = manifest.LifecycleArchived
	sessions := []*manifest.Manifest{m}
	ctx := testCtx(sessions)

	_, err := KillSession(ctx, &KillSessionRequest{Identifier: "id-1"})
	if err == nil {
		t.Fatal("expected error for archived session")
	}
	opErr := &OpError{}
	ok := errors.As(err, &opErr)
	if !ok {
		t.Fatalf("expected *OpError, got %T", err)
	}
	if opErr.Code != ErrCodeSessionArchived {
		t.Errorf("expected code %s, got %s", ErrCodeSessionArchived, opErr.Code)
	}
}

func TestKillSession_NotFound(t *testing.T) {
	ctx := testCtx(nil)
	_, err := KillSession(ctx, &KillSessionRequest{Identifier: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for missing session")
	}
}

func TestKillSession_EmptyIdentifier(t *testing.T) {
	ctx := testCtx(nil)
	_, err := KillSession(ctx, &KillSessionRequest{Identifier: ""})
	if err == nil {
		t.Fatal("expected error for empty identifier")
	}
}

func TestKillSession_DryRun(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "my-session", "~/project"),
	}
	ctx := testCtx(sessions, "my-session")
	ctx.DryRun = true

	result, err := KillSession(ctx, &KillSessionRequest{Identifier: "id-1", ConfirmedStuck: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.DryRun {
		t.Error("expected DryRun=true")
	}
	if !result.WasRunning {
		t.Error("expected WasRunning=true")
	}
	tm := ctx.Tmux.(*mockTmux)
	if !tm.sessions["my-session"] || len(tm.killed) != 0 {
		t.Fatalf("dry run mutated tmux: sessions=%v killed=%v", tm.sessions, tm.killed)
	}
}

func TestKillSession_ByName(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "my-session", "~/project"),
	}
	ctx := testCtx(sessions, "my-session")

	result, err := KillSession(ctx, &KillSessionRequest{Identifier: "my-session", ConfirmedStuck: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SessionID != "id-1" {
		t.Errorf("expected session ID id-1, got %s", result.SessionID)
	}
}

func TestKillSession_ActiveSession_RequiresConfirmedStuck(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "active-session", "~/project"),
	}
	ctx := testCtx(sessions, "active-session") // tmux running

	_, err := KillSession(ctx, &KillSessionRequest{Identifier: "active-session"})
	if err == nil {
		t.Fatal("expected error when killing active session without --confirmed-stuck")
	}
	var opErr *OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("expected *OpError, got %T", err)
	}
	if opErr.Code != ErrCodeActiveSessionKill {
		t.Errorf("expected code %s, got %s", ErrCodeActiveSessionKill, opErr.Code)
	}
}

func TestKillSession_ActiveSession_WithConfirmedStuck(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "active-session", "~/project"),
	}
	ctx := testCtx(sessions, "active-session") // tmux running

	result, err := KillSession(ctx, &KillSessionRequest{
		Identifier:     "active-session",
		ConfirmedStuck: true,
	})
	if err != nil {
		t.Fatalf("--confirmed-stuck should allow killing active session, got error: %v", err)
	}
	if !result.WasRunning {
		t.Error("expected WasRunning=true")
	}
	if result.Name != "active-session" {
		t.Errorf("expected name active-session, got %s", result.Name)
	}
}

func TestKillSession_StoppedSession_NoFlagNeeded(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "stopped-session", "~/project"),
	}
	ctx := testCtx(sessions) // no tmux sessions — session is stopped

	result, err := KillSession(ctx, &KillSessionRequest{Identifier: "stopped-session"})
	if err != nil {
		t.Fatalf("stopped session should not require --confirmed-stuck, got error: %v", err)
	}
	if result.WasRunning {
		t.Error("expected WasRunning=false for stopped session")
	}
}

func TestKillSession_KillProtect_RecentlyActive(t *testing.T) {
	m := newManifest("id-1", "active-session", "~/project")
	m.UpdatedAt = time.Now().Add(-1 * time.Minute) // active 1 min ago
	sessions := []*manifest.Manifest{m}
	ctx := testCtx(sessions) // no tmux session — stopped but recently active

	// Without any bypass flag, recently active is protected.
	_, err := KillSession(ctx, &KillSessionRequest{Identifier: "active-session"})
	if err == nil {
		t.Fatal("expected kill-protected error for recently active session")
	}
	opErr := &OpError{}
	ok := errors.As(err, &opErr)
	if !ok {
		t.Fatalf("expected OpError, got %T", err)
	}
	if opErr.Code != ErrCodeKillProtected {
		t.Errorf("expected code %s, got %s", ErrCodeKillProtected, opErr.Code)
	}
}

// TestKillSession_ConfirmedStuckAloneSuffices guards against the ce-axsr flag
// ping-pong: --confirmed-stuck demanded --force, and --force demanded
// --confirmed-stuck, so no single re-run command could kill a stuck session.
// One flag path must be sufficient.
func TestKillSession_ConfirmedStuckAloneSuffices(t *testing.T) {
	m := newManifest("id-1", "active-session", "~/project")
	m.UpdatedAt = time.Now().Add(-1 * time.Minute) // recently active
	sessions := []*manifest.Manifest{m}
	ctx := testCtx(sessions, "active-session") // tmux session exists

	result, err := KillSession(ctx, &KillSessionRequest{
		Identifier:     "active-session",
		ConfirmedStuck: true,
	})
	if err != nil {
		t.Fatalf("--confirmed-stuck alone must suffice for an active, recently-active session, got: %v", err)
	}
	if !result.RecentlyActive {
		t.Error("expected RecentlyActive=true")
	}
}

func TestKillSession_KillProtect_ForceBypass(t *testing.T) {
	m := newManifest("id-1", "active-session", "~/project")
	m.UpdatedAt = time.Now().Add(-1 * time.Minute) // active 1 min ago
	sessions := []*manifest.Manifest{m}
	ctx := testCtx(sessions, "active-session")

	result, err := KillSession(ctx, &KillSessionRequest{
		Identifier:     "active-session",
		Force:          true,
		ConfirmedStuck: true,
	})
	if err != nil {
		t.Fatalf("--force should bypass kill-protect, got error: %v", err)
	}
	if !result.RecentlyActive {
		t.Error("expected RecentlyActive=true")
	}
	if result.LastActivity == nil {
		t.Error("expected LastActivity to be set")
	}
}

func TestKillSession_KillProtect_OldSession(t *testing.T) {
	m := newManifest("id-1", "old-session", "~/project")
	m.UpdatedAt = time.Now().Add(-10 * time.Minute) // active 10 min ago
	sessions := []*manifest.Manifest{m}
	ctx := testCtx(sessions, "old-session")

	result, err := KillSession(ctx, &KillSessionRequest{Identifier: "old-session", ConfirmedStuck: true})
	if err != nil {
		t.Fatalf("old session should not be kill-protected: %v", err)
	}
	if result.RecentlyActive {
		t.Error("expected RecentlyActive=false for old session")
	}
}

// --- KillSession harness-liveness tests (ce-axsr) ---

// TestKillSession_ZombiePane_NoConfirmedStuckNeeded: a tmux session that
// exists but whose harness process is dead (pane fell back to zsh) is NOT an
// active session — `tmux has-session` alone is false-green.
func TestKillSession_ZombiePane_NoConfirmedStuckNeeded(t *testing.T) {
	m := newManifest("id-1", "zombie-session", "~/project")
	m.UpdatedAt = time.Now().Add(-10 * time.Minute) // not recently active
	tm := &mockTmuxWithLiveness{
		mockTmux: newMockTmux("zombie-session"),
		liveness: map[string]session.LivenessInfo{
			"zombie-session": {SessionExists: true, HarnessAlive: false, ZombieWriter: true, Evidence: "zsh,agm"},
		},
	}
	ctx := testCtxWithLiveness([]*manifest.Manifest{m}, tm)

	result, err := KillSession(ctx, &KillSessionRequest{Identifier: "zombie-session"})
	if err != nil {
		t.Fatalf("zombie pane must be killable without --confirmed-stuck, got: %v", err)
	}
	if result.WasRunning {
		t.Error("expected WasRunning=false: harness process is dead")
	}
	if !result.HarnessDead {
		t.Error("expected HarnessDead=true")
	}
	if !result.ZombieWriter {
		t.Error("expected ZombieWriter=true (orphaned agm child in pane tree)")
	}
	if result.LivenessEvidence != "zsh,agm" {
		t.Errorf("expected liveness evidence to say why, got %q", result.LivenessEvidence)
	}
}

// TestKillSession_ZombiePane_RecentlyActive_OneFlagSuffices: even when the
// zombie's manifest was recently touched (e.g. by the orphaned writer),
// EITHER --force or --confirmed-stuck alone must complete the kill.
func TestKillSession_ZombiePane_RecentlyActive_OneFlagSuffices(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  KillSessionRequest
	}{
		{"force alone", KillSessionRequest{Identifier: "zombie-session", Force: true}},
		{"confirmed-stuck alone", KillSessionRequest{Identifier: "zombie-session", ConfirmedStuck: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newManifest("id-1", "zombie-session", "~/project")
			m.UpdatedAt = time.Now().Add(-1 * time.Minute) // recently active
			tm := &mockTmuxWithLiveness{
				mockTmux: newMockTmux("zombie-session"),
				liveness: map[string]session.LivenessInfo{
					"zombie-session": {SessionExists: true, HarnessAlive: false, Evidence: "zsh"},
				},
			}
			ctx := testCtxWithLiveness([]*manifest.Manifest{m}, tm)

			result, err := KillSession(ctx, &tc.req)
			if err != nil {
				t.Fatalf("one flag must be sufficient to kill a proven-dead session, got: %v", err)
			}
			if !result.HarnessDead {
				t.Error("expected HarnessDead=true")
			}
		})
	}
}

// TestKillSession_LiveHarness_StillRequiresConfirmedStuck: when the process
// check proves the harness IS running, the safety gate stays.
func TestKillSession_LiveHarness_StillRequiresConfirmedStuck(t *testing.T) {
	m := newManifest("id-1", "live-session", "~/project")
	tm := &mockTmuxWithLiveness{
		mockTmux: newMockTmux("live-session"),
		liveness: map[string]session.LivenessInfo{
			"live-session": {SessionExists: true, HarnessAlive: true, Evidence: "zsh,claude"},
		},
	}
	ctx := testCtxWithLiveness([]*manifest.Manifest{m}, tm)

	_, err := KillSession(ctx, &KillSessionRequest{Identifier: "live-session"})
	if err == nil {
		t.Fatal("expected active-session error: harness process is alive")
	}
	opErr := &OpError{}
	if !errors.As(err, &opErr) || opErr.Code != ErrCodeActiveSessionKill {
		t.Fatalf("expected %s, got %v", ErrCodeActiveSessionKill, err)
	}
}

// TestKillSession_LivenessProbeError_FailsSafe: a failed process scan proves
// nothing — the session must still be treated as active (conservative).
func TestKillSession_LivenessProbeError_FailsSafe(t *testing.T) {
	m := newManifest("id-1", "opaque-session", "~/project")
	tm := &mockTmuxWithLiveness{
		mockTmux:    newMockTmux("opaque-session"),
		livenessErr: errors.New("ps unavailable"),
	}
	ctx := testCtxWithLiveness([]*manifest.Manifest{m}, tm)

	_, err := KillSession(ctx, &KillSessionRequest{Identifier: "opaque-session"})
	if err == nil {
		t.Fatal("expected active-session error when liveness cannot be verified")
	}
	opErr := &OpError{}
	if !errors.As(err, &opErr) || opErr.Code != ErrCodeActiveSessionKill {
		t.Fatalf("expected %s, got %v", ErrCodeActiveSessionKill, err)
	}
}

// --- SendMessage tests ---

func TestSendMessage_Success(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "my-session", "~/project"),
	}
	ctx := testCtx(sessions, "my-session")

	result, err := SendMessage(ctx, &SendMessageRequest{
		Recipient: "id-1",
		Message:   "hello world",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Operation != "send_message" {
		t.Errorf("expected operation send_message, got %s", result.Operation)
	}
	if result.Recipient != "my-session" {
		t.Errorf("expected recipient my-session, got %s", result.Recipient)
	}
	if result.SessionID != "id-1" {
		t.Errorf("expected stable session ID id-1, got %s", result.SessionID)
	}
	if result.MessageLength != 11 {
		t.Errorf("expected message length 11, got %d", result.MessageLength)
	}
	// With a Tmux client available, the legacy path delivers via send-keys.
	if !result.Delivered {
		t.Error("expected Delivered=true via tmux delivery")
	}
	mt := ctx.Tmux.(*mockTmux)
	if len(mt.readinessChecks) != 1 || mt.readinessChecks[0] != "my-session:claude-code" {
		t.Fatalf("readiness checks = %v, want [my-session:claude-code]", mt.readinessChecks)
	}
	if len(mt.sent) != 1 {
		t.Fatalf("expected 1 send-keys call, got %d", len(mt.sent))
	}
	if mt.sent[0].session != "%1" || mt.sent[0].keys != "hello world" {
		t.Errorf("unexpected send-keys: %+v", mt.sent[0])
	}
}

func TestSendMessageSerializesAndReloadsTmuxDeliveryByStableSessionID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage := dolt.NewMockAdapter()
	original := &manifest.Manifest{
		SessionID: "stable-send-id",
		Name:      "old-send-name",
		Harness:   "claude-code",
		Tmux:      manifest.Tmux{SessionName: "old-send-tmux"},
	}
	if err := storage.CreateSession(original); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	tmuxClient := newMockTmux("old-send-tmux", "current-send-tmux")

	type outcome struct {
		result *SendMessageResult
		err    error
	}
	started := make(chan struct{})
	done := make(chan outcome, 1)
	err := WithSessionLockContext(t.Context(), original.SessionID, func() error {
		go func() {
			close(started)
			result, sendErr := SendMessage(&OpContext{
				Context: t.Context(),
				Storage: storage,
				Tmux:    tmuxClient,
			}, &SendMessageRequest{Recipient: original.Name, Message: "hello"})
			done <- outcome{result: result, err: sendErr}
		}()
		<-started
		select {
		case <-done:
			return errors.New("SendMessage crossed the stable-session lock")
		case <-time.After(100 * time.Millisecond):
		}

		current, getErr := storage.GetSession(original.SessionID)
		if getErr != nil {
			return getErr
		}
		current.Name = "current-send-name"
		current.Tmux.SessionName = "current-send-tmux"
		return storage.UpdateSession(current)
	})
	if err != nil {
		t.Fatalf("hold stable-session lock: %v", err)
	}

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("SendMessage() error: %v", got.err)
		}
		if got.result == nil || got.result.SessionID != original.SessionID || got.result.Recipient != "current-send-name" {
			t.Fatalf("SendMessage() result = %#v, want stable ID and current name", got.result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SendMessage did not continue after stable-session lock released")
	}
	if len(tmuxClient.atomicChecks) != 1 || tmuxClient.atomicChecks[0] != "current-send-tmux:claude-code" {
		t.Fatalf("atomic input checks = %v, want current-send-tmux:claude-code", tmuxClient.atomicChecks)
	}
}

func TestSendMessage_NotReadyReturnsTypedNonDeliveryWithoutInput(t *testing.T) {
	for _, readiness := range []string{"NO", "QUEUE", "OVERLAY", "NOT_FOUND"} {
		t.Run(readiness, func(t *testing.T) {
			ctx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")}, "my-session")
			tm := ctx.Tmux.(*mockTmux)
			tm.readiness = session.InputReadiness{Ready: false, State: readiness}

			result, err := SendMessage(ctx, &SendMessageRequest{Recipient: "id-1", Message: "must not send"})
			if result == nil || result.Delivered {
				t.Fatalf("result = %#v, want typed non-delivery", result)
			}
			opErr := &OpError{}
			if !errors.As(err, &opErr) || opErr.Code != ErrCodeSessionNotReady {
				t.Fatalf("error = %v, want %s", err, ErrCodeSessionNotReady)
			}
			if len(tm.sent) != 0 {
				t.Fatalf("not-ready pane received input: %v", tm.sent)
			}
		})
	}
}

func TestSendMessage_ReadinessProbeFailureDoesNotSend(t *testing.T) {
	wantErr := errors.New("capture failed")
	ctx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")}, "my-session")
	tm := ctx.Tmux.(*mockTmux)
	tm.readinessErr = wantErr

	result, err := SendMessage(ctx, &SendMessageRequest{Recipient: "id-1", Message: "must not send"})
	if result == nil || result.Delivered {
		t.Fatalf("result = %#v, want non-delivery", result)
	}
	if err == nil || !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("error = %v, want readiness probe failure", err)
	}
	if len(tm.sent) != 0 {
		t.Fatalf("failed readiness probe sent input: %v", tm.sent)
	}
}

func TestSendMessage_RequiresReadinessCapabilityBeforeTmuxDelivery(t *testing.T) {
	ctx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")}, "my-session")
	base := ctx.Tmux.(*mockTmux)
	ctx.Tmux = &createOnlyTmux{TmuxInterface: base}

	result, err := SendMessage(ctx, &SendMessageRequest{Recipient: "id-1", Message: "must not send"})
	if result == nil || result.Delivered {
		t.Fatalf("result = %#v, want non-delivery", result)
	}
	opErr := &OpError{}
	if !errors.As(err, &opErr) || opErr.Code != ErrCodeSessionNotReady {
		t.Fatalf("error = %v, want %s", err, ErrCodeSessionNotReady)
	}
	if len(base.sent) != 0 {
		t.Fatalf("unverified pane received input: %v", base.sent)
	}
}

// TestSendMessage_NoDeliveryMechanism verifies that when neither a manager
// Backend nor a Tmux client is configured, SendMessage reports non-delivery
// without an error — the best-effort contract stall recovery depends on.
func TestSendMessage_NoDeliveryMechanism(t *testing.T) {
	mock := dolt.NewMockAdapter()
	_ = mock.CreateSession(newManifest("id-1", "my-session", "~/project"))
	ctx := &OpContext{Storage: mock} // no Tmux, no Manager

	result, err := SendMessage(ctx, &SendMessageRequest{
		Recipient: "id-1",
		Message:   "hello world",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Delivered {
		t.Error("expected Delivered=false when no delivery mechanism is configured")
	}
}

// TestSendMessage_DeliveryError verifies that a tmux send failure surfaces as
// an error with Delivered=false.
func TestSendMessage_DeliveryError(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "my-session", "~/project"),
	}
	ctx := testCtx(sessions, "my-session")
	ctx.Tmux.(*mockTmux).sendErr = errors.New("tmux send failed")

	result, err := SendMessage(ctx, &SendMessageRequest{
		Recipient: "id-1",
		Message:   "hello world",
	})
	if err == nil {
		t.Fatal("expected error when tmux delivery fails")
	}
	if result.Delivered {
		t.Error("expected Delivered=false on delivery failure")
	}
}

func TestSendMessage_EmptyRecipient(t *testing.T) {
	ctx := testCtx(nil)
	_, err := SendMessage(ctx, &SendMessageRequest{Recipient: "", Message: "hello"})
	if err == nil {
		t.Fatal("expected error for empty recipient")
	}
}

func TestSendMessage_RequiresOperationContextAndStorage(t *testing.T) {
	request := &SendMessageRequest{Recipient: "id-1", Message: "hello"}
	for _, test := range []struct {
		name string
		ctx  *OpContext
	}{
		{name: "nil context"},
		{name: "nil storage", ctx: &OpContext{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := SendMessage(test.ctx, request)
			opErr := &OpError{}
			if !errors.As(err, &opErr) || opErr.Code != ErrCodeStorageError {
				t.Fatalf("SendMessage() error = %v, want %s", err, ErrCodeStorageError)
			}
		})
	}
}

func TestSendMessage_EmptyMessage(t *testing.T) {
	ctx := testCtx(nil)
	_, err := SendMessage(ctx, &SendMessageRequest{Recipient: "id-1", Message: ""})
	if err == nil {
		t.Fatal("expected error for empty message")
	}
}

func TestSendMessage_NotFound(t *testing.T) {
	ctx := testCtx(nil)
	_, err := SendMessage(ctx, &SendMessageRequest{Recipient: "nonexistent", Message: "hello"})
	if err == nil {
		t.Fatal("expected error for missing session")
	}
}

func TestSendMessage_ArchivedSession(t *testing.T) {
	m := newManifest("id-1", "archived-session", "~/project")
	m.Lifecycle = manifest.LifecycleArchived
	sessions := []*manifest.Manifest{m}
	ctx := testCtx(sessions)

	_, err := SendMessage(ctx, &SendMessageRequest{Recipient: "id-1", Message: "hello"})
	if err == nil {
		t.Fatal("expected error for archived session")
	}
	opErr := &OpError{}
	ok := errors.As(err, &opErr)
	if !ok {
		t.Fatalf("expected *OpError, got %T", err)
	}
	if opErr.Code != ErrCodeSessionArchived {
		t.Errorf("expected code %s, got %s", ErrCodeSessionArchived, opErr.Code)
	}
}
