package ops

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/agysession"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

func TestIsRetryableAgySpawnError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "agy discovery conversation-not-found is retryable",
			err:  ErrStorageError("agy.identity.discover", agysession.ErrConversationNotFound),
			want: true,
		},
		{
			name: "agy discovery wrapping conversation-not-found is retryable",
			err:  ErrStorageError("agy.identity.discover", fmt.Errorf("discover failed: %w", agysession.ErrConversationNotFound)),
			want: true,
		},
		{
			name: "agy discovery pre-create-conversation race is retryable",
			err:  ErrStorageError("agy.identity.discover", errors.New(`provider still reports pre-create conversation "abc"`)),
			want: true,
		},
		{
			name: "agy identity non-race error is not retryable",
			err:  ErrStorageError("agy.identity.discover", errors.New("provider still reports stale identity")),
			want: false,
		},
		{
			name: "AGM-011 on a different operation is not retryable",
			err:  ErrStorageError("storage.CreateSession", agysession.ErrConversationNotFound),
			want: false,
		},
		{
			name: "non-storage error is not retryable",
			err:  ErrInvalidInput("harness", "bad"),
			want: false,
		},
		{
			name: "nil is not retryable",
			err:  nil,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryableAgySpawnError(tt.err); got != tt.want {
				t.Errorf("isRetryableAgySpawnError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSpawnRetryBackoff(t *testing.T) {
	base := time.Second
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{10, maxSpawnRetryDelay}, // capped
	}
	for _, c := range cases {
		if got := spawnRetryBackoff(base, c.attempt); got != c.want {
			t.Errorf("spawnRetryBackoff(%v, %d) = %v, want %v", base, c.attempt, got, c.want)
		}
	}
}

func TestCreateSessionRouted_NonAgyPassesThrough(t *testing.T) {
	dir := t.TempDir()
	tmuxMock := session.NewMockTmux()
	store := &createMockStorage{}
	result, err := CreateSessionRouted(t.Context(), &OpContext{Tmux: tmuxMock, Storage: store}, &CreateSessionRequest{
		Cwd: dir, Prompt: "p", Title: "cc-passthrough", Harness: "claude-code", Model: "opus",
		// Retry/fallback are set but must be ignored for a non-agy harness.
		SpawnRetries: 3, SpawnRetryBaseDelay: time.Millisecond, FallbackHarness: "codex-cli",
	})
	if err != nil {
		t.Fatalf("CreateSessionRouted: %v", err)
	}
	if result.Harness != "claude-code" {
		t.Fatalf("harness = %q, want claude-code", result.Harness)
	}
}

func TestCreateSessionRouted_RetriesTransientAgyDiscoveryRaceThenSucceeds(t *testing.T) {
	dir := t.TempDir()
	tmuxMock := session.NewMockTmux()
	store := &createMockStorage{}
	tracker := successfulCreateTestAgyIdentityTracker()
	var discoverCalls int
	tracker.discover = func(_ context.Context, workDir, _ string) (*agysession.Metadata, error) {
		discoverCalls++
		if discoverCalls == 1 {
			return nil, agysession.ErrConversationNotFound // transient throttle race
		}
		return &agysession.Metadata{ConversationID: "new-native-id", WorkspacePath: workDir}, nil
	}

	result, err := CreateSessionRouted(t.Context(), &OpContext{
		Tmux: tmuxMock, Storage: store, CreationRuntime: &createTestRuntime{}, AgyCreateIdentityTracker: tracker,
	}, &CreateSessionRequest{
		Cwd: dir, Prompt: "p", Title: "agy-retry", Harness: "agy", Model: "3.5-flash-low",
		SpawnRetries: 3, SpawnRetryBaseDelay: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("CreateSessionRouted after transient race: %v", err)
	}
	if result.Harness != "agy" {
		t.Fatalf("harness = %q, want agy (retry must not change harness)", result.Harness)
	}
	if discoverCalls != 2 {
		t.Fatalf("discover called %d times, want 2 (one race, one success)", discoverCalls)
	}
}

func TestCreateSessionRouted_FallsBackToCodexAfterAgyExhausted(t *testing.T) {
	dir := t.TempDir()
	tmuxMock := session.NewMockTmux()
	store := &createMockStorage{}
	tracker := successfulCreateTestAgyIdentityTracker()
	tracker.discover = func(context.Context, string, string) (*agysession.Metadata, error) {
		return nil, agysession.ErrConversationNotFound // agy stays throttled
	}

	result, err := CreateSessionRouted(t.Context(), &OpContext{
		Tmux: tmuxMock, Storage: store, CreationRuntime: &createTestRuntime{}, AgyCreateIdentityTracker: tracker,
	}, &CreateSessionRequest{
		Cwd: dir, Prompt: "p", Title: "agy-fallback", Harness: "agy", Model: "3.5-flash-low",
		SpawnRetries: 0, SpawnRetryBaseDelay: time.Millisecond, FallbackHarness: "codex-cli",
		// The clone inherits this; keep the headless fallback off the 20s
		// codex remote-control bridge probe that has no server in tests.
		SkipCodexRemoteControl: true,
	})
	if err != nil {
		t.Fatalf("CreateSessionRouted fallback: %v", err)
	}
	if result.Harness != "codex-cli" {
		t.Fatalf("harness = %q, want codex-cli after fallback", result.Harness)
	}
	if len(store.created) != 1 || store.created[0].Harness != "codex-cli" {
		t.Fatalf("stored manifest = %+v, want exactly one codex-cli (no residual agy registration)", store.created)
	}
}

func TestCreateSessionRouted_FallbackFailurePreservesAgyError(t *testing.T) {
	dir := t.TempDir()
	tmuxMock := session.NewMockTmux()
	store := &createMockStorage{}
	tracker := successfulCreateTestAgyIdentityTracker()
	tracker.discover = func(context.Context, string, string) (*agysession.Metadata, error) {
		return nil, agysession.ErrConversationNotFound
	}
	// agy launches fine but never discovers (throttle); the codex fallback fails
	// to launch. The surfaced error must retain both causes.
	runtime := &createTestRuntime{launch: func(_ context.Context, spec HarnessLaunchSpec) (CreateSessionLaunchResult, error) {
		if spec.Harness == "codex-cli" {
			return CreateSessionLaunchResult{}, errors.New("codex unavailable")
		}
		return CreateSessionLaunchResult{}, nil
	}}
	_, err := CreateSessionRouted(t.Context(), &OpContext{
		Tmux: tmuxMock, Storage: store, CreationRuntime: runtime, AgyCreateIdentityTracker: tracker,
	}, &CreateSessionRequest{
		Cwd: dir, Prompt: "p", Title: "agy-fb-fail", Harness: "agy", Model: "3.5-flash-low",
		SpawnRetries: 0, SpawnRetryBaseDelay: time.Millisecond, FallbackHarness: "codex-cli",
		SkipCodexRemoteControl: true,
	})
	if err == nil {
		t.Fatal("expected an error when both agy and the fallback fail")
	}
	if !errors.Is(err, agysession.ErrConversationNotFound) {
		t.Errorf("error must preserve the agy discovery race, got: %v", err)
	}
	if !strings.Contains(err.Error(), "codex unavailable") {
		t.Errorf("error must include the fallback failure, got: %v", err)
	}
}

func TestCreateSessionRouted_ReusedTmuxPassesThroughWithoutRetry(t *testing.T) {
	dir := t.TempDir()
	tmuxMock := session.NewMockTmux()
	store := &createMockStorage{}
	tracker := successfulCreateTestAgyIdentityTracker()
	var discoverCalls int
	tracker.discover = func(context.Context, string, string) (*agysession.Metadata, error) {
		discoverCalls++
		return nil, agysession.ErrConversationNotFound
	}
	// A reused tmux session is preserved on rollback, so routing must NOT retry
	// or fall back into it — the request passes straight through.
	_, err := CreateSessionRouted(t.Context(), &OpContext{
		Tmux: tmuxMock, Storage: store, CreationRuntime: &createTestRuntime{}, AgyCreateIdentityTracker: tracker,
	}, &CreateSessionRequest{
		Cwd: dir, Prompt: "p", Title: "agy-reuse", Harness: "agy", Model: "3.5-flash-low",
		SpawnRetries: 3, SpawnRetryBaseDelay: time.Millisecond, FallbackHarness: "codex-cli",
		ReuseExistingTmux: true,
	})
	if err == nil {
		t.Fatal("expected the discovery error to surface")
	}
	if discoverCalls != 1 {
		t.Fatalf("discover called %d times, want 1 (reused tmux must not retry)", discoverCalls)
	}
}

func TestCreateSessionRouted_InvalidFallbackHarnessRejectedBeforeLaunch(t *testing.T) {
	dir := t.TempDir()
	tmuxMock := session.NewMockTmux()
	store := &createMockStorage{}
	tracker := successfulCreateTestAgyIdentityTracker()
	var discoverCalls int
	tracker.discover = func(context.Context, string, string) (*agysession.Metadata, error) {
		discoverCalls++
		return successfulCreateTestAgyIdentityTracker().discover(context.Background(), "", "")
	}
	_, err := CreateSessionRouted(t.Context(), &OpContext{
		Tmux: tmuxMock, Storage: store, CreationRuntime: &createTestRuntime{}, AgyCreateIdentityTracker: tracker,
	}, &CreateSessionRequest{
		Cwd: dir, Prompt: "p", Title: "agy-badfb", Harness: "agy", Model: "3.5-flash-low",
		SpawnRetries: 1, SpawnRetryBaseDelay: time.Millisecond, FallbackHarness: "not-a-harness",
	})
	if err == nil {
		t.Fatal("expected an invalid-fallback error")
	}
	if discoverCalls != 0 {
		t.Fatalf("discover called %d times, want 0 (fallback validation must precede any agy launch)", discoverCalls)
	}
}

func TestCreateSessionRouted_NonRetryableAgyErrorDoesNotRetryOrFallback(t *testing.T) {
	dir := t.TempDir()
	tmuxMock := session.NewMockTmux()
	store := &createMockStorage{}
	tracker := successfulCreateTestAgyIdentityTracker()
	var discoverCalls int
	tracker.discover = func(context.Context, string, string) (*agysession.Metadata, error) {
		discoverCalls++
		return nil, errors.New("provider still reports stale identity") // non-retryable
	}

	_, err := CreateSessionRouted(t.Context(), &OpContext{
		Tmux: tmuxMock, Storage: store, CreationRuntime: &createTestRuntime{}, AgyCreateIdentityTracker: tracker,
	}, &CreateSessionRequest{
		Cwd: dir, Prompt: "p", Title: "agy-hardfail", Harness: "agy", Model: "3.5-flash-low",
		SpawnRetries: 3, SpawnRetryBaseDelay: time.Millisecond, FallbackHarness: "codex-cli",
	})
	if err == nil {
		t.Fatal("expected a hard error, got nil")
	}
	if discoverCalls != 1 {
		t.Fatalf("discover called %d times, want 1 (no retry on a non-retryable error)", discoverCalls)
	}
	for _, m := range store.created {
		if m.Harness == "codex-cli" {
			t.Fatal("must not fall back to codex on a non-retryable agy error")
		}
	}
}
