package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

// TestExitCodeFromError_MapsCorrectly exercises the OpError.Status → exit code
// translation table across every mapped class plus the generic fallbacks.
func TestExitCodeFromError_MapsCorrectly(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   int
	}{
		{"unauthenticated 401 -> auth", 401, ExitAuth},
		{"forbidden 403 -> auth", 403, ExitAuth},
		{"bad request 400 -> bad input", 400, ExitBadInput},
		{"unprocessable 422 -> bad input", 422, ExitBadInput},
		{"not found 404 -> not found", 404, ExitNotFound},
		{"conflict 409 -> state conflict", 409, ExitStateConflict},
		{"server error 500 -> generic", 500, ExitGeneric},
		{"unavailable 503 -> generic", 503, ExitGeneric},
		{"unknown 0 -> generic", 0, ExitGeneric},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := &ops.OpError{Status: tc.status, Code: "AGM-TEST"}
			if got := exitCodeFromError(err); got != tc.want {
				t.Errorf("exitCodeFromError(status=%d) = %d, want %d", tc.status, got, tc.want)
			}
		})
	}
}

// TestExitCodeFromError_Nil confirms a successful command exits 0.
func TestExitCodeFromError_Nil(t *testing.T) {
	if got := exitCodeFromError(nil); got != ExitOK {
		t.Errorf("exitCodeFromError(nil) = %d, want %d", got, ExitOK)
	}
}

// TestExitCodeFromError_PlainError confirms non-OpError failures are generic.
func TestExitCodeFromError_PlainError(t *testing.T) {
	if got := exitCodeFromError(errors.New("boom")); got != ExitGeneric {
		t.Errorf("exitCodeFromError(plain) = %d, want %d", got, ExitGeneric)
	}
}

// TestExitCodeFromError_WrappedOpError confirms the mapping survives wrapping,
// since errors propagate through several %w layers before reaching run().
func TestExitCodeFromError_WrappedOpError(t *testing.T) {
	base := ops.ErrSessionNotFound("ghost")
	wrapped := fmt.Errorf("kill failed: %w", base)
	if got := exitCodeFromError(wrapped); got != ExitNotFound {
		t.Errorf("exitCodeFromError(wrapped 404) = %d, want %d", got, ExitNotFound)
	}
}

// TestExitCodeFromError_ExitError confirms the exitError wrapper produced by
// handleError carries its code through to the top level.
func TestExitCodeFromError_ExitError(t *testing.T) {
	ee := &exitError{code: ExitStateConflict, msg: "AGM-002"}
	if got := exitCodeFromError(ee); got != ExitStateConflict {
		t.Errorf("exitCodeFromError(exitError) = %d, want %d", got, ExitStateConflict)
	}
	if ee.Error() != "AGM-002" {
		t.Errorf("exitError.Error() = %q, want %q", ee.Error(), "AGM-002")
	}
}

// TestHandleError_ArchiveSession_Returns4 covers the headline agent scenario:
// killing/modifying an archived session yields a state-conflict exit (4).
func TestHandleError_ArchiveSession_Returns4(t *testing.T) {
	wrapped := handleError(ops.ErrSessionArchived("old-session"))
	if got := exitCodeFromError(wrapped); got != ExitStateConflict {
		t.Errorf("archived session exit = %d, want %d", got, ExitStateConflict)
	}
}

// TestHandleError_NotFoundSession_Returns5 covers killing a session that does
// not exist: agents can distinguish this (5) from archived (4) on $?.
func TestHandleError_NotFoundSession_Returns5(t *testing.T) {
	wrapped := handleError(ops.ErrSessionNotFound("ghost"))
	if got := exitCodeFromError(wrapped); got != ExitNotFound {
		t.Errorf("not-found session exit = %d, want %d", got, ExitNotFound)
	}
}

// TestHandleError_AuthFailure_Returns2 covers a permission/auth error (403)
// mapping to the auth exit code (2).
func TestHandleError_AuthFailure_Returns2(t *testing.T) {
	authErr := &ops.OpError{
		Status: 403,
		Type:   "permission/denied",
		Code:   ops.ErrCodePermissionDenied,
		Title:  "Permission denied",
		Detail: "not authorized",
	}
	wrapped := handleError(authErr)
	if got := exitCodeFromError(wrapped); got != ExitAuth {
		t.Errorf("auth failure exit = %d, want %d", got, ExitAuth)
	}
}

// TestHandleError_BadInput_Returns3 covers a validation error (400) mapping to
// the bad-input exit code (3).
func TestHandleError_BadInput_Returns3(t *testing.T) {
	wrapped := handleError(ops.ErrInvalidInput("name", "must not be empty"))
	if got := exitCodeFromError(wrapped); got != ExitBadInput {
		t.Errorf("bad input exit = %d, want %d", got, ExitBadInput)
	}
}

// TestHandleError_PreservesCodeString confirms handleError still renders as the
// stable OpError code so cobra's printed "Error:" line is unchanged.
func TestHandleError_PreservesCodeString(t *testing.T) {
	wrapped := handleError(ops.ErrSessionNotFound("ghost"))
	if wrapped == nil {
		t.Fatal("handleError returned nil for an OpError")
	}
	if wrapped.Error() != ops.ErrCodeSessionNotFound {
		t.Errorf("handleError error string = %q, want %q", wrapped.Error(), ops.ErrCodeSessionNotFound)
	}
}

// TestHandleError_PassesThroughPlainErrors confirms non-OpError errors are
// returned unchanged (no exitError wrapping, no exit-code remap).
func TestHandleError_PassesThroughPlainErrors(t *testing.T) {
	plain := errors.New("disk full")
	if got := handleError(plain); !errors.Is(got, plain) {
		t.Errorf("handleError(plain) = %v, want passthrough of %v", got, plain)
	}
}
