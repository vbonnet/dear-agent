package main

import (
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// tmuxWithLiveness wraps session.MockTmux with the optional
// session.HarnessLivenessChecker capability, mirroring internal/ops's
// mockTmuxWithLiveness. session.MockTmux deliberately does not implement it,
// so other cmd/agm tests keep exercising the capability-absent fallback.
type tmuxWithLiveness struct {
	*session.MockTmux
	liveness map[string]session.LivenessInfo
}

func (m *tmuxWithLiveness) HarnessLiveness(name string) (session.LivenessInfo, error) {
	return m.liveness[name], nil
}

func newEscalationTestAdapter(t *testing.T, sessions ...*manifest.Manifest) *dolt.Adapter {
	t.Helper()
	adapter, err := dolt.NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("open session storage: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	for _, m := range sessions {
		if err := adapter.CreateSession(m); err != nil {
			t.Fatalf("create session %s: %v", m.Name, err)
		}
	}
	return adapter
}

// The regression this exists for: this PR made completion routing treat
// DONE as reachable because the completion watcher only stamps DONE after
// confirming the composer is idle and input-ready. discoverEscalationEntry
// shares ops.SessionIsLive with that routing path (by design, see its doc
// comment), so it inherited the same change even though a VROOM node's DONE
// can just as easily be the state detector's unconfirmed "safe default" —
// stamped when the harness process has actually exited but the pane
// lingers. An escalation entered there records state on a node that can
// never deliver it.
func TestDiscoverEscalationEntryWithTmuxSkipsAZombieDoneNode(t *testing.T) {
	zombie := &manifest.Manifest{SessionID: "vroom-orch-id", Name: "vroom-orchestrator", Harness: "claude-code", State: manifest.StateDone}
	fallback := &manifest.Manifest{SessionID: "vroom-overseer-id", Name: "vroom-overseer", Harness: "claude-code", State: manifest.StateReady}
	adapter := newEscalationTestAdapter(t, zombie, fallback)

	tm := &tmuxWithLiveness{
		MockTmux: session.NewMockTmux(),
		liveness: map[string]session.LivenessInfo{
			"vroom-orchestrator": {SessionExists: true, HarnessAlive: false},
		},
	}

	entry := discoverEscalationEntryWithTmux(adapter, tm)
	if entry == nil {
		t.Fatal("discoverEscalationEntryWithTmux() = nil, want it to fall through to the live vroom-overseer")
	}
	if entry.Role != "vroom-overseer" {
		t.Fatalf("Role = %q, want the zombie vroom-orchestrator skipped in favor of vroom-overseer", entry.Role)
	}
}

// A DONE VROOM node whose harness is confirmed still running must still be
// picked — this PR's underlying fix must not regress here.
func TestDiscoverEscalationEntryWithTmuxAcceptsAConfirmedLiveDoneNode(t *testing.T) {
	live := &manifest.Manifest{SessionID: "vroom-orch-id", Name: "vroom-orchestrator", Harness: "claude-code", State: manifest.StateDone}
	adapter := newEscalationTestAdapter(t, live)

	tm := &tmuxWithLiveness{
		MockTmux: session.NewMockTmux(),
		liveness: map[string]session.LivenessInfo{
			"vroom-orchestrator": {SessionExists: true, HarnessAlive: true},
		},
	}

	entry := discoverEscalationEntryWithTmux(adapter, tm)
	if entry == nil || entry.Role != "vroom-orchestrator" {
		t.Fatalf("discoverEscalationEntryWithTmux() = %+v, want the confirmed-live vroom-orchestrator", entry)
	}
}

// Without a liveness checker (nil tmux, or a TmuxInterface not implementing
// the capability), verification is unavailable and the prior, DONE-trusting
// behavior must be preserved rather than the entry point going silently
// undiscoverable.
func TestDiscoverEscalationEntryWithTmuxTrustsStateWithoutALivenessChecker(t *testing.T) {
	live := &manifest.Manifest{SessionID: "vroom-orch-id", Name: "vroom-orchestrator", Harness: "claude-code", State: manifest.StateDone}
	adapter := newEscalationTestAdapter(t, live)

	entry := discoverEscalationEntryWithTmux(adapter, nil)
	if entry == nil || entry.Role != "vroom-orchestrator" {
		t.Fatalf("discoverEscalationEntryWithTmux() = %+v, want DONE trusted when no liveness checker is available", entry)
	}
}
