package main

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

func setupCodexResumeTransaction(t *testing.T) (*dolt.Adapter, *manifest.Manifest, *HealthStatus) {
	t.Helper()
	adapter, err := dolt.NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() {
		if err := adapter.Close(); err != nil {
			t.Errorf("close adapter: %v", err)
		}
	})

	now := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "codex-resume-id",
		Name:          "codex-resume",
		Harness:       "codex-cli",
		CreatedAt:     now,
		UpdatedAt:     now,
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "codex-resume"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	health := &HealthStatus{
		TmuxSessionName: m.Tmux.SessionName,
		WorktreePath:    m.Context.Project,
		TmuxExists:      false,
	}
	return adapter, m, health
}

func setDetachedResumeTestGlobals(t *testing.T, detached bool) {
	t.Helper()
	oldDetached, oldPrompt, oldPromptFile, oldDeletePromptFile := resumeDetached, resumePrompt, resumePromptFile, resumeDeletePromptFile
	resumeDetached, resumePrompt, resumePromptFile, resumeDeletePromptFile = detached, "", "", false
	t.Cleanup(func() {
		resumeDetached, resumePrompt, resumePromptFile, resumeDeletePromptFile = oldDetached, oldPrompt, oldPromptFile, oldDeletePromptFile
	})
}

func testTmuxIdentity(id string) tmux.SessionIdentity {
	return tmux.SessionIdentity{ID: id, Token: "0123456789abcdef0123456789abcdef"}
}

func TestRestoreResumeTmuxNameTreatsSuccessfulSwapAsComplete(t *testing.T) {
	previousUpdatedAt := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	m := &manifest.Manifest{
		UpdatedAt: previousUpdatedAt.Add(30 * time.Minute),
		Tmux:      manifest.Tmux{SessionName: "canonical-name"},
	}
	change := resumeTmuxNameChange{
		Applied: true,
		Change: dolt.TmuxSessionNameChange{
			PreviousName:      "historical.name",
			PreviousUpdatedAt: previousUpdatedAt,
			CurrentName:       "canonical-name",
			CurrentRevision:   "owned-revision",
		},
	}
	restoreCalls := 0
	err := restoreResumeTmuxNameWith(t.Context(), func(_ context.Context, got dolt.TmuxSessionNameChange) (bool, error) {
		restoreCalls++
		if !reflect.DeepEqual(got, change.Change) {
			t.Fatalf("restore change = %#v, want %#v", got, change.Change)
		}
		return true, nil
	}, m, change)
	if err != nil {
		t.Fatalf("restoreResumeTmuxNameWith() error = %v", err)
	}
	if restoreCalls != 1 {
		t.Fatalf("restore calls = %d, want 1", restoreCalls)
	}
	if m.Tmux.SessionName != change.Change.PreviousName || !m.UpdatedAt.Equal(previousUpdatedAt) {
		t.Fatalf("restored snapshot = (%q, %v), want (%q, %v)", m.Tmux.SessionName, m.UpdatedAt, change.Change.PreviousName, previousUpdatedAt)
	}
}

func TestPersistResumeTmuxNameRetainsPendingChangeUntilReloadCompensationIsProven(t *testing.T) {
	reloadErr := errors.New("reload failed")
	restoreErr := errors.New("restore unavailable")
	change := dolt.TmuxSessionNameChange{
		SessionID:         "session-id",
		PreviousName:      "historical-name",
		PreviousUpdatedAt: time.Now().Add(-time.Hour).UTC().Truncate(time.Second),
		CurrentName:       "canonical-name",
		CurrentRevision:   "owned-revision",
	}
	tests := []struct {
		name           string
		restored       bool
		restoreErr     error
		wantPending    bool
		wantRestoreErr bool
	}{
		{name: "restore error", restoreErr: restoreErr, wantPending: true, wantRestoreErr: true},
		{name: "restore rejected", restored: false, wantPending: true},
		{name: "restore proven", restored: true, wantPending: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &manifest.Manifest{SessionID: change.SessionID}
			got, err := persistResumeTmuxNameWith(
				t.Context(),
				m,
				change.CurrentName,
				func(_ context.Context, sessionID, newName string) (*dolt.TmuxSessionNameChange, error) {
					if sessionID != change.SessionID || newName != change.CurrentName {
						t.Fatalf("begin args = (%q, %q)", sessionID, newName)
					}
					copy := change
					return &copy, nil
				},
				func(sessionID string) (*manifest.Manifest, error) {
					if sessionID != change.SessionID {
						t.Fatalf("load session ID = %q, want %q", sessionID, change.SessionID)
					}
					return nil, reloadErr
				},
				func(_ context.Context, got dolt.TmuxSessionNameChange) (bool, error) {
					if !reflect.DeepEqual(got, change) {
						t.Fatalf("restore change = %#v, want %#v", got, change)
					}
					return tt.restored, tt.restoreErr
				},
			)
			if !errors.Is(err, reloadErr) {
				t.Fatalf("persistResumeTmuxNameWith() error = %v, want reload failure", err)
			}
			if tt.wantRestoreErr && !errors.Is(err, restoreErr) {
				t.Fatalf("persistResumeTmuxNameWith() error = %v, want restore failure", err)
			}
			if got.Applied != tt.wantPending {
				t.Fatalf("pending change = %#v, want Applied=%v", got, tt.wantPending)
			}
			if tt.wantPending && !reflect.DeepEqual(got.Change, change) {
				t.Fatalf("pending change = %#v, want %#v", got.Change, change)
			}
		})
	}
}

func TestPersistResumeTmuxNameRetainsChangeWhenBeginCommitIsAmbiguous(t *testing.T) {
	commitErr := errors.New("commit acknowledgement lost")
	change := dolt.TmuxSessionNameChange{
		SessionID:         "session-id",
		PreviousName:      "historical-name",
		PreviousUpdatedAt: time.Now().Add(-time.Hour).UTC().Truncate(time.Second),
		CurrentName:       "canonical-name",
		CurrentRevision:   "owned-revision",
	}

	got, err := persistResumeTmuxNameWith(
		t.Context(),
		&manifest.Manifest{SessionID: change.SessionID},
		change.CurrentName,
		func(context.Context, string, string) (*dolt.TmuxSessionNameChange, error) {
			copy := change
			return &copy, commitErr
		},
		func(string) (*manifest.Manifest, error) {
			t.Fatal("load must not run after an ambiguous begin error")
			return nil, nil
		},
		func(context.Context, dolt.TmuxSessionNameChange) (bool, error) {
			t.Fatal("restore belongs to the outer rollback transaction")
			return false, nil
		},
	)
	if !errors.Is(err, commitErr) {
		t.Fatalf("persistResumeTmuxNameWith() error = %v, want commit failure", err)
	}
	if !got.Applied || !reflect.DeepEqual(got.Change, change) {
		t.Fatalf("pending change = %#v, want %#v", got, change)
	}
}

