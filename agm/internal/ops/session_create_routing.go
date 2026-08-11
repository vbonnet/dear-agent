package ops

import (
	"context"
	"errors"
	"fmt"
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

// CreateSessionRouted adds agy (Antigravity/Gemini) spawn resilience around the
// self-contained CreateSessionWithContext create path used by the MCP/Dispatch
// surface.
//
// It is deliberately NOT used from the interactive CLI create flow: that flow
// wraps CreateSessionWithContext in per-invocation, non-re-entrant setup
// (sandbox provisioning, a preparer bound to the requested harness, one-shot
// circuit-breaker admission, and post-create dispatch keyed on the global
// harness), so re-running or harness-switching it mid-flow is unsafe. The MCP
// path builds a fresh request and calls this once, and each attempt is a fully
// rolled-back create, so retry and fallback are safe there.
//
// Behaviours (agy only; non-agy harnesses and reused-tmux requests pass straight
// through, as do agy requests that opt out via SpawnRetries==0 and no
// FallbackHarness):
//
//  1. Bounded retry-with-backoff on a transient agy identity-discovery race
//     (AGM-011 agy.identity + ErrConversationNotFound). Antigravity persists its
//     conversation DB only after the model first responds, so under provider
//     throttle the discovery window times out. Each failed attempt is fully
//     rolled back (tmux, DB registration, name reservation), so a retry
//     re-spawns cleanly. If a rollback itself fails, the next attempt collides
//     on the still-registered session and returns a non-retryable error, which
//     ends the loop rather than retrying onto dirty state.
//
//  2. Fallback to req.FallbackHarness (typically codex-cli) once retries are
//     exhausted, re-dispatching the identical request on the freed session name
//     with the model re-defaulted for the fallback harness.
func CreateSessionRouted(ctx context.Context, opCtx *OpContext, req *CreateSessionRequest) (*CreateSessionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !agySpawnRoutingApplies(req) {
		return CreateSessionWithContext(ctx, opCtx, req)
	}
	// Resolve and validate the fallback harness up front so an unsupported value
	// is rejected before any agy attempt is launched.
	fallback, err := resolveFallbackHarness(req.FallbackHarness)
	if err != nil {
		return nil, err
	}

	attempts := max(req.SpawnRetries, 0)
	baseDelay := req.SpawnRetryBaseDelay
	if baseDelay <= 0 {
		baseDelay = defaultSpawnRetryBaseDelay
	}

	var lastErr error
	for attempt := 0; attempt <= attempts; attempt++ {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		result, spawnErr := CreateSessionWithContext(ctx, opCtx, req)
		if spawnErr == nil {
			return result, nil
		}
		lastErr = spawnErr
		if !isRetryableAgySpawnError(spawnErr) {
			return nil, spawnErr
		}
		if attempt < attempts {
			delay := spawnRetryBackoff(baseDelay, attempt)
			debug.Log("agy spawn attempt %d/%d hit a transient discovery race; retrying in %s: %v",
				attempt+1, attempts+1, delay, spawnErr)
			if waitErr := sleepForSpawnRetry(ctx, delay); waitErr != nil {
				return nil, waitErr
			}
		}
	}

	if fallback == "" {
		return nil, lastErr
	}
	// The final retryable attempt (and its rollback) may have run under a context
	// that is now cancelled; do not enter a fresh create if so.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	return createFallbackSession(ctx, opCtx, req, fallback, lastErr)
}

// agySpawnRoutingApplies reports whether retry/fallback routing should wrap this
// request. Only a self-contained agy create that opts in qualifies: a nil
// request, a non-agy harness, a reused (rollback-preserved) tmux session, or an
// opt-out (no retries and no fallback) all bypass routing.
func agySpawnRoutingApplies(req *CreateSessionRequest) bool {
	if req == nil || agent.NormalizeHarnessName(req.Harness) != "agy" || req.ReuseExistingTmux {
		return false
	}
	return req.SpawnRetries > 0 || strings.TrimSpace(req.FallbackHarness) != ""
}

// resolveFallbackHarness normalizes and validates the configured fallback
// harness. It returns "" when there is no usable fallback (unset, or agy
// itself), and an error for an unsupported value.
func resolveFallbackHarness(raw string) (string, error) {
	fallback := agent.NormalizeHarnessName(raw)
	if fallback == "" || fallback == "agy" {
		return "", nil
	}
	if err := agent.ValidateHarnessName(fallback); err != nil {
		return "", ErrInvalidInput("fallback-harness", err.Error())
	}
	return fallback, nil
}

// createFallbackSession re-dispatches the request on the fallback harness after
// agy attempts are exhausted, re-defaulting the model for that harness (the agy
// model is invalid elsewhere) and clearing the routing fields. If the fallback
// itself fails it returns both the original agy-exhaustion error and the
// fallback error joined, so the throttle cause is not lost behind (e.g.) a
// "codex unavailable".
func createFallbackSession(ctx context.Context, opCtx *OpContext, req *CreateSessionRequest, fallback string, agyErr error) (*CreateSessionResult, error) {
	debug.Log("agy spawn exhausted under throttle; falling back to %s for session %q", fallback, req.Title)
	fallbackReq := *req
	fallbackReq.Harness = fallback
	fallbackReq.Model = defaultModelForCreateSession(fallback)
	fallbackReq.SpawnRetries = 0
	fallbackReq.FallbackHarness = ""
	result, err := CreateSessionWithContext(ctx, opCtx, &fallbackReq)
	if err != nil {
		return result, errors.Join(
			fmt.Errorf("agy spawn exhausted under throttle: %w", agyErr),
			fmt.Errorf("%s fallback also failed: %w", fallback, err),
		)
	}
	return result, nil
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
	for range attempt {
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
