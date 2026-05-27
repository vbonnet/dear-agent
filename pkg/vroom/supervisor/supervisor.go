package supervisor

import (
	"context"
	"time"
)

// Supervisor is one of the three long-lived agents in the VROOM mesh. Each
// supervisor has a Role (which is constant for the supervisor's lifetime) and
// a Tick — the "own job" half of every loop iteration. The other half
// (mutual-unblock-first against the two peers) is the responsibility of the
// Loop driver, not the Supervisor.
//
// Implementations must be safe for concurrent use by a single Loop. They are
// not expected to handle concurrent Ticks (the Loop never calls Tick twice in
// parallel for the same supervisor).
type Supervisor interface {
	// Role reports which supervisor this is. The result must not change
	// over the lifetime of the supervisor.
	Role() Role

	// Tick performs one iteration of the supervisor's "own job" — for
	// Meta-Orchestrator, scanning the roadmap; for Orchestrator,
	// dispatching work; for Overseer, checking resources. It must return
	// promptly relative to the Loop's interval; long-running work should
	// be either chunked across ticks or delegated to a Worker.
	//
	// Returning an error does NOT abort the Loop — the Loop records the
	// error to the decision trail and proceeds to the next iteration.
	// Aborting the supervisor cleanly is done by cancelling ctx.
	Tick(ctx context.Context) error
}

// LoopStatus is the read-only view that a CheckSkill sees of a peer's running
// state. It is a separate interface (not Supervisor) so tests can stub a
// peer's status without constructing a real Loop, and so the CheckSkill
// contract makes clear it depends only on observable runtime state — not on
// the peer Supervisor's own implementation details.
type LoopStatus interface {
	// Role identifies which supervisor this status belongs to.
	Role() Role

	// LastHeartbeat is the wall-clock time at which the peer's Loop most
	// recently completed an iteration. Zero time means "no iterations
	// completed yet" — Loops are expected to publish a startup heartbeat
	// so this is rarely zero in practice.
	LastHeartbeat() time.Time

	// LastTickError is the result of the peer's most recent Tick. nil
	// means the last Tick succeeded (or no Tick has run yet). The
	// CheckSkill may use this to decide whether to consider a peer
	// degraded even if its heartbeat is fresh.
	LastTickError() error
}