func TestResumeResolvedSessionAcquiresSessionLockBeforeReads(t *testing.T) {
	wantErr := errors.New("lock unavailable")
	lockCalls := 0
	err := resumeResolvedSessionWithLocker(t.Context(), nil, "stable-session-id", "", func(sessionID string, fn func() error) error {
		lockCalls++
		if sessionID != "stable-session-id" {
			t.Fatalf("lock key = %q, want stable-session-id", sessionID)
		}
		if fn == nil {
			t.Fatal("locked resume callback is nil")
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("resumeResolvedSessionWithLocker() error = %v, want %v", err, wantErr)
	}
	if lockCalls != 1 {
		t.Fatalf("lock calls = %d, want 1", lockCalls)
	}
}

func TestResumeAttachmentRunsAfterSessionLockReleases(t *testing.T) {
	setDetachedResumeTestGlobals(t, false)
	locked := false
	attached := false
	attachment, err := runResumeTransactionWithLock("stable-session-id", func(sessionID string, fn func() error) error {
		if sessionID != "stable-session-id" {
			t.Fatalf("lock key = %q, want stable-session-id", sessionID)
		}
		locked = true
		defer func() { locked = false }()
		return fn()
	}, func() (*resumeAttachment, error) {
		if !locked {
			t.Fatal("resume transaction ran outside the session lock")
		}
		return &resumeAttachment{
			ctx:       t.Context(),
			sessionID: "stable-session-id",
			health:    &HealthStatus{TmuxSessionName: "stable-session"},
			runtime: resumeSessionRuntime{attachTmux: func(string) error {
				if locked {
					t.Fatal("interactive attachment ran while the session lock was held")
				}
				attached = true
				return nil
			}},
		}, nil
	})
	if err != nil {
		t.Fatalf("runResumeTransactionWithLock() error = %v", err)
	}
	if locked {
		t.Fatal("session lock remained held after the transaction returned")
	}
	if err := attachment.finish(); err != nil {
		t.Fatalf("attachment.finish() error = %v", err)
	}
	if !attached {
		t.Fatal("attachment did not run")
	}
}

func recordingResumeRuntime(calls *[]string) resumeSessionRuntime {
	record := func(call string) { *calls = append(*calls, call) }
	return resumeSessionRuntime{
		createTmux: func(string, string) (tmux.SessionIdentity, error) {
			record("create")
			return testTmuxIdentity("$42"), nil
		},
		killTmux: func(createdResumeTmux) error { record("kill"); return nil },
		dispatch: func(*dolt.Adapter, *manifest.Manifest, string, *HealthStatus) error {
			record("dispatch")
			return nil
		},
		wait: func(string, *HealthStatus) error { record("wait"); return nil },
		persistTmuxName: func(context.Context, *dolt.Adapter, *manifest.Manifest, string) (resumeTmuxNameChange, error) {
			record("persist")
			return resumeTmuxNameChange{}, nil
		},
		restoreTmuxName: func(context.Context, *dolt.Adapter, *manifest.Manifest, resumeTmuxNameChange) error {
			record("compensate")
			return nil
		},
		restorePermission: func(string, *manifest.Manifest, *HealthStatus) {
			record("restore")
		},
		updateActivity: func(context.Context, *dolt.Adapter, string, string) error { record("update"); return nil },
		updateTabTitle: func(string) { record("tab") },
		deliverPrompt:  func(string, string, string, bool) error { record("prompt"); return nil },
		attachTmux:     func(string) error { record("attach"); return nil },
	}
}

func TestResumeSessionCodexCommitsEffectsOnlyAfterReadiness(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	resumePrompt = "continue after readiness"
	adapter, m, health := setupCodexResumeTransaction(t)
	var calls []string
	runtime := recordingResumeRuntime(&calls)

	if err := resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime); err != nil {
		t.Fatalf("resumeSessionWithRuntime() error = %v", err)
	}
	want := []string{"create", "dispatch", "wait", "persist", "prompt", "restore", "update", "tab"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("resume calls = %v, want %v", calls, want)
	}
}

func TestResumeSessionClaudeRestoresSavedModeBeforePromptDelivery(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	resumePrompt = "continue in the saved plan mode"
	adapter, m, health := setupCodexResumeTransaction(t)
	m.Harness = "claude-code"
	m.PermissionMode = "plan"
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.loadManifest = func(context.Context, *dolt.Adapter, string, string) (*manifest.Manifest, error) {
		return m, nil
	}

	if err := resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime); err != nil {
		t.Fatalf("resumeSessionWithRuntime() error = %v", err)
	}
	want := []string{"create", "dispatch", "wait", "persist", "restore", "update", "tab", "prompt"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("resume calls = %v, want %v", calls, want)
	}
}

func TestResumeSessionCodexRollsBackNewTmuxBeforeActivityUpdate(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	for _, failurePoint := range []string{"dispatch", "wait"} {
		t.Run(failurePoint, func(t *testing.T) {
			adapter, m, health := setupCodexResumeTransaction(t)
			storedBefore, err := adapter.GetSession(m.SessionID)
			if err != nil {
				t.Fatalf("GetSession() before resume error = %v", err)
			}
			wantErr := errors.New(failurePoint + " failed")
			var calls []string
			runtime := recordingResumeRuntime(&calls)
			if failurePoint == "dispatch" {
				runtime.dispatch = func(*dolt.Adapter, *manifest.Manifest, string, *HealthStatus) error {
					calls = append(calls, "dispatch")
					return wantErr
				}
			} else {
				runtime.wait = func(string, *HealthStatus) error {
					calls = append(calls, "wait")
					return wantErr
				}
			}

			err = resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime)
			if !errors.Is(err, wantErr) {
				t.Fatalf("error = %v, want %v", err, wantErr)
			}
			if len(calls) == 0 || calls[len(calls)-1] != "kill" {
				t.Fatalf("resume calls = %v, want rollback as final effect", calls)
			}
			for _, forbidden := range []string{"restore", "update", "tab", "prompt", "attach"} {
				for _, call := range calls {
					if call == forbidden {
						t.Fatalf("resume calls = %v, %q must not run after readiness failure", calls, forbidden)
					}
				}
			}
			storedAfter, getErr := adapter.GetSession(m.SessionID)
			if getErr != nil {
				t.Fatalf("GetSession() after resume error = %v", getErr)
			}
			if !storedAfter.UpdatedAt.Equal(storedBefore.UpdatedAt) {
				t.Fatalf("UpdatedAt changed after failed resume: before=%v after=%v", storedBefore.UpdatedAt, storedAfter.UpdatedAt)
			}
		})
	}
}

