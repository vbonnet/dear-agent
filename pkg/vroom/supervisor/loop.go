package supervisor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
)

// ErrAllPeersDark is returned by Loop.Run when the zombie detection
// threshold is reached: all peers have been stale for too many consecutive
// iterations. The loop self-terminates to avoid burning tokens uselessly.
var ErrAllPeersDark = errors.New("supervisor: all peers dark, self-terminating")

// Loop drives one supervisor through the constant iteration cycle described
// in CONTEXT.md §"The three supervisors" and ADR-002 §"Two load-bearing
// invariants":
//
//  1. Mutual-unblock-first. Run the CheckSkill against each of the two peer
//     supervisors. Outcomes are recorded to the decision trail.
//  2. Own job. Call the supervisor's Tick.
//  3. Heartbeat + record. Stamp LastHeartbeat, increment counter, record
//     tick outcome.
//  4. Sleep one Interval and repeat.
//
// The Loop owns the runtime state that CheckSkill needs to read from peers
// (LastHeartbeat, LastTickError), so Loop implements LoopStatus. The Mesh
// (mesh.go) glues Loops together so each Loop can resolve its peers' status.
type Loop struct {
	sup      Supervisor
	mesh     PeerLookup
	check    CheckSkill
	recovery PeerRecovery
	trail    decisiontrail.Trail
	interval time.Duration
	clock    func() time.Time

	// Zombie detection: when all peers are stale for this many consecutive
	// iterations, the loop self-terminates with ErrAllPeersDark. Zero
	// disables zombie detection.
	zombieThreshold int
	allPeersDarkRun int // consecutive iterations with all peers stale

	// status fields — protected by mu.
	mu            sync.RWMutex
	lastHeartbeat time.Time
	lastTickStart time.Time
	lastTickError error

	tickCount  atomic.Uint64
	checkCount atomic.Uint64
}

// PeerLookup is the minimal interface a Loop needs to find its peers. The
// concrete implementation is Mesh; defining it as an interface keeps the
// Loop testable without forcing tests to build a full Mesh.
type PeerLookup interface {
	Get(r Role) (LoopStatus, bool)
}

// LoopConfig holds the dependencies a Loop needs. All fields are required
// except Clock, Recovery, and ZombieThreshold (defaults to time.Now, nil,
// and 10 respectively).
type LoopConfig struct {
	Supervisor Supervisor
	Mesh       PeerLookup
	Check      CheckSkill
	Recovery   PeerRecovery // nil = detect-only, no corrective action
	Trail      decisiontrail.Trail
	Interval   time.Duration

	// ZombieThreshold is the number of consecutive iterations where ALL
	// peers are stale before the loop self-terminates with ErrAllPeersDark.
	// Zero disables zombie detection. Default: 10.
	ZombieThreshold int

	// Clock is the source of time. Defaults to time.Now. Tests inject a
	// fake clock here.
	Clock func() time.Time
}

// NewLoop constructs a Loop. It validates that all required dependencies
// are non-nil and that Interval > 0.
func NewLoop(cfg LoopConfig) (*Loop, error) {
	if cfg.Supervisor == nil {
		return nil, errors.New("supervisor: LoopConfig.Supervisor is nil")
	}
	if !cfg.Supervisor.Role().Valid() {
		return nil, errors.New("supervisor: LoopConfig.Supervisor has invalid Role")
	}
	if cfg.Mesh == nil {
		return nil, errors.New("supervisor: LoopConfig.Mesh is nil")
	}
	if cfg.Check == nil {
		return nil, errors.New("supervisor: LoopConfig.Check is nil")
	}
	if cfg.Trail == nil {
		return nil, errors.New("supervisor: LoopConfig.Trail is nil")
	}
	if cfg.Interval <= 0 {
		return nil, errors.New("supervisor: LoopConfig.Interval must be positive")
	}
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	zombieThreshold := cfg.ZombieThreshold
	if zombieThreshold == 0 {
		zombieThreshold = 10
	}
	return &Loop{
		sup:             cfg.Supervisor,
		mesh:            cfg.Mesh,
		check:           cfg.Check,
		recovery:        cfg.Recovery,
		trail:           cfg.Trail,
		interval:        cfg.Interval,
		clock:           clock,
		zombieThreshold: zombieThreshold,
	}, nil
}

// Role implements LoopStatus.
func (l *Loop) Role() Role { return l.sup.Role() }

// LastHeartbeat implements LoopStatus.
func (l *Loop) LastHeartbeat() time.Time {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.lastHeartbeat
}

// LastTickError implements LoopStatus.
func (l *Loop) LastTickError() error {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.lastTickError
}

// TickCount reports the number of Tick calls completed (success or
// failure). Useful for tests and metrics.
func (l *Loop) TickCount() uint64 { return l.tickCount.Load() }

// CheckCount reports the total number of CheckSkill.Check calls made
// across all iterations (typically 2 × TickCount for the three-supervisor
// mesh).
func (l *Loop) CheckCount() uint64 { return l.checkCount.Load() }

