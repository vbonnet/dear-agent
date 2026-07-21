package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
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

func recordingResumeRuntime(calls *[]string) resumeSessionRuntime {
	record := func(call string) { *calls = append(*calls, call) }
	return resumeSessionRuntime{
		createTmux: func(string, string) error { record("create"); return nil },
		killTmux:   func(string) error { record("kill"); return nil },
		dispatch: func(*dolt.Adapter, *manifest.Manifest, string, *HealthStatus) error {
			record("dispatch")
			return nil
		},
		wait: func(string, *HealthStatus) error { record("wait"); return nil },
		restorePermission: func(string, *manifest.Manifest, *HealthStatus) {
			record("restore")
		},
		updateActivity: func(*dolt.Adapter, string, string) error { record("update"); return nil },
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
	want := []string{"create", "dispatch", "wait", "restore", "update", "tab", "prompt"}
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

func TestResumeSessionCodexJoinsCleanupFailure(t *testing.T) {
	setDetachedResumeTestGlobals(t, true)
	adapter, m, health := setupCodexResumeTransaction(t)
	primaryErr := errors.New("composer missing")
	cleanupErr := errors.New("tmux target remained")
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.wait = func(string, *HealthStatus) error { return primaryErr }
	runtime.killTmux = func(string) error { return cleanupErr }

	err := resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime)
	if !errors.Is(err, primaryErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("error = %v, want joined primary and cleanup failures", err)
	}
}

func TestResumeSessionPreservesPreexistingTmuxOnLaterFailure(t *testing.T) {
	setDetachedResumeTestGlobals(t, false)
	adapter, m, health := setupCodexResumeTransaction(t)
	health.TmuxExists = true
	attachErr := errors.New("attach failed")
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.createTmux = func(string, string) error {
		t.Fatal("create called for pre-existing tmux session")
		return nil
	}
	runtime.killTmux = func(string) error {
		t.Fatal("pre-existing tmux session was killed")
		return nil
	}
	runtime.attachTmux = func(string) error { return attachErr }

	err := resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime)
	if !errors.Is(err, attachErr) {
		t.Fatalf("error = %v, want %v", err, attachErr)
	}
}

func TestWaitForResumedCodexRequiresProcessAndComposer(t *testing.T) {
	health := &HealthStatus{TmuxSessionName: "codex-resume"}
	processErr := errors.New("process missing")
	composerErr := errors.New("composer missing")

	t.Run("missing process", func(t *testing.T) {
		composerCalled := false
		err := waitForResumedCodexWithRuntime(t.Context(), health, codexResumeReadinessRuntime{
			waitForProcess: func(session, process string, timeout time.Duration) error {
				if session != health.TmuxSessionName || process != "codex" || timeout != 15*time.Second {
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
			waitForProcess: func(string, string, time.Duration) error {
				calls = append(calls, "process")
				return nil
			},
			waitForComposer: func(string, time.Duration) error {
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
	health.TmuxSessionName = "codex-resume-rollback"
	wantErr := errors.New("fake Codex composer missing")
	var calls []string
	runtime := recordingResumeRuntime(&calls)
	runtime.createTmux = tmux.NewSession
	runtime.killTmux = killCreatedResumeTmux
	runtime.wait = func(string, *HealthStatus) error { return wantErr }

	err := resumeSessionWithRuntime(t.Context(), adapter, m.SessionID, "manifest.yaml", m.Harness, health, runtime)
	if !errors.Is(err, wantErr) {
		t.Fatalf("resumeSessionWithRuntime() error = %v, want %v", err, wantErr)
	}
	exists, hasErr := tmux.HasSession(health.TmuxSessionName)
	if hasErr != nil {
		t.Fatalf("HasSession() error = %v", hasErr)
	}
	if exists {
		t.Fatalf("new tmux session %q survived failed Codex readiness", health.TmuxSessionName)
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
	if err := os.WriteFile(fakeCodex, []byte("#!/bin/sh\nprintf 'OpenAI Codex\\n/model to change\\n'\nsleep 10\n"), 0o755); err != nil {
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