func TestResumeSessionCodexPropagatesHookReviewBeforeActivityUpdate(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	adapter, m, health := setupCodexResumeTransaction(t)
	storedBefore, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() before resume error = %v", err)
	}

	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.wait = func(string, *HealthStatus) error {
		calls = append(calls, "wait")
		return tmux.CodexHookReviewError()
	}

	err = resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime)
	if !errors.Is(err, tmux.ErrCodexHookReviewRequired) {
		t.Fatalf("resumeSessionWithRuntime() error = %v, want ErrCodexHookReviewRequired", err)
	}
	if len(calls) == 0 || calls[len(calls)-1] != "kill" {
		t.Fatalf("resume calls = %v, want rollback as final effect", calls)
	}
	for _, forbidden := range []string{"restore", "update", "tab", "prompt", "attach"} {
		if slices.Contains(calls, forbidden) {
			t.Fatalf("resume calls = %v, %q must not run after hook review failure", calls, forbidden)
		}
	}
	storedAfter, getErr := adapter.GetSession(m.SessionID)
	if getErr != nil {
		t.Fatalf("GetSession() after resume error = %v", getErr)
	}
	if !storedAfter.UpdatedAt.Equal(storedBefore.UpdatedAt) {
		t.Fatalf("UpdatedAt changed after failed resume: before=%v after=%v", storedBefore.UpdatedAt, storedAfter.UpdatedAt)
	}
}

func TestResumeSessionCodexRollsBackWhenPromptDeliveryIsCanceled(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	resumePrompt = "continue only if the caller remains active"
	adapter, m, health := setupCodexResumeTransaction(t)
	storedBefore, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() before resume error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.deliverPrompt = func(string, string, string, bool) error {
		calls = append(calls, "prompt")
		cancel()
		return ctx.Err()
	}

	err = resumeSessionWithRuntime(ctx, adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("resumeSessionWithRuntime() error = %v, want context.Canceled", err)
	}
	if want := []string{"create", "dispatch", "wait", "persist", "prompt", "kill"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("resume calls = %v, want %v", calls, want)
	}
	storedAfter, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after resume error = %v", err)
	}
	if !storedAfter.UpdatedAt.Equal(storedBefore.UpdatedAt) || storedAfter.Tmux.SessionName != storedBefore.Tmux.SessionName {
		t.Fatalf("canceled prompt committed resume metadata: before=%#v after=%#v", storedBefore, storedAfter)
	}
}

func TestResumeSessionCodexRollsBackPromptlessCancellationBeforeFinalization(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	adapter, m, health := setupCodexResumeTransaction(t)
	health.TmuxSessionName = "codex.resume:promptless-cancel"
	storedBefore, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() before resume error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.persistTmuxName = func(ctx context.Context, adapter *dolt.Adapter, m *manifest.Manifest, name string) (resumeTmuxNameChange, error) {
		calls = append(calls, "persist")
		return persistResumeTmuxName(ctx, adapter, m, name)
	}
	runtime.restoreTmuxName = func(ctx context.Context, adapter *dolt.Adapter, m *manifest.Manifest, change resumeTmuxNameChange) error {
		calls = append(calls, "compensate")
		return restoreResumeTmuxName(ctx, adapter, m, change)
	}
	runtime.completeTmuxName = func(ctx context.Context, adapter *dolt.Adapter, change resumeTmuxNameChange) error {
		calls = append(calls, "complete")
		return completeResumeTmuxName(ctx, adapter, change)
	}
	runtime.restorePermission = func(string, *manifest.Manifest, *HealthStatus) {
		calls = append(calls, "restore")
		cancel()
	}

	err = resumeSessionWithRuntime(ctx, adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("resumeSessionWithRuntime() error = %v, want context.Canceled", err)
	}
	if want := []string{"create", "dispatch", "wait", "persist", "restore", "compensate", "kill"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("resume calls = %v, want promptless cancellation rollback %v", calls, want)
	}
	storedAfter, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after resume error = %v", err)
	}
	if storedAfter.Tmux.SessionName != storedBefore.Tmux.SessionName || !storedAfter.UpdatedAt.Equal(storedBefore.UpdatedAt) {
		t.Fatalf("promptless cancellation left canonical metadata: before=%#v after=%#v", storedBefore, storedAfter)
	}
}

func TestResumeSessionCodexRollsBackPromptlessCancellationAfterActivityTouch(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	adapter, m, health := setupCodexResumeTransaction(t)
	health.TmuxSessionName = "codex.resume:activity-cancel"
	storedBefore, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() before resume error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.persistTmuxName = func(ctx context.Context, adapter *dolt.Adapter, m *manifest.Manifest, name string) (resumeTmuxNameChange, error) {
		calls = append(calls, "persist")
		return persistResumeTmuxName(ctx, adapter, m, name)
	}
	runtime.restoreTmuxName = func(ctx context.Context, adapter *dolt.Adapter, m *manifest.Manifest, change resumeTmuxNameChange) error {
		calls = append(calls, "compensate")
		return restoreResumeTmuxName(ctx, adapter, m, change)
	}
	runtime.completeTmuxName = func(context.Context, *dolt.Adapter, resumeTmuxNameChange) error {
		calls = append(calls, "complete")
		return nil
	}
	runtime.updateActivity = func(ctx context.Context, adapter *dolt.Adapter, sessionID, _ string) error {
		calls = append(calls, "update")
		if err := adapter.TouchSessionActivity(context.WithoutCancel(ctx), sessionID); err != nil {
			return err
		}
		cancel()
		return nil
	}

	err = resumeSessionWithRuntime(ctx, adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("resumeSessionWithRuntime() error = %v, want context.Canceled", err)
	}
	if want := []string{"create", "dispatch", "wait", "persist", "restore", "update", "compensate", "kill"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("resume calls = %v, want post-touch rollback %v", calls, want)
	}
	storedAfter, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after resume error = %v", err)
	}
	if storedAfter.Tmux.SessionName != storedBefore.Tmux.SessionName || !storedAfter.UpdatedAt.Equal(storedBefore.UpdatedAt) {
		t.Fatalf("post-touch cancellation left finalization effects: before=%#v after=%#v", storedBefore, storedAfter)
	}
}

func TestResumeSessionCodexDoesNotReturnCancellationAfterPromptDelivery(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	resumePrompt = "start irreversible work"
	adapter, m, health := setupCodexResumeTransaction(t)
	ctx, cancel := context.WithCancel(t.Context())
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.persistTmuxName = func(context.Context, *dolt.Adapter, *manifest.Manifest, string) (resumeTmuxNameChange, error) {
		calls = append(calls, "persist")
		return resumeTmuxNameChange{
			Applied: true,
			Change: dolt.TmuxSessionNameChange{
				SessionID:       m.SessionID,
				CurrentName:     health.TmuxSessionName,
				CurrentRevision: "owned-revision",
			},
		}, nil
	}
	runtime.deliverPrompt = func(string, string, string, bool) error {
		calls = append(calls, "prompt")
		cancel()
		return nil
	}
	runtime.completeTmuxName = func(completeCtx context.Context, _ *dolt.Adapter, change resumeTmuxNameChange) error {
		if err := completeCtx.Err(); err != nil {
			t.Fatalf("metadata completion context after prompt = %v, want active", err)
		}
		if !change.Applied || change.Change.CurrentRevision != "owned-revision" {
			t.Fatalf("metadata completion change = %#v", change)
		}
		calls = append(calls, "complete")
		return nil
	}

	if err := resumeSessionWithRuntime(ctx, adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime); err != nil {
		t.Fatalf("resumeSessionWithRuntime() after delivered prompt error = %v, want success", err)
	}
	if want := []string{"create", "dispatch", "wait", "persist", "prompt", "complete", "restore", "update", "tab"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("resume calls = %v, want completed success effects %v", calls, want)
	}
}