// Run executes the loop until ctx is cancelled or zombie detection triggers.
// The returned error is the context error (context.Canceled / DeadlineExceeded)
// on graceful shutdown, or ErrAllPeersDark if all peers have been stale for
// too many consecutive iterations.
//
// Tick errors and Check errors are recorded to the trail but do NOT cause
// Run to return — the loop is designed to keep running through transient
// failures.
//
// Run publishes a startup heartbeat *before* the first iteration so peer
// HeartbeatCheckSkill calls don't false-positive on a freshly-started
// supervisor.
func (l *Loop) Run(ctx context.Context) error {
	l.recordStartup(ctx)

	for {
		zombie := l.iterate(ctx)
		if zombie {
			_ = l.trail.Append(ctx, decisiontrail.Record{
				Role: string(l.sup.Role()),
				Kind: "supervisor.loop.zombie_exit",
				Payload: map[string]any{
					"consecutive_all_dark": l.allPeersDarkRun,
					"threshold":            l.zombieThreshold,
				},
			})
			return fmt.Errorf("%w: %d consecutive iterations with all peers stale (threshold %d)",
				ErrAllPeersDark, l.allPeersDarkRun, l.zombieThreshold)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(l.interval):
		}
	}
}

// iterate runs one cycle: peer checks, own tick, record heartbeat.
// Returns true if zombie self-termination should trigger.
// Exposed (lowercase) for tests that want to drive iterations manually
// without time-based sleeping.
func (l *Loop) iterate(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}
	l.checkPeers(ctx)
	l.tick(ctx)
	return l.zombieThreshold > 0 && l.allPeersDarkRun >= l.zombieThreshold
}

// checkPeers implements the mutual-unblock-first invariant.
func (l *Loop) checkPeers(ctx context.Context) {
	peers, err := l.sup.Role().PeerList()
	if err != nil {
		_ = l.trail.Append(ctx, decisiontrail.Record{
			Role: string(l.sup.Role()),
			Kind: "supervisor.check.config_error",
			Payload: map[string]any{
				"error": err.Error(),
			},
		})
		return
	}

	allStale := true
	peersChecked := 0

	for _, peerRole := range peers {
		peer, ok := l.mesh.Get(peerRole)
		if !ok {
			_ = l.trail.Append(ctx, decisiontrail.Record{
				Role: string(l.sup.Role()),
				Kind: "supervisor.check.peer_missing",
				Payload: map[string]any{
					"peer": string(peerRole),
				},
			})
			continue
		}
		peersChecked++
		l.checkCount.Add(1)
		checkErr := l.check.Check(ctx, peer)
		rec := decisiontrail.Record{
			Role: string(l.sup.Role()),
			Kind: "supervisor.check.peer",
			Payload: map[string]any{
				"peer": string(peerRole),
				"ok":   checkErr == nil,
			},
		}
		if checkErr != nil {
			rec.Payload["error"] = checkErr.Error()
		}
		_ = l.trail.Append(ctx, rec)

		if checkErr == nil {
			allStale = false
		}

		if checkErr != nil && l.recovery != nil {
			recoverErr := l.recovery.Recover(ctx, peerRole, checkErr.Error())
			recoveryRec := decisiontrail.Record{
				Role: string(l.sup.Role()),
				Kind: "supervisor.check.recovery_attempt",
				Payload: map[string]any{
					"peer": string(peerRole),
					"ok":   recoverErr == nil,
				},
			}
			if recoverErr != nil {
				recoveryRec.Payload["error"] = recoverErr.Error()
			}
			_ = l.trail.Append(ctx, recoveryRec)
		}
	}

	if peersChecked > 0 && allStale {
		l.allPeersDarkRun++
	} else {
		l.allPeersDarkRun = 0
	}
}

// tick runs the supervisor's own job and stamps a fresh heartbeat after.
func (l *Loop) tick(ctx context.Context) {
	start := l.clock()
	l.mu.Lock()
	l.lastTickStart = start
	l.mu.Unlock()

	_ = l.trail.Append(ctx, decisiontrail.Record{
		Role:      string(l.sup.Role()),
		Kind:      "supervisor.tick.start",
		Timestamp: start,
	})

	err := l.sup.Tick(ctx)

	end := l.clock()
	l.mu.Lock()
	l.lastTickError = err
	l.lastHeartbeat = end
	l.mu.Unlock()
	l.tickCount.Add(1)

	rec := decisiontrail.Record{
		Role:      string(l.sup.Role()),
		Kind:      "supervisor.tick.end",
		Timestamp: end,
		Payload: map[string]any{
			"ok":          err == nil,
			"duration_ms": end.Sub(start).Milliseconds(),
		},
	}
	if err != nil {
		rec.Payload["error"] = err.Error()
	}
	_ = l.trail.Append(ctx, rec)
}

// recordStartup writes the initial heartbeat so peers don't see zero time.
func (l *Loop) recordStartup(ctx context.Context) {
	now := l.clock()
	l.mu.Lock()
	l.lastHeartbeat = now
	l.mu.Unlock()
	_ = l.trail.Append(ctx, decisiontrail.Record{
		Role:      string(l.sup.Role()),
		Kind:      "supervisor.loop.start",
		Timestamp: now,
	})
}
