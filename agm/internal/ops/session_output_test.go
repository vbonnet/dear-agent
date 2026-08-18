package ops

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

func TestGetSessionOutput_LivePane(t *testing.T) {
	m := testManifest("worker-out", "", time.Time{})
	storage := &mockStorage{sessions: []*manifest.Manifest{m}}
	tmux := session.NewMockTmux()
	tmux.Sessions["worker-out"] = true
	tmux.PaneContents = map[string]string{"worker-out": "build ok\nall tests passed\n"}

	result, err := GetSessionOutput(&OpContext{Storage: storage, Tmux: tmux}, &GetSessionOutputRequest{Identifier: "worker-out"})
	if err != nil {
		t.Fatalf("GetSessionOutput: %v", err)
	}
	if result.Source != "live-pane" {
		t.Fatalf("source = %q, want live-pane", result.Source)
	}
	if !strings.Contains(result.Output, "all tests passed") {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestGetSessionOutput_FallsBackToFinalCapture(t *testing.T) {
	m := testManifest("worker-done", "", time.Time{})
	m.FinalOutput = "RESULT: shipped PR #7"
	m.FinalOutputAt = time.Now()
	storage := &mockStorage{sessions: []*manifest.Manifest{m}}
	tmux := session.NewMockTmux() // tmux session gone

	result, err := GetSessionOutput(&OpContext{Storage: storage, Tmux: tmux}, &GetSessionOutputRequest{Identifier: "worker-done"})
	if err != nil {
		t.Fatalf("GetSessionOutput: %v", err)
	}
	if result.Source != "final-capture" {
		t.Fatalf("source = %q, want final-capture", result.Source)
	}
	if result.Output != "RESULT: shipped PR #7" {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestGetSessionOutput_NoOutputAnywhere(t *testing.T) {
	m := testManifest("worker-empty", "", time.Time{})
	storage := &mockStorage{sessions: []*manifest.Manifest{m}}
	tmux := session.NewMockTmux()

	if _, err := GetSessionOutput(&OpContext{Storage: storage, Tmux: tmux}, &GetSessionOutputRequest{Identifier: "worker-empty"}); err == nil {
		t.Fatal("expected error when no output exists anywhere")
	}
}

// TestGetSessionOutput_UnprovenAbsenceDoesNotServeFinalCapture pins the rule
// that the durable capture describes an EARLIER completion. When the tmux
// backend is unreachable, the plain existence check collapses the failure into
// "absent" and the session computes as stopped — serving FinalOutput there
// would answer a request for current output with a previous task's result.
func TestGetSessionOutput_UnprovenAbsenceDoesNotServeFinalCapture(t *testing.T) {
	m := testManifest("worker-flaky", "", time.Time{})
	m.FinalOutput = "RESULT: from an earlier task"
	m.FinalOutputAt = time.Now()
	storage := &mockStorage{sessions: []*manifest.Manifest{m}}
	tmux := session.NewMockTmux()
	tmux.HasSessionError = errors.New("tmux socket unreachable")

	if _, err := GetSessionOutput(&OpContext{Storage: storage, Tmux: tmux}, &GetSessionOutputRequest{Identifier: "worker-flaky"}); err == nil {
		t.Fatal("served a stale final capture without proving the pane was gone")
	}
}

// strictListTmux implements the strict observation capability so the list path
// can distinguish "observed, nothing running" from "could not observe".
type strictListTmux struct {
	*session.MockTmux
	err error
}

func (s *strictListTmux) ListSessionsWithInfoStrict() ([]session.SessionInfo, error) {
	return nil, s.err
}

// TestListSessions_StrictObservationFailureReportsUnknown pins the production
// case the plain lister cannot express: tmux maps a missing or permission-denied
// socket to an empty list with a nil error, which would look like a successful
// observation of zero sessions and mark every live manifest stopped.
func TestListSessions_StrictObservationFailureReportsUnknown(t *testing.T) {
	m := testManifest("live-worker", "", time.Time{})
	storage := &mockStorage{sessions: []*manifest.Manifest{m}}
	tmux := &strictListTmux{MockTmux: session.NewMockTmux(), err: errors.New("tmux socket unreachable")}

	result, err := ListSessions(&OpContext{Storage: storage, Tmux: tmux}, &ListSessionsRequest{})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if got := result.Sessions[0].Status; got != "unknown" {
		t.Fatalf("status on an unobservable tmux = %q, want unknown", got)
	}
}

func TestListSessions_NoTmuxReportsUnknownNotStopped(t *testing.T) {
	m := testManifest("live-worker", "", time.Time{})
	storage := &mockStorage{sessions: []*manifest.Manifest{m}}

	result, err := ListSessions(&OpContext{Storage: storage}, &ListSessionsRequest{})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(result.Sessions) != 1 {
		t.Fatalf("got %d sessions", len(result.Sessions))
	}
	if got := result.Sessions[0].Status; got != "unknown" {
		t.Fatalf("status without tmux = %q, want unknown (reporting stopped made live sessions look dead, ce-0zng9)", got)
	}
}

// TestGetSessionOutput_UnobservableBackendIsRetryableWithoutDurableCapture pins
// the classification for a tmux backend that was already unreachable when the
// request arrived. The status probe fails, so the session computes as "unknown"
// and the live capture is skipped; with no durable capture to fall back on, the
// operation used to answer AGM-005/400 — telling a programmatic client its
// well-formed request was permanently invalid — when the honest answer is the
// retryable AGM-017/503. "Could not determine" is not "absent".
func TestGetSessionOutput_UnobservableBackendIsRetryableWithoutDurableCapture(t *testing.T) {
	m := testManifest("worker-unobservable", "", time.Time{})
	storage := &mockStorage{sessions: []*manifest.Manifest{m}}
	tmux := session.NewMockTmux()
	socketDown := errors.New("error connecting to /tmp/agm.sock (Permission denied)")
	tmux.HasSessionError = socketDown

	_, err := GetSessionOutput(&OpContext{Storage: storage, Tmux: tmux}, &GetSessionOutputRequest{Identifier: "worker-unobservable"})
	if err == nil {
		t.Fatal("expected an error when the tmux backend cannot be observed")
	}
	var opErr *OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("error is not an OpError: %v", err)
	}
	if opErr.Code != ErrCodeOutputUnavailable || opErr.Status != 503 {
		t.Fatalf("unobservable backend classified as %s/%d, want %s/503 (retryable)", opErr.Code, opErr.Status, ErrCodeOutputUnavailable)
	}
	// OPS-92: the typed backend failure stays reachable through the envelope.
	if !errors.Is(err, socketDown) {
		t.Fatalf("AGM-017 did not preserve the probe failure as its cause: %v", err)
	}
}

// TestLastSession_StrictObservationFailureReportsUnknown pins the shared status
// helper, which LastSession, SessionHealth and the batch worker view all read
// through. The plain lister maps a socket or permission failure to an empty
// list with a nil error, which those surfaces then read as a successful
// observation of zero sessions and report every live worker as stopped.
func TestLastSession_StrictObservationFailureReportsUnknown(t *testing.T) {
	m := testManifest("live-worker", "", time.Time{})
	storage := &mockStorage{sessions: []*manifest.Manifest{m}}
	// The mock's plain lister returns an empty list with a nil error — exactly
	// what production tmux does on an ExitError — while the strict lister
	// reports the failure it actually is.
	tmux := &strictListTmux{MockTmux: session.NewMockTmux(), err: errors.New("tmux socket unreachable")}

	result, err := LastSession(&OpContext{Storage: storage, Tmux: tmux}, &LastSessionRequest{})
	if err != nil {
		t.Fatalf("LastSession: %v", err)
	}
	if got := result.Session.Status; got != "unknown" {
		t.Fatalf("status on an unobservable tmux = %q, want unknown", got)
	}
}
