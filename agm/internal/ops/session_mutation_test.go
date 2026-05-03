package ops

import (
	"errors"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
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
	ctx := testCtx(sessions, "active-session")

	// With --confirmed-stuck but without --force, recently active should still be protected
	_, err := KillSession(ctx, &KillSessionRequest{Identifier: "active-session", ConfirmedStuck: true})
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
	if result.MessageLength != 11 {
		t.Errorf("expected message length 11, got %d", result.MessageLength)
	}
	// Stub always returns false for delivered
	if result.Delivered {
		t.Error("expected Delivered=false (stub)")
	}
}

func TestSendMessage_EmptyRecipient(t *testing.T) {
	ctx := testCtx(nil)
	_, err := SendMessage(ctx, &SendMessageRequest{Recipient: "", Message: "hello"})
	if err == nil {
		t.Fatal("expected error for empty recipient")
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
