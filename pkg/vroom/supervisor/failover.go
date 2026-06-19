package supervisor

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
)

// defaultPromoteThreshold is the number of consecutive check intervals a peer
// must appear dark before its Secondary starts a leader-election claim (retro
// R.1: "stale for > 3 check intervals").
const defaultPromoteThreshold = 3

// PeerNotifier announces a failover event to surviving peers (the "announce"
// phase of retro R.1). In-process meshes can leave it nil; the AGM-backed
// implementation sends a message to each surviving peer's session.
type PeerNotifier interface {
	// Notify tells peer that this supervisor has assumed target's role.
	Notify(ctx context.Context, peer, target, by Role, reason string) error
}

// PeerNotifierFunc adapts a plain function to PeerNotifier.
type PeerNotifierFunc func(ctx context.Context, peer, target, by Role, reason string) error

// Notify implements PeerNotifier.
func (f PeerNotifierFunc) Notify(ctx context.Context, peer, target, by Role, reason string) error {
	return f(ctx, peer, target, by, reason)
}

// Failover is the per-supervisor coordinator that turns the Loop's
// per-iteration peer-staleness signal into the three failover behaviours the
// retro prescribes:
//
//   - Degraded-mode tracking (retro R.4): derives MeshMode (3/3, 2/3, 1/3) from
//     how many peers are reachable, records every transition with the
//     capability set now in force.
//   - Leader election (retro R.1): once a peer has been dark past
//     PromoteThreshold consecutive intervals, drives the Election state machine
//     to claim → verify → assume → announce that peer's role.
//   - Authority bookkeeping for roadmap delegation (retro R.5): tracks which
//     targets this supervisor currently holds, so a DelegatedRoadmap can gate
//     failover writes on real assumed authority.
//
// Failover implements FailoverObserver; the Loop calls Observe once per
// iteration after its peer checks.
type Failover struct {
	self             Role
	trail            decisiontrail.Trail
	election         *Election
	notifier         PeerNotifier
	clock            func() time.Time
	promoteThreshold int

	mu        sync.Mutex
	staleRuns map[Role]int
	mode      MeshMode
	modeSet   bool
	assumed   map[Role]bool
}

// FailoverConfig holds Failover dependencies. Self, Trail and Election are
// required. Notifier is optional (nil = trail-only announce). PromoteThreshold
// defaults to 3; Clock defaults to time.Now.
type FailoverConfig struct {
	Self             Role
	Trail            decisiontrail.Trail
	Election         *Election
	Notifier         PeerNotifier
	PromoteThreshold int
	Clock            func() time.Time
}

// NewFailover constructs a Failover coordinator.
func NewFailover(cfg FailoverConfig) (*Failover, error) {
	if !cfg.Self.Valid() {
		return nil, errors.New("supervisor: Failover.Self is not a valid role")
	}
	if cfg.Trail == nil {
		return nil, errors.New("supervisor: Failover requires a Trail")
	}
	if cfg.Election == nil {
		return nil, errors.New("supervisor: Failover requires an Election")
	}
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	pt := cfg.PromoteThreshold
	if pt <= 0 {
		pt = defaultPromoteThreshold
	}
	return &Failover{
		self:             cfg.Self,
		trail:            cfg.Trail,
		election:         cfg.Election,
		notifier:         cfg.Notifier,
		clock:            clock,
		promoteThreshold: pt,
		staleRuns:        map[Role]int{},
		assumed:          map[Role]bool{},
	}, nil
}