func TestResumeSessionCodexPreservesStartedWorkWhenPromptAcknowledgementIsLost(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	resumePrompt = "start work exactly once despite a lost tmux reply"
	adapter, m, health := setupCodexResumeTransaction(t)
	health.TmuxSessionName = "codex.resume:lost-prompt-ack"
	wantErr := errors.New("tmux Enter acknowledgement lost")
	ctx, cancel := context.WithCancel(t.Context())
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.persistTmuxName = func(ctx context.Context, adapter *dolt.Adapter, m *manifest.Manifest, name string) (resumeTmuxNameChange, error) {
		calls = append(calls, "persist")
		return persistResumeTmuxName(ctx, adapter, m, name)
	}
	runtime.restoreTmuxName = func(ctx context.Context, adapter *dolt.Adapter, m *manifest.Manifest, change resumeTmuxNameChange) error {
		calls = append(calls, "compensate")
		return restoreResumeTmuxName(ctx, adapter, m, change)
	}
	runtime.completeTmuxName = func(ctx context.Context, adapter *dolt.Adapter, change resumeTmuxNameChange) error {
		if err := ctx.Err(); err != nil {
			t.Fatalf("completion context after ambiguous prompt submission = %v, want active", err)
		}
		calls = append(calls, "complete")
		return completeResumeTmuxName(ctx, adapter, change)
	}
	runtime.deliverPrompt = func(string, string, string, bool) error {
		calls = append(calls, "prompt")
		cancel()
		return tmux.MarkPromptSubmissionUncertain(wantErr)
	}

	if err := resumeSessionWithRuntime(ctx, adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime); err != nil {
		t.Fatalf("resumeSessionWithRuntime() after lost prompt acknowledgement = %v, want irreversible success", err)
	}
	if want := []string{"create", "dispatch", "wait", "persist", "prompt", "complete", "restore", "update", "tab"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("resume calls = %v, want preservation without compensation %v", calls, want)
	}
	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after ambiguous prompt submission: %v", err)
	}
	if stored.Tmux.SessionName != tmux.SanitizeSessionName(health.TmuxSessionName) {
		t.Fatalf("stored tmux name = %q, want preserved canonical name %q", stored.Tmux.SessionName, tmux.SanitizeSessionName(health.TmuxSessionName))
	}
}

func TestResumeSessionCodexDoesNotReturnAttachFailureAfterPromptDelivery(t *testing.T) {
	setDetachedResumeTestGlobals(t, false)
	resumePrompt = "start irreversible work"
	adapter, m, health := setupCodexResumeTransaction(t)
	attachErr := errors.New("terminal unavailable")
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.attachTmux = func(string) error {
		calls = append(calls, "attach")
		return attachErr
	}

	if err := resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime); err != nil {
		t.Fatalf("resumeSessionWithRuntime() after delivered prompt and attach failure = %v, want success", err)
	}
	if want := []string{"create", "dispatch", "wait", "persist", "prompt", "restore", "update", "tab", "attach"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("resume calls = %v, want post-prompt attach attempt without rollback %v", calls, want)
	}
}

func TestResumeSessionCodexReturnsAttachFailureAfterPromptDeliveryFails(t *testing.T) {
	setDetachedResumeTestGlobals(t, false)
	resumePrompt = "start irreversible work"
	adapter, m, health := setupCodexResumeTransaction(t)
	health.TmuxExists = true
	promptErr := errors.New("tmux paste failed")
	attachErr := errors.New("terminal unavailable")
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.createTmux = func(string, string) (tmux.SessionIdentity, error) {
		t.Fatal("create called for pre-existing tmux session")
		return tmux.SessionIdentity{}, nil
	}
	runtime.killTmux = func(createdResumeTmux) error {
		t.Fatal("pre-existing tmux session was killed")
		return nil
	}
	runtime.deliverPrompt = func(string, string, string, bool) error {
		calls = append(calls, "prompt")
		return promptErr
	}
	runtime.attachTmux = func(string) error {
		calls = append(calls, "attach")
		return attachErr
	}

	err := resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime)
	if !errors.Is(err, attachErr) {
		t.Fatalf("resumeSessionWithRuntime() error = %v, want attach failure %v", err, attachErr)
	}
	if want := []string{"prompt", "update", "tab", "attach"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("resume calls = %v, want failed prompt followed by observable attach failure %v", calls, want)
	}
}

func TestResumeSessionCodexCompensatesCanonicalNameWhenOrdinaryPromptDeliveryFails(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	resumePrompt = "start work only after the resume transaction commits"
	adapter, m, health := setupCodexResumeTransaction(t)
	health.TmuxSessionName = "codex.resume:prompt-failure"
	storedBefore, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() before resume error = %v", err)
	}
	wantErr := tmux.ErrPasteNotSubmitted
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.persistTmuxName = func(ctx context.Context, adapter *dolt.Adapter, m *manifest.Manifest, name string) (resumeTmuxNameChange, error) {
		calls = append(calls, "persist")
		return persistResumeTmuxName(ctx, adapter, m, name)
	}
	runtime.restoreTmuxName = func(ctx context.Context, adapter *dolt.Adapter, m *manifest.Manifest, change resumeTmuxNameChange) error {
		calls = append(calls, "compensate")
		return restoreResumeTmuxName(ctx, adapter, m, change)
	}
	runtime.deliverPrompt = func(string, string, string, bool) error {
		calls = append(calls, "prompt")
		return wantErr
	}

	err = resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime)
	if !errors.Is(err, wantErr) {
		t.Fatalf("resumeSessionWithRuntime() error = %v, want %v", err, wantErr)
	}
	wantCalls := []string{"create", "dispatch", "wait", "persist", "prompt", "compensate", "kill"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("resume calls = %v, want %v", calls, wantCalls)
	}
	storedAfter, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after resume error = %v", err)
	}
	if storedAfter.Tmux.SessionName != storedBefore.Tmux.SessionName || !storedAfter.UpdatedAt.Equal(storedBefore.UpdatedAt) {
		t.Fatalf("failed prompt left provisional metadata: before=%#v after=%#v", storedBefore, storedAfter)
	}
}

