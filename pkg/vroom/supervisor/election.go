package supervisor

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
)

var (
	errInvalidElectionSelf = errors.New("supervisor: Election.Self is not a valid role")
	errNilClaimStore       = errors.New("supervisor: Election requires a ClaimStore")
	errNilElectionTrail    = errors.New("supervisor: Election requires a Trail")
)

// Claim is one supervisor's bid to assume the responsibilities of a dark peer.
// Claims are written to a ClaimStore during the claim phase of leader election
// (retro R.1) so competing supervisors can detect each other and avoid the
// split-brain scenario the retro warns about ("two supervisors both claim
// Meta-O authority").
type Claim struct {
	// Target is the dark supervisor whose role is being claimed.
	Target Role
	// By is the supervisor making the claim.
	By Role
	// Reason is a short human-readable justification (e.g. "primary stale for
	// 4 check intervals").
	Reason string
	// Timestamp is when the claim was written. Earlier claims win (see
	// claimWins); ties break by canonical role order.
	Timestamp time.Time
}

// ClaimStore persists failover claims. The default in-process implementation
// is InMemoryClaimStore; a file-backed implementation (FileClaimStore) writes
// to ~/.agm/supervisors/<by>/ so claims survive across separate AGM sessions,
// as the retro's claim/verify/assume/announce protocol requires.
type ClaimStore interface {
	// Put writes (or overwrites) by's claim on target.
	Put(ctx context.Context, c Claim) error
	// Get returns the *winning* claim on target across all claimants — the
	// earliest by Timestamp, ties broken by canonical role order — and whether
	// any claim exists.
	Get(ctx context.Context, target Role) (Claim, bool, error)
	// Delete removes by's claim on target. Deleting a claim that does not
	// exist is not an error.
	Delete(ctx context.Context, target Role, by Role) error
}

// claimWins reports whether claim a beats claim b. Earlier timestamp wins;
// equal timestamps break ties by canonical role order (AllRoles index) so the
// resolution is deterministic and every supervisor agrees on the winner.
func claimWins(a, b Claim) bool {
	if a.Timestamp.Before(b.Timestamp) {
		return true
	}
	if b.Timestamp.Before(a.Timestamp) {
		return false
	}
	return roleOrder(a.By) < roleOrder(b.By)
}

// roleOrder returns the index of r in AllRoles, or a large sentinel for
// unknown roles so they sort last.
func roleOrder(r Role) int {
	for i, c := range AllRoles() {
		if c == r {
			return i
		}
	}
	return len(AllRoles())
}

// ElectionPhase is the state of one target's election from a single
// supervisor's perspective.
type ElectionPhase int

const (
	// PhaseIdle: no active claim on the target.
	PhaseIdle ElectionPhase = iota
	// PhaseClaimed: a claim has been written and the verify window is open.
	PhaseClaimed
	// PhaseAssumed: the verify window passed unchallenged and this supervisor
	// now holds the target's responsibilities.
	PhaseAssumed
)

// String renders the phase for the decision trail.
func (p ElectionPhase) String() string {
	switch p {
	case PhaseIdle:
		return "idle"
	case PhaseClaimed:
		return "claimed"
	case PhaseAssumed:
		return "assumed"
	default:
		return "unknown"
	}
}

// ElectionOutcome reports what one Step did. It is a value so callers (the
// Failover coordinator and tests) can react without inspecting Election state.
type ElectionOutcome struct {
	// Phase is the target's phase after the step.
	Phase ElectionPhase
	// Eligible reports whether this supervisor is permitted to contest the
	// target at all (Secondary always; Tertiary only if the Secondary is also
	// dark). An ineligible step is a no-op deferral.
	Eligible bool
	// Claimed is true on the step that wrote a fresh claim.
	Claimed bool
	// Assumed is true on the step that transitioned to PhaseAssumed.
	Assumed bool
	// Yielded is true on the step that abandoned a claim because a competing
	// supervisor won the verify race.
	Yielded bool
	// Relinquished is true on the step that dropped authority because the
	// target recovered.
	Relinquished bool
}

