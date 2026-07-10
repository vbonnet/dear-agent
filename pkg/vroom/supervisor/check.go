package supervisor

import (
	"context"
	"fmt"
	"time"
)

// CheckSkill is the "supervisor-check skill" referenced in CONTEXT.md §"The
// three supervisors" and ADR-002 §"Two load-bearing invariants" — the first
// action of every loop iteration, run against the two peer supervisors. A
// non-nil error means "peer appears blocked"; the Loop records that to the
// decision trail and (in future iterations) takes corrective action.
//
// CheckSkill is an interface rather than a fixed implementation because:
//
//   - In PR 1 the supervisors run in-process and "blocked" reduces to "no
//     fresh heartbeat" (HeartbeatCheckSkill). That is enough to validate
//     the loop-and-mesh wiring.
//   - In follow-up PRs the supervisors run as separate AGM sessions and
//     "blocked" includes "child harness process awaiting a permission
//     prompt" (the historically dominant blockage per CONTEXT.md). That
//     CheckSkill will read AGM session state and may send AGM messages to
//     unblock — but the Loop's contract with CheckSkill stays the same.
type CheckSkill interface {
	// Check inspects peer. A nil return means "peer looks healthy"; a
	// non-nil error means "peer appears blocked, and here is why". The
	// Loop is responsible for recording either outcome and for any
	// follow-up action — Check itself is a pure observer.
	Check(ctx context.Context, peer LoopStatus) error
}

// CheckSkillFunc adapts a plain function to CheckSkill.
type CheckSkillFunc func(ctx context.Context, peer LoopStatus) error

// Check implements CheckSkill.
func (f CheckSkillFunc) Check(ctx context.Context, peer LoopStatus) error {
	return f(ctx, peer)
}

// HeartbeatCheckSkill is the PR-1 default: a peer is considered blocked if
// its LastHeartbeat is older than Threshold. A zero LastHeartbeat (no ticks
// completed yet) is treated as "blocked" if the Loop has been running long
// enough for at least one heartbeat — but HeartbeatCheckSkill itself has no
// notion of "long enough", so it reports the zero case as blocked
// unconditionally. The Loop deliberately publishes a startup heartbeat
// before the first Check call (see Loop.Run) so the zero case only fires
// when a supervisor never started at all.
type HeartbeatCheckSkill struct {
	// Threshold is the maximum age of a heartbeat before the peer is
	// considered blocked. A reasonable default is 3x the Loop interval,
	// so transient lag in one tick does not trigger a false positive.
	Threshold time.Duration

	// Now returns the current time. Defaults to time.Now if nil; tests
	// inject a fake clock here.
	Now func() time.Time
}

// PeerRecovery is the corrective-action half of the mutual-unblock invariant.
// When CheckSkill reports a peer as blocked, the Loop calls PeerRecovery to
// attempt remediation (e.g. sending an AGM wake message). The interface is
// optional — a nil PeerRecovery means "detect but don't act", which was the
// previous behavior and is still the default for in-process test meshes.
type PeerRecovery interface {
	Recover(ctx context.Context, peerRole Role, reason string) error
}

// PeerRecoveryFunc adapts a plain function to PeerRecovery.
type PeerRecoveryFunc func(ctx context.Context, peerRole Role, reason string) error

// Recover implements PeerRecovery.
func (f PeerRecoveryFunc) Recover(ctx context.Context, peerRole Role, reason string) error {
	return f(ctx, peerRole, reason)
}

// ProcessLivenessCheckSkill layers a harness-process probe over an inner
// CheckSkill (typically HeartbeatCheckSkill). A fresh heartbeat alone must
// not prove liveness (ce-axsr/ce-qkf7): an orphaned writer can keep a peer's
// heartbeat fresh for hours after its harness process died. This skill
// reports such a peer as blocked — DEAD with a zombie-heartbeat reason —
// even though the heartbeat check would pass.
//
// Probe semantics: (alive=false, err=nil) means "proven dead"; a non-nil err
// means "could not verify", which fails open to the inner check only — an
// unverifiable probe must not flip a healthy peer red.
type ProcessLivenessCheckSkill struct {
	// Inner is the heartbeat-freshness (or other) check that must ALSO pass.
	// Nil means process liveness is the only check.
	Inner CheckSkill

	// Probe resolves whether the peer's harness process is actually running
	// (e.g. a tmux pane process-tree scan). detail is appended to the error
	// for a proven-dead peer so callers can say why.
	Probe func(ctx context.Context, peer LoopStatus) (alive bool, detail string, err error)
}

// Check implements CheckSkill: the peer is blocked if the process probe
// proves its harness dead, or if the inner check reports it blocked.
func (s *ProcessLivenessCheckSkill) Check(ctx context.Context, peer LoopStatus) error {
	if s.Probe != nil {
		alive, detail, err := s.Probe(ctx, peer)
		if err == nil && !alive {
			return fmt.Errorf("peer %q is DEAD: no harness process is running (%s) — a fresh heartbeat alone does not prove liveness (zombie heartbeat writer, ce-qkf7)", peer.Role(), detail)
		}
	}
	if s.Inner != nil {
		return s.Inner.Check(ctx, peer)
	}
	return nil
}

// Check reports an error if peer's heartbeat is too stale.
func (h *HeartbeatCheckSkill) Check(_ context.Context, peer LoopStatus) error {
	if h.Threshold <= 0 {
		return fmt.Errorf("supervisor: HeartbeatCheckSkill.Threshold must be positive")
	}
	now := time.Now
	if h.Now != nil {
		now = h.Now
	}
	hb := peer.LastHeartbeat()
	if hb.IsZero() {
		return fmt.Errorf("peer %q has not heartbeated yet", peer.Role())
	}
	age := now().Sub(hb)
	if age > h.Threshold {
		return fmt.Errorf("peer %q heartbeat is %s old (threshold %s)", peer.Role(), age, h.Threshold)
	}
	return nil
}