func TestResumeSessionCodexCompensationPreservesNewerMetadata(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	resumePrompt = "start after durable identity"
	adapter, m, health := setupCodexResumeTransaction(t)
	health.TmuxSessionName = "codex.resume:newer-metadata"
	wantErr := errors.New("prompt delivery failed")
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.persistTmuxName = func(ctx context.Context, adapter *dolt.Adapter, m *manifest.Manifest, name string) (resumeTmuxNameChange, error) {
		calls = append(calls, "persist")
		return persistResumeTmuxName(ctx, adapter, m, name)
	}
	runtime.restoreTmuxName = func(ctx context.Context, adapter *dolt.Adapter, m *manifest.Manifest, change resumeTmuxNameChange) error {
		calls = append(calls, "compensate")
		return restoreResumeTmuxName(ctx, adapter, m, change)
	}
	runtime.deliverPrompt = func(string, string, string, bool) error {
		calls = append(calls, "prompt")
		latest, err := adapter.GetSession(m.SessionID)
		if err != nil {
			t.Fatalf("GetSession() during prompt error: %v", err)
		}
		latest.Context.Notes = "newer writer after provisional persistence"
		if err := adapter.UpdateSession(latest); err != nil {
			t.Fatalf("UpdateSession() during prompt error: %v", err)
		}
		return wantErr
	}

	err := resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime)
	if !errors.Is(err, wantErr) || err == nil || !strings.Contains(err.Error(), "metadata no longer matches") {
		t.Fatalf("resumeSessionWithRuntime() error = %v, want joined prompt and compensation ownership failures", err)
	}
	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after failed compensation error: %v", err)
	}
	if stored.Tmux.SessionName != tmux.SanitizeSessionName(health.TmuxSessionName) || stored.Context.Notes != "newer writer after provisional persistence" {
		t.Fatalf("newer metadata was overwritten: name=%q notes=%q", stored.Tmux.SessionName, stored.Context.Notes)
	}
	wantCalls := []string{"create", "dispatch", "wait", "persist", "prompt", "compensate"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("resume calls = %v, want %v; superseded metadata must preserve tmux", calls, wantCalls)
	}
}

func TestResumeSessionCodexPreservesTmuxWhenPersistenceCompensationIsUnproven(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	adapter, m, health := setupCodexResumeTransaction(t)
	reloadErr := errors.New("reload after provisional persistence failed")
	ownershipErr := errors.New("metadata no longer matches this resume transaction")
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	change := resumeTmuxNameChange{
		Applied: true,
		Change: dolt.TmuxSessionNameChange{
			SessionID:       m.SessionID,
			CurrentName:     tmux.SanitizeSessionName(health.TmuxSessionName),
			CurrentRevision: "owned-revision",
		},
	}
	runtime.persistTmuxName = func(context.Context, *dolt.Adapter, *manifest.Manifest, string) (resumeTmuxNameChange, error) {
		calls = append(calls, "persist")
		return change, reloadErr
	}
	runtime.restoreTmuxName = func(context.Context, *dolt.Adapter, *manifest.Manifest, resumeTmuxNameChange) error {
		calls = append(calls, "compensate")
		return ownershipErr
	}
	runtime.killTmux = func(createdResumeTmux) error {
		calls = append(calls, "kill")
		return nil
	}

	err := resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime)
	if !errors.Is(err, reloadErr) || !errors.Is(err, ownershipErr) {
		t.Fatalf("resumeSessionWithRuntime() error = %v, want joined reload and ownership failures", err)
	}
	if want := []string{"create", "dispatch", "wait", "persist", "compensate"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("resume calls = %v, want %v; unproven compensation must preserve tmux", calls, want)
	}
}

func TestResumeSessionCodexRollsBackCreationFailureWhenTmuxReturnedIdentity(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	adapter, m, health := setupCodexResumeTransaction(t)
	wantErr := errors.New("workdir initialization failed")
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.createTmux = func(string, string) (tmux.SessionIdentity, error) {
		calls = append(calls, "create")
		return testTmuxIdentity("$42"), wantErr
	}
	runtime.killTmux = func(created createdResumeTmux) error {
		calls = append(calls, "kill:"+created.Identity.ID)
		return nil
	}

	err := resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime)
	if !errors.Is(err, wantErr) {
		t.Fatalf("resumeSessionWithRuntime() error = %v, want %v", err, wantErr)
	}
	if want := []string{"create", "kill:$42"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("resume calls = %v, want %v", calls, want)
	}
}

func TestResumeSessionCodexRollsBackCreationFailureWhenOnlyProvisionalIdentityReturned(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	adapter, m, health := setupCodexResumeTransaction(t)
	wantErr := errors.New("tmux output lost after provisional creation")
	token := "0123456789abcdef0123456789abcdef"
	partial := tmux.SessionIdentity{Token: token, CreationName: "agm-create-" + token}
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.createTmux = func(string, string) (tmux.SessionIdentity, error) {
		calls = append(calls, "create")
		return partial, wantErr
	}
	runtime.killTmux = func(created createdResumeTmux) error {
		if created.Identity != partial || !created.owned() {
			t.Fatalf("cleanup identity = %#v, want owned provisional %#v", created.Identity, partial)
		}
		calls = append(calls, "kill:"+created.Identity.CreationName)
		return nil
	}

	err := resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime)
	if !errors.Is(err, wantErr) {
		t.Fatalf("resumeSessionWithRuntime() error = %v, want %v", err, wantErr)
	}
	if want := []string{"create", "kill:" + partial.CreationName}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("resume calls = %v, want %v", calls, want)
	}
}

func TestResumeSessionCodexJoinsCleanupFailure(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	adapter, m, health := setupCodexResumeTransaction(t)
	primaryErr := errors.New("composer missing")
	cleanupErr := errors.New("tmux target remained")
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.wait = func(string, *HealthStatus) error { return primaryErr }
	runtime.killTmux = func(createdResumeTmux) error { return cleanupErr }

	err := resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime)
	if !errors.Is(err, primaryErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("error = %v, want joined primary and cleanup failures", err)
	}
}

func TestResumeSessionCodexRollbackUsesCreatedCanonicalTmuxName(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	adapter, m, health := setupCodexResumeTransaction(t)
	health.TmuxSessionName = "codex.resume:legacy"
	wantName := tmux.SanitizeSessionName(health.TmuxSessionName)
	wantErr := errors.New("dispatch failed")
	var calls []string
	var createdName, killedName, killedID string
	runtime := recordingResumeRuntime(&calls)
	runtime.createTmux = func(name, _ string) (tmux.SessionIdentity, error) {
		createdName = name
		return testTmuxIdentity("$43"), nil
	}
	runtime.dispatch = func(*dolt.Adapter, *manifest.Manifest, string, *HealthStatus) error {
		return wantErr
	}
	runtime.killTmux = func(created createdResumeTmux) error {
		killedName = created.Name
		killedID = created.Identity.ID
		return nil
	}

	err := resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime)
	if !errors.Is(err, wantErr) {
		t.Fatalf("resumeSessionWithRuntime() error = %v, want %v", err, wantErr)
	}
	if createdName != wantName || killedName != wantName || killedID != "$43" || health.TmuxSessionName != wantName {
		t.Fatalf("tmux identity = create %q, kill %q (%s), health %q; want %q ($43)", createdName, killedName, killedID, health.TmuxSessionName, wantName)
	}
}