// Observe is called once per loop iteration with the set of peers that
// appeared dark this iteration (true = stale/dark). It updates the per-peer
// stale streaks, records any mesh-mode transition with the in-force capability
// set, and drives leader election for peers that have been dark past the
// promotion threshold.
func (f *Failover) Observe(ctx context.Context, staleness map[Role]bool) {
	peers, err := f.self.PeerList()
	if err != nil {
		return
	}

	f.mu.Lock()
	alive := 0
	for _, p := range peers {
		if staleness[p] {
			f.staleRuns[p]++
		} else {
			f.staleRuns[p] = 0
			alive++
		}
	}

	// Mesh-mode transition + capability snapshot.
	newMode := ModeForAlivePeers(alive)
	if !f.modeSet || newMode != f.mode {
		prev := f.mode
		hadMode := f.modeSet
		f.mode = newMode
		f.modeSet = true
		caps := f.capabilitiesLocked()
		payload := map[string]any{
			"mode":         newMode.String(),
			"alive_peers":  alive,
			"capabilities": caps.asPayload(),
		}
		if hadMode {
			payload["prev_mode"] = prev.String()
		}
		f.recordLocked(ctx, "supervisor.mode.transition", payload)
	}

	// Snapshot the per-iteration staleness so the election closures read a
	// stable view without holding f.mu (Step records to the trail).
	isStale := func(r Role) bool { return staleness[r] }
	type stepReq struct {
		target Role
		reason string
	}
	var reqs []stepReq
	for _, target := range peers {
		if f.staleRuns[target] >= f.promoteThreshold || f.election.Active(target) {
			reqs = append(reqs, stepReq{
				target: target,
				reason: failoverReason(f.staleRuns[target], f.promoteThreshold),
			})
		}
	}
	f.mu.Unlock()

	for _, r := range reqs {
		out := f.election.Step(ctx, r.target, r.reason, isStale)
		f.applyOutcome(ctx, r.target, r.reason, out)
	}
}

// applyOutcome updates assumed-authority bookkeeping and fires the announce
// notification when a target is freshly assumed.
func (f *Failover) applyOutcome(ctx context.Context, target Role, reason string, out ElectionOutcome) {
	switch {
	case out.Assumed:
		f.mu.Lock()
		f.assumed[target] = true
		f.mu.Unlock()
		f.announce(ctx, target, reason)
	case out.Relinquished || out.Yielded:
		f.mu.Lock()
		delete(f.assumed, target)
		f.mu.Unlock()
	}
}

// announce notifies every surviving peer (all roles except self and the dead
// target) that self has assumed target's role, and records the announcement to
// the trail. This is the "announce" phase of retro R.1; the assumption itself
// was already recorded by Election as supervisor.failover.assumed.
func (f *Failover) announce(ctx context.Context, target Role, reason string) {
	var notified []string
	if f.notifier != nil {
		for _, peer := range AllRoles() {
			if peer == f.self || peer == target {
				continue
			}
			if err := f.notifier.Notify(ctx, peer, target, f.self, reason); err != nil {
				f.recordLocked(ctx, "supervisor.failover.announce_failed", map[string]any{
					"target": string(target),
					"peer":   string(peer),
					"error":  err.Error(),
				})
				continue
			}
			notified = append(notified, string(peer))
		}
	}
	f.recordLocked(ctx, "supervisor.failover.announced", map[string]any{
		"target":   string(target),
		"by":       string(f.self),
		"notified": notified,
		"reason":   reason,
	})
}

// Mode returns the current observed MeshMode.
func (f *Failover) Mode() MeshMode {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mode
}

// Capabilities returns the capability set in force for the current mode and
// assumed authority.
func (f *Failover) Capabilities() Capabilities {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.capabilitiesLocked()
}

// HoldsAuthority reports whether this supervisor currently holds target's
// responsibilities via an assumed failover (PhaseAssumed). Used by
// DelegatedRoadmap to gate failover roadmap writes (retro R.5).
func (f *Failover) HoldsAuthority(target Role) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.assumed[target]
}

// capabilitiesLocked computes the capability set; caller must hold f.mu.
func (f *Failover) capabilitiesLocked() Capabilities {
	roadmapAuthority := f.self == RoleMetaOrchestrator || f.assumed[RoleMetaOrchestrator]
	return CapabilitiesFor(f.mode, roadmapAuthority)
}

func (f *Failover) recordLocked(ctx context.Context, kind string, payload map[string]any) {
	_ = f.trail.Append(ctx, decisiontrail.Record{
		Role:    string(f.self),
		Kind:    kind,
		Payload: payload,
	})
}

// failoverReason renders a short reason string for a claim.
func failoverReason(staleRun, threshold int) string {
	return "peer dark for " + strconv.Itoa(staleRun) + " consecutive intervals (threshold " + strconv.Itoa(threshold) + ")"
}