// Election runs the claim → verify → assume → announce protocol (retro R.1)
// for one supervisor against its dark peers. It is a per-target state machine
// driven one step per loop iteration: the verify phase deliberately spans
// multiple iterations so a competing claim has time to surface before any
// supervisor assumes authority (retro: "Wait 2 check intervals, verify no
// competing claim").
//
// Election is safe for concurrent use by a single Loop.
type Election struct {
	self         Role
	store        ClaimStore
	trail        decisiontrail.Trail
	clock        func() time.Time
	verifyWindow time.Duration

	mu     sync.Mutex
	states map[Role]*electionState
}

type electionState struct {
	phase     ElectionPhase
	claimedAt time.Time
	reason    string
	// deferLogged avoids spamming the trail with a deferral record every
	// iteration while a Tertiary waits on a live Secondary.
	deferLogged bool
}

// ElectionConfig holds Election dependencies. Store and Trail are required;
// Clock defaults to time.Now and VerifyWindow defaults to 2× a nominal check
// interval (callers should pass 2×Loop.Interval).
type ElectionConfig struct {
	Self         Role
	Store        ClaimStore
	Trail        decisiontrail.Trail
	VerifyWindow time.Duration
	Clock        func() time.Time
}

// NewElection constructs an Election. It returns an error if Self is not a
// valid role or a required dependency is nil.
func NewElection(cfg ElectionConfig) (*Election, error) {
	if !cfg.Self.Valid() {
		return nil, errInvalidElectionSelf
	}
	if cfg.Store == nil {
		return nil, errNilClaimStore
	}
	if cfg.Trail == nil {
		return nil, errNilElectionTrail
	}
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	vw := cfg.VerifyWindow
	if vw <= 0 {
		vw = 2 * time.Minute
	}
	return &Election{
		self:         cfg.Self,
		store:        cfg.Store,
		trail:        cfg.Trail,
		clock:        clock,
		verifyWindow: vw,
		states:       map[Role]*electionState{},
	}, nil
}

// Active reports whether an election for target is mid-flight (claimed or
// assumed). The Failover coordinator uses this to keep stepping a target after
// its stale streak resets — so a recovered peer triggers relinquish.
func (e *Election) Active(target Role) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.states[target]
	return st != nil && st.phase != PhaseIdle
}

// Phase reports the current election phase for target.
func (e *Election) Phase(target Role) ElectionPhase {
	e.mu.Lock()
	defer e.mu.Unlock()
	if st := e.states[target]; st != nil {
		return st.phase
	}
	return PhaseIdle
}