func TestResumeSessionCodexPersistsCreatedCanonicalTmuxName(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	resumePrompt = "continue after canonical persistence"
	adapter, m, health := setupCodexResumeTransaction(t)
	health.TmuxSessionName = "codex.resume:persist"
	wantName := tmux.SanitizeSessionName(health.TmuxSessionName)
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.persistTmuxName = func(ctx context.Context, adapter *dolt.Adapter, m *manifest.Manifest, name string) (resumeTmuxNameChange, error) {
		calls = append(calls, "persist")
		return persistResumeTmuxName(ctx, adapter, m, name)
	}
	runtime.completeTmuxName = func(ctx context.Context, adapter *dolt.Adapter, change resumeTmuxNameChange) error {
		calls = append(calls, "complete")
		return completeResumeTmuxName(ctx, adapter, change)
	}

	if err := resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime); err != nil {
		t.Fatalf("resumeSessionWithRuntime() error = %v", err)
	}
	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if stored.Tmux.SessionName != wantName {
		t.Fatalf("stored tmux name = %q, want %q", stored.Tmux.SessionName, wantName)
	}
	if want := []string{"create", "dispatch", "wait", "persist", "prompt", "complete", "restore", "update", "tab"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("resume calls = %v, want %v", calls, want)
	}
}

func TestResumeSessionCodexTmuxPersistencePreservesConcurrentMetadata(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	adapter, m, health := setupCodexResumeTransaction(t)
	health.TmuxSessionName = "codex.resume:concurrent"
	wantName := tmux.SanitizeSessionName(health.TmuxSessionName)
	const wantUUID = "hook-associated-during-readiness"
	const wantNotes = "updated while Codex was starting"
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.wait = func(string, *HealthStatus) error {
		calls = append(calls, "wait")
		latest, err := adapter.GetSession(m.SessionID)
		if err != nil {
			return err
		}
		latest.Claude.UUID = wantUUID
		latest.Context.Notes = wantNotes
		return adapter.UpdateSession(latest)
	}
	runtime.persistTmuxName = func(ctx context.Context, adapter *dolt.Adapter, m *manifest.Manifest, name string) (resumeTmuxNameChange, error) {
		calls = append(calls, "persist")
		return persistResumeTmuxName(ctx, adapter, m, name)
	}
	runtime.completeTmuxName = func(ctx context.Context, adapter *dolt.Adapter, change resumeTmuxNameChange) error {
		calls = append(calls, "complete")
		return completeResumeTmuxName(ctx, adapter, change)
	}

	if err := resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime); err != nil {
		t.Fatalf("resumeSessionWithRuntime() error = %v", err)
	}
	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if stored.Tmux.SessionName != wantName {
		t.Fatalf("stored tmux name = %q, want %q", stored.Tmux.SessionName, wantName)
	}
	if stored.Claude.UUID != wantUUID || stored.Context.Notes != wantNotes {
		t.Fatalf("concurrent metadata was lost: uuid=%q notes=%q", stored.Claude.UUID, stored.Context.Notes)
	}
	if want := []string{"create", "dispatch", "wait", "persist", "restore", "update", "tab", "complete"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("resume calls = %v, want %v", calls, want)
	}
}

func TestResumeSessionCodexRollsBackWhenCanonicalNamePersistenceFails(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	adapter, m, health := setupCodexResumeTransaction(t)
	health.TmuxSessionName = "codex.resume:persist-failure"
	wantErr := errors.New("persist failed")
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.persistTmuxName = func(context.Context, *dolt.Adapter, *manifest.Manifest, string) (resumeTmuxNameChange, error) {
		calls = append(calls, "persist")
		return resumeTmuxNameChange{}, wantErr
	}

	err := resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime)
	if !errors.Is(err, wantErr) {
		t.Fatalf("resumeSessionWithRuntime() error = %v, want %v", err, wantErr)
	}
	if want := []string{"create", "dispatch", "wait", "persist", "kill"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("resume calls = %v, want %v", calls, want)
	}
}

func TestResumeSessionPreservesPreexistingTmuxOnLaterFailure(t *testing.T) {
	setDetachedResumeTestGlobals(t, false)
	adapter, m, health := setupCodexResumeTransaction(t)
	health.TmuxExists = true
	attachErr := errors.New("attach failed")
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.createTmux = func(string, string) (tmux.SessionIdentity, error) {
		t.Fatal("create called for pre-existing tmux session")
		return tmux.SessionIdentity{}, nil
	}
	runtime.killTmux = func(createdResumeTmux) error {
		t.Fatal("pre-existing tmux session was killed")
		return nil
	}
	runtime.attachTmux = func(string) error { return attachErr }

	err := resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime)
	if !errors.Is(err, attachErr) {
		t.Fatalf("error = %v, want %v", err, attachErr)
	}
}

func TestResumeSessionPiRelaunchesOnlyInExistingRestartableShell(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	adapter, m, health := setupCodexResumeTransaction(t)
	m.Harness = "pi-cli"
	health.TmuxExists = true
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.loadManifest = func(context.Context, *dolt.Adapter, string, string) (*manifest.Manifest, error) {
		return m, nil
	}
	runtime.checkPiProcess = func(_ context.Context, session, _ string) (bool, error) {
		calls = append(calls, "check-process")
		if session != health.TmuxSessionName {
			t.Fatalf("exact process session = %q", session)
		}
		return false, nil
	}
	runtime.checkLiveness = func(_ context.Context, session, _ string) (tmux.PaneLiveness, error) {
		calls = append(calls, "check-liveness")
		if session != health.TmuxSessionName {
			t.Fatalf("liveness session = %q", session)
		}
		return tmux.PaneLiveness{SessionExists: true, RestartableShell: true, Evidence: "zsh"}, nil
	}

	if err := resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime); err != nil {
		t.Fatalf("resumeSessionWithRuntime() error = %v", err)
	}
	want := []string{"check-process", "check-liveness", "dispatch", "wait", "restore", "update", "tab"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("Pi shell resume calls = %v, want %v", calls, want)
	}
}

