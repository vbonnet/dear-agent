package ops

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/agysession"
	"github.com/vbonnet/dear-agent/agm/internal/debug"
)

// defaultSpawnRetryBaseDelay seeds the exponential backoff used when retrying a
// transient agy spawn failure.
const defaultSpawnRetryBaseDelay = 2 * time.Second

// maxSpawnRetryDelay caps the exponential backoff so a large retry budget
// cannot produce an unbounded wait.
const maxSpawnRetryDelay = 30 * time.Second

// CreateSessionRouted wraps CreateSessionWithContext with two routing behaviours
// that make the throttle-prone agy (Antigravity/Gemini) harness usable instead
// of a hard failure:
//
//  1. Bounded retry-with-backoff on a transient agy spawn failure — the
//     Antigravity identity-discovery race. Antigravity persists its
//     conversation DB only after the model first responds, so under provider
//     throttle the discovery window times out and surfaces as an AGM-011
//     agy.identity error. Each failed attempt is already fully rolled back by
//     CreateSessionWithContext's deferred cleanup (tmux, DB registration and the
//     name reservation are released), so every retry re-spawns cleanly and
//     gives the throttled model another chance to persist its DB.
//
//  2. Fallback to req.FallbackHarness (typically codex-cli) once the agy retries
//     are exhausted, re-dispatching the identical request on the fallback
//     harness. Because the failed agy attempt already released its reservation,
//     the fallback reuses the same session name with no collision; the
//     agy-specific model is replaced with the fallback harness default.
//
// Non-agy harnesses are dispatched straight through unchanged, as are agy
// requests that opt out (SpawnRetries==0 and no FallbackHarness).
func CreateSessionRouted(ctx context.Context, opCtx *OpContext, req *CreateSessionRequest) (*CreateSessionResult, error) {
	if req == nil || agent.NormalizeHarnessName(req.Harness) != "agy" {
		return CreateSessionWithContext(ctx, opCtx, req)
	}
	if req.SpawnRetries <= 0 && strings.TrimSpace(req.FallbackHarness) == "" {
		return CreateSessionWithContext(ctx, opCtx, req)
	}

	attempts := req.SpawnRetries
	if attempts < 0 {
		attempts = 0
	}
	baseDelay := req.SpawnRetryBaseDelay
	if baseDelay <= 0 {
		baseDelay = defaultSpawnRetryBaseDelay
	}

	var lastErr error
	for attempt := 0; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result, err := CreateSessionWithContext(ctx, opCtx, req)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !isRetryableAgySpawnError(err) {
			return nil, err
		}
		if attempt < attempts {
			delay := spawnRetryBackoff(baseDelay, attempt)
			debug.Log("agy spawn attempt %d/%d hit a transient discovery race; retrying in %s: %v",
				attempt+1, attempts+1, delay, err)
			if waitErr := sleepForSpawnRetry(ctx, delay); waitErr != nil {
				return nil, waitErr
			}
		}
	}

	fallback := agent.NormalizeHarnessName(req.FallbackHarness)
	if fallback == "" || fallback == "agy" {
		return nil, lastErr
	}
	debug.Log("agy spawn exhausted %d attempt(s) under throttle; falling back to %s for session %q",
		attempts+1, fallback, req.Title)
	fallbackReq := *req
	fallbackReq.Harness = fallback
	// The agy model is invalid for the fallback harness; re-default it to the
	// fallback's canonical model (same helper CreateSessionWithContext uses).
	fallbackReq.Model = defaultModelForCreateSession(fallback)
	fallbackReq.SpawnRetries = 0
	fallbackReq.FallbackHarness = ""
	return CreateSessionWithContext(ctx, opCtx, &fallbackReq)
}

// isRetryableAgySpawnError reports whether err is a transient agy spawn failure
// worth retrying — the Antigravity identity-discovery race under provider
// throttle — rather than a genuine storage/config failure a retry cannot help.
// It matches the stable AGM-011 storage code scoped to an agy.identity
// operation, then confirms the underlying cause is the discovery race.
func isRetryableAgySpawnError(err error) bool {
	var opErr *OpError
	if !errors.As(err, &opErr) || opErr.Code != ErrCodeStorageError {
		return false
	}
	if !strings.HasPrefix(opErr.Instance, "agy.identity") {
		return false
	}
	if errors.Is(err, agysession.ErrConversationNotFound) {
		return true
	}
	// The throttled "provider still reports the pre-create conversation" case is
	// a plain formatted error, not a sentinel; match it by its stable text,
	// which ErrStorageError carries verbatim in Detail.
	return strings.Contains(opErr.Detail, "provider still reports pre-create conversation")
}

// spawnRetryBackoff is capped exponential backoff: base * 2^attempt, never
// exceeding maxSpawnRetryDelay.
func spawnRetryBackoff(base time.Duration, attempt int) time.Duration {
	delay := base
	for i := 0; i < attempt; i++ {
		delay *= 2
		if delay >= maxSpawnRetryDelay {
			return maxSpawnRetryDelay
		}
	}
	if delay > maxSpawnRetryDelay {
		return maxSpawnRetryDelay
	}
	return delay
}

// sleepForSpawnRetry waits for delay or until the context is cancelled.
func sleepForSpawnRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