// Step advances the election for target by at most one phase. isStale reports
// whether a given role currently appears dark (used to check that target is
// still down and, for a Tertiary contender, that the Secondary is also down).
//
// The phases (retro R.1):
//
//  1. Claim — write a claim and open the verify window.
//  2. Verify — once the verify window elapses, re-read the store; if a
//     competing supervisor's claim wins, yield; otherwise proceed.
//  3. Assume — record assumption and hold the target's responsibilities.
//  4. Announce — emit supervisor.failover.assumed to the trail (the Failover
//     coordinator additionally notifies surviving peers).
//
// If target recovers at any point, Step relinquishes the claim and returns to
// idle.
func (e *Election) Step(ctx context.Context, target Role, reason string, isStale func(Role) bool) ElectionOutcome {
	e.mu.Lock()
	defer e.mu.Unlock()

	st := e.states[target]
	if st == nil {
		st = &electionState{phase: PhaseIdle}
		e.states[target] = st
	}

	// Recovery: the target came back. Drop any claim/authority we hold.
	if !isStale(target) {
		if st.phase != PhaseIdle {
			prev := st.phase
			_ = e.store.Delete(ctx, target, e.self)
			st.phase = PhaseIdle
			st.deferLogged = false
			e.record(ctx, "supervisor.failover.relinquished", map[string]any{
				"target":     string(target),
				"prev_phase": prev.String(),
				"reason":     "target recovered",
			})
			return ElectionOutcome{Phase: PhaseIdle, Eligible: true, Relinquished: true}
		}
		return ElectionOutcome{Phase: PhaseIdle, Eligible: true}
	}

	eligible, secondary := e.eligible(target, isStale)
	if !eligible {
		// Tertiary deferring to a live Secondary. Log once.
		if !st.deferLogged {
			e.record(ctx, "supervisor.failover.deferred", map[string]any{
				"target":       string(target),
				"defers_to":    string(secondary),
				"reason":       "Secondary alive; Tertiary defers (retro R.1)",
				"contender_is": string(e.self),
			})
			st.deferLogged = true
		}
		return ElectionOutcome{Phase: st.phase, Eligible: false}
	}
	st.deferLogged = false

	now := e.clock()
	switch st.phase {
	case PhaseIdle:
		// Claim phase.
		st.reason = reason
		st.claimedAt = now
		st.phase = PhaseClaimed
		if err := e.store.Put(ctx, Claim{Target: target, By: e.self, Reason: reason, Timestamp: now}); err != nil {
			e.record(ctx, "supervisor.failover.claim_failed", map[string]any{
				"target": string(target),
				"error":  err.Error(),
			})
			st.phase = PhaseIdle
			return ElectionOutcome{Phase: PhaseIdle, Eligible: true}
		}
		e.record(ctx, "supervisor.failover.claim", map[string]any{
			"target": string(target),
			"by":     string(e.self),
			"reason": reason,
		})
		return ElectionOutcome{Phase: PhaseClaimed, Eligible: true, Claimed: true}

	case PhaseClaimed:
		// Verify phase — hold until the verify window elapses so competing
		// claims have time to surface.
		if now.Sub(st.claimedAt) < e.verifyWindow {
			return ElectionOutcome{Phase: PhaseClaimed, Eligible: true}
		}
		winner, ok, err := e.store.Get(ctx, target)
		if err != nil {
			e.record(ctx, "supervisor.failover.verify_failed", map[string]any{
				"target": string(target),
				"error":  err.Error(),
			})
			return ElectionOutcome{Phase: PhaseClaimed, Eligible: true}
		}
		if ok && winner.By != e.self {
			// Lost the verify race — yield to avoid split-brain.
			_ = e.store.Delete(ctx, target, e.self)
			st.phase = PhaseIdle
			e.record(ctx, "supervisor.failover.yielded", map[string]any{
				"target": string(target),
				"winner": string(winner.By),
				"by":     string(e.self),
				"reason": "competing claim won verify race",
			})
			return ElectionOutcome{Phase: PhaseIdle, Eligible: true, Yielded: true}
		}
		// Assume + announce.
		st.phase = PhaseAssumed
		e.record(ctx, "supervisor.failover.assumed", map[string]any{
			"target":        string(target),
			"by":            string(e.self),
			"reason":        st.reason,
			"verify_window": e.verifyWindow.String(),
		})
		return ElectionOutcome{Phase: PhaseAssumed, Eligible: true, Assumed: true}

	default: // PhaseAssumed — steady state, nothing to do.
		return ElectionOutcome{Phase: PhaseAssumed, Eligible: true}
	}
}

// eligible reports whether self may contest target, and returns target's
// Secondary for trail context. The Secondary may always contest; the Tertiary
// may only contest if the Secondary is also dark (retro R.1).
func (e *Election) eligible(target Role, isStale func(Role) bool) (bool, Role) {
	secondary, err := SecondaryFor(target)
	if err != nil {
		return false, ""
	}
	if e.self == secondary {
		return true, secondary
	}
	tertiary, err := TertiaryFor(target)
	if err != nil {
		return false, secondary
	}
	if e.self == tertiary {
		return isStale(secondary), secondary
	}
	return false, secondary
}

func (e *Election) record(ctx context.Context, kind string, payload map[string]any) {
	_ = e.trail.Append(ctx, decisiontrail.Record{
		Role:    string(e.self),
		Kind:    kind,
		Payload: payload,
	})
}