func TestResumeSessionPiPreservesExactExistingProcess(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	adapter, m, health := setupCodexResumeTransaction(t)
	m.Harness = "pi-cli"
	health.TmuxExists = true
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.loadManifest = func(context.Context, *dolt.Adapter, string, string) (*manifest.Manifest, error) {
		return m, nil
	}
	runtime.checkPiProcess = func(context.Context, string, string) (bool, error) {
		calls = append(calls, "check-process")
		return true, nil
	}
	runtime.checkLiveness = func(context.Context, string, string) (tmux.PaneLiveness, error) {
		t.Fatal("generic liveness ran after exact Pi proof")
		return tmux.PaneLiveness{}, nil
	}

	if err := resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime); err != nil {
		t.Fatalf("resumeSessionWithRuntime() error = %v", err)
	}
	want := []string{"check-process", "update", "tab"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("exact Pi resume calls = %v, want %v", calls, want)
	}
}

func TestResumeSessionPiFailsBeforeMutationWhenExistingPaneScanFails(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	adapter, m, health := setupCodexResumeTransaction(t)
	m.Harness = "pi-cli"
	health.TmuxExists = true
	wantErr := errors.New("process scan unavailable")
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.loadManifest = func(context.Context, *dolt.Adapter, string, string) (*manifest.Manifest, error) {
		return m, nil
	}
	runtime.checkPiProcess = func(context.Context, string, string) (bool, error) {
		calls = append(calls, "check-process")
		return false, wantErr
	}
	runtime.checkLiveness = func(context.Context, string, string) (tmux.PaneLiveness, error) {
		t.Fatal("generic liveness ran after exact-process scan failure")
		return tmux.PaneLiveness{}, nil
	}

	err := resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime)
	if !errors.Is(err, wantErr) {
		t.Fatalf("resumeSessionWithRuntime() error = %v, want %v", err, wantErr)
	}
	if want := []string{"check-process"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("failed Pi scan calls = %v, want %v", calls, want)
	}
}

func TestPiResumeLivenessChecksUseCommandContext(t *testing.T) {
	health := &HealthStatus{TmuxExists: true, TmuxSessionName: "pi-resume"}
	for _, test := range []struct {
		name             string
		wantProcessCheck bool
		wantLiveness     bool
	}{
		{name: "exact process scan", wantProcessCheck: true},
		{name: "pane classification", wantProcessCheck: true, wantLiveness: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			processChecked := false
			livenessChecked := false
			runtime := resumeSessionRuntime{
				checkPiProcess: func(got context.Context, _, _ string) (bool, error) {
					processChecked = true
					if !test.wantLiveness {
						return false, got.Err()
					}
					return false, nil
				},
				checkLiveness: func(got context.Context, _, _ string) (tmux.PaneLiveness, error) {
					livenessChecked = true
					return tmux.PaneLiveness{}, got.Err()
				},
			}
			_, err := shouldSendHarnessResumeCommands(ctx, "pi-cli", health, runtime)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("shouldSendHarnessResumeCommands() error = %v, want context.Canceled", err)
			}
			if processChecked != test.wantProcessCheck || livenessChecked != test.wantLiveness {
				t.Fatalf("checks = process:%v liveness:%v, want process:%v liveness:%v", processChecked, livenessChecked, test.wantProcessCheck, test.wantLiveness)
			}
		})
	}
}

func TestWaitForResumedCodexRequiresProcessAndComposer(t *testing.T) {
	health := &HealthStatus{TmuxSessionName: "codex-resume"}
	processErr := errors.New("process missing")
	composerErr := errors.New("composer missing")

	t.Run("missing process after full startup window", func(t *testing.T) {
		composerCalled := false
		err := waitForResumedCodexWithRuntime(t.Context(), health, codexResumeReadinessRuntime{
			waitForProcess: func(session, process string, timeout time.Duration) error {
				if session != health.TmuxSessionName || process != "codex" || timeout != 60*time.Second {
					t.Fatalf("process wait = (%q, %q, %v)", session, process, timeout)
				}
				return processErr
			},
			waitForComposer: func(string, time.Duration) error {
				composerCalled = true
				return nil
			},
		})
		if !errors.Is(err, processErr) {
			t.Fatalf("error = %v, want %v", err, processErr)
		}
		if composerCalled {
			t.Fatal("composer wait ran without a ready Codex process")
		}
	})

	t.Run("missing composer", func(t *testing.T) {
		err := waitForResumedCodexWithRuntime(t.Context(), health, codexResumeReadinessRuntime{
			waitForProcess: func(string, string, time.Duration) error { return nil },
			waitForComposer: func(session string, timeout time.Duration) error {
				if session != health.TmuxSessionName || timeout != 60*time.Second {
					t.Fatalf("composer wait = (%q, %v)", session, timeout)
				}
				return composerErr
			},
		})
		if !errors.Is(err, composerErr) {
			t.Fatalf("error = %v, want %v", err, composerErr)
		}
	})

	t.Run("ready", func(t *testing.T) {
		var calls []string
		err := waitForResumedCodexWithRuntime(t.Context(), health, codexResumeReadinessRuntime{
			waitForProcess: func(_ string, _ string, timeout time.Duration) error {
				if timeout != 60*time.Second {
					t.Fatalf("process timeout = %v, want 60s", timeout)
				}
				calls = append(calls, "process")
				return nil
			},
			waitForComposer: func(_ string, timeout time.Duration) error {
				if timeout != 60*time.Second {
					t.Fatalf("composer timeout = %v, want 60s", timeout)
				}
				calls = append(calls, "composer")
				return nil
			},
		})
		if err != nil {
			t.Fatalf("waitForResumedCodexWithRuntime() error = %v", err)
		}
		if want := []string{"process", "composer"}; !reflect.DeepEqual(calls, want) {
			t.Fatalf("readiness calls = %v, want %v", calls, want)
		}
	})
}

func requireCodexResumeTmuxIntegration(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping isolated tmux integration test in short mode")
	}
	if os.Getenv("CI_SKIP_TMUX") == "true" {
		t.Skip("skipping isolated tmux integration test because CI_SKIP_TMUX=true")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not available")
	}
}

func TestResumeSessionCodexReadinessFailureRemovesIsolatedTmux(t *testing.T) {
	requireCodexResumeTmuxIntegration(t)
	setDetachedResumeTestGlobals(t, true)
	setupRegressionSocket(t)
	adapter, m, health := setupCodexResumeTransaction(t)
	requestedName := "codex.resume:rollback"
	createdName := tmux.SanitizeSessionName(requestedName)
	health.TmuxSessionName = requestedName
	t.Cleanup(func() {
		tmux.KillSession(createdName)
		tmux.KillSession(requestedName)
	})
	wantErr := errors.New("fake Codex composer missing")
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.createTmux = tmux.NewSessionWithIdentity
	runtime.killTmux = killCreatedResumeTmux
	runtime.wait = func(string, *HealthStatus) error { return wantErr }

	err := resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime)
	if !errors.Is(err, wantErr) {
		t.Fatalf("resumeSessionWithRuntime() error = %v, want %v", err, wantErr)
	}
	if health.TmuxSessionName != createdName {
		t.Fatalf("transaction tmux name = %q, want created name %q", health.TmuxSessionName, createdName)
	}
	exists, hasErr := tmux.HasSession(createdName)
	if hasErr != nil {
		t.Fatalf("HasSession() error = %v", hasErr)
	}
	if exists {
		t.Fatalf("new tmux session %q survived failed Codex readiness", createdName)
	}
}

