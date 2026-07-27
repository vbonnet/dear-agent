package session

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

func TestRealTmuxResumeReadinessReturnsCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := NewRealTmux().WaitForResumeReady(ctx, "unused", "opencode-cli", "", time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitForResumeReady() error = %v, want context.Canceled", err)
	}
}

func TestCodexResumeReadinessRequiresProcessThenComposer(t *testing.T) {
	var calls []string
	result, err := waitForCodexResumeReady(
		t.Context(),
		"codex-resume",
		time.Minute,
		func(_ context.Context, sessionName, process string, timeout time.Duration) error {
			calls = append(calls, "process")
			if sessionName != "codex-resume" || process != "codex" || timeout != time.Minute {
				t.Fatalf("process wait = (%q, %q, %v)", sessionName, process, timeout)
			}
			return nil
		},
		func(_ context.Context, sessionName string, timeout time.Duration) error {
			calls = append(calls, "composer")
			if sessionName != "codex-resume" || timeout != time.Minute {
				t.Fatalf("composer wait = (%q, %v)", sessionName, timeout)
			}
			return nil
		},
	)
	if err != nil || len(result.Warnings) != 0 {
		t.Fatalf("waitForCodexResumeReady() = (%#v, %v)", result, err)
	}
	if want := []string{"process", "composer"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("readiness calls = %v, want %v", calls, want)
	}
}

func TestCodexResumeReadinessStopsBeforeComposerWithoutProcess(t *testing.T) {
	wantErr := errors.New("process unavailable")
	composerCalled := false
	_, err := waitForCodexResumeReady(
		t.Context(),
		"codex-resume",
		time.Minute,
		func(context.Context, string, string, time.Duration) error { return wantErr },
		func(context.Context, string, time.Duration) error {
			composerCalled = true
			return nil
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("waitForCodexResumeReady() error = %v, want %v", err, wantErr)
	}
	if composerCalled {
		t.Fatal("composer wait ran without a ready Codex process")
	}
}

func TestAgyResumeReadinessPreservesOnboardingAndSlowStartupPolicy(t *testing.T) {
	t.Run("onboarding is terminal", func(t *testing.T) {
		_, err := waitForAgyResumeReady(
			t.Context(),
			"agy-resume",
			time.Minute,
			func(context.Context, string, time.Duration) error { return tmux.ErrAgyOnboardingRequired },
		)
		if !errors.Is(err, tmux.ErrAgyOnboardingRequired) {
			t.Fatalf("waitForAgyResumeReady() error = %v, want onboarding failure", err)
		}
	})

	t.Run("slow startup remains a warning", func(t *testing.T) {
		result, err := waitForAgyResumeReady(
			t.Context(),
			"agy-resume",
			time.Minute,
			func(context.Context, string, time.Duration) error { return errors.New("startup timeout") },
		)
		if err != nil || len(result.Warnings) != 1 {
			t.Fatalf("waitForAgyResumeReady() = (%#v, %v), want one warning", result, err)
		}
	})
}