func TestResumeSessionCodexRollbackReportsInaccessibleSocketAndPreservesHiddenTarget(t *testing.T) {
	requireCodexResumeTmuxIntegration(t)
	setDetachedResumeTestGlobals(t, true)
	socketPath := setupRegressionSocket(t)
	hiddenSocket := socketPath + ".hidden"
	adapter, m, health := setupCodexResumeTransaction(t)
	health.TmuxSessionName = "codex-resume-hidden-socket"
	primaryErr := errors.New("fake Codex readiness failure")
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.createTmux = tmux.NewSessionWithIdentity
	runtime.killTmux = killCreatedResumeTmux
	runtime.wait = func(string, *HealthStatus) error {
		if err := os.Rename(socketPath, hiddenSocket); err != nil {
			t.Fatalf("hide tmux socket: %v", err)
		}
		return primaryErr
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-S", hiddenSocket, "kill-server").Run()
	})

	err := resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime)
	if !errors.Is(err, primaryErr) {
		t.Fatalf("resumeSessionWithRuntime() error = %v, want primary readiness failure", err)
	}
	if err == nil || !strings.Contains(err.Error(), "tmux kill session") {
		t.Fatalf("rollback error = %v, want inaccessible-socket cleanup failure", err)
	}
	if checkErr := exec.Command("tmux", "-S", hiddenSocket, "has-session", "-t", tmux.FormatSessionTarget(health.TmuxSessionName)).Run(); checkErr != nil {
		t.Fatalf("hidden tmux target was not preserved for verification: %v", checkErr)
	}
}

func TestKillCreatedResumeTmuxPreservesSameNamedReplacement(t *testing.T) {
	requireCodexResumeTmuxIntegration(t)
	setupRegressionSocket(t)
	sessionName := "codex-resume-replacement"
	originalIdentity, err := tmux.NewSessionWithIdentity(sessionName, t.TempDir())
	if err != nil {
		t.Fatalf("NewSessionWithIdentity(original) error = %v", err)
	}
	if err := tmux.KillSessionIdentityChecked(originalIdentity); err != nil {
		t.Fatalf("KillSessionIdentityChecked(original) error = %v", err)
	}
	replacementIdentity, err := tmux.NewSessionWithIdentity(sessionName, t.TempDir())
	if err != nil {
		t.Fatalf("NewSessionWithIdentity(replacement) error = %v", err)
	}
	t.Cleanup(func() { _ = tmux.KillSessionIdentityChecked(replacementIdentity) })
	if replacementIdentity.ID == originalIdentity.ID {
		t.Fatalf("same-server replacement reused session ID %q", originalIdentity.ID)
	}

	if err := killCreatedResumeTmux(createdResumeTmux{Name: sessionName, Identity: originalIdentity}); err != nil {
		t.Fatalf("killCreatedResumeTmux(stale identity) error = %v", err)
	}
	exists, err := tmux.HasSessionIdentityStrict(replacementIdentity)
	if err != nil {
		t.Fatalf("HasSessionIdentityStrict(replacement) error = %v", err)
	}
	if !exists {
		t.Fatalf("same-named replacement %s was killed while compensating %s", replacementIdentity.ID, originalIdentity.ID)
	}
}

func waitForTmuxServerExit(t *testing.T, socketPath string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.DialTimeout("unix", socketPath, 50*time.Millisecond)
		if err != nil {
			return
		}
		_ = conn.Close()
		if time.Now().After(deadline) {
			t.Fatalf("tmux server at %s did not exit after kill-server", socketPath)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestKillCreatedResumeTmuxPreservesIDReusedAfterServerRestart(t *testing.T) {
	requireCodexResumeTmuxIntegration(t)
	socketPath := setupRegressionSocket(t)
	sessionName := "codex-resume-server-restart"
	originalIdentity, err := tmux.NewSessionWithIdentity(sessionName, t.TempDir())
	if err != nil {
		t.Fatalf("NewSessionWithIdentity(original) error = %v", err)
	}
	if err := exec.Command("tmux", "-S", socketPath, "kill-server").Run(); err != nil {
		t.Fatalf("kill original tmux server: %v", err)
	}
	waitForTmuxServerExit(t, socketPath)
	replacementIdentity, err := tmux.NewSessionWithIdentity(sessionName, t.TempDir())
	if err != nil {
		t.Fatalf("NewSessionWithIdentity(replacement) error = %v", err)
	}
	t.Cleanup(func() { _ = tmux.KillSessionIdentityChecked(replacementIdentity) })
	if replacementIdentity.ID != originalIdentity.ID {
		t.Fatalf("server restart did not reuse session ID: original=%q replacement=%q", originalIdentity.ID, replacementIdentity.ID)
	}
	if replacementIdentity.Token == originalIdentity.Token {
		t.Fatalf("server restart reused creation token %q", originalIdentity.Token)
	}

	if err := killCreatedResumeTmux(createdResumeTmux{Name: sessionName, Identity: originalIdentity}); err != nil {
		t.Fatalf("killCreatedResumeTmux(stale identity) error = %v", err)
	}
	exists, err := tmux.HasSessionIdentityStrict(replacementIdentity)
	if err != nil {
		t.Fatalf("HasSessionIdentityStrict(replacement) error = %v", err)
	}
	if !exists {
		t.Fatalf("server-restart replacement %s was killed while compensating stale token", replacementIdentity.ID)
	}
}

func TestWaitForResumedCodexDetectsFakeComposerInIsolatedTmux(t *testing.T) {
	requireCodexResumeTmuxIntegration(t)
	setupRegressionSocket(t)
	sessionName := "codex-resume-composer"
	if err := tmux.NewSession(sessionName, t.TempDir()); err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	t.Cleanup(func() { tmux.KillSession(sessionName) })

	fakeCodex := filepath.Join(t.TempDir(), "fake-codex")
	if err := os.WriteFile(fakeCodex, []byte("#!/bin/sh\nprintf 'OpenAI Codex\\n/model to change\\n›\\n'\nsleep 10\n"), 0o755); err != nil {
		t.Fatalf("write fake Codex: %v", err)
	}
	if err := tmux.SendCommand(sessionName, strconv.Quote(fakeCodex)); err != nil {
		t.Fatalf("SendCommand() error = %v", err)
	}

	err := waitForResumedCodexWithRuntime(t.Context(), &HealthStatus{TmuxSessionName: sessionName}, codexResumeReadinessRuntime{
		waitForProcess:  func(string, string, time.Duration) error { return nil },
		waitForComposer: tmux.WaitForCodexPrompt,
	})
	if err != nil {
		t.Fatalf("waitForResumedCodexWithRuntime() error = %v", err)
	}
}
