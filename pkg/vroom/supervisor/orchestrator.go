package supervisor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
)

// Queue is the seam the Orchestrator drives. Per CONTEXT.md §"The three
// supervisors":
//
//	Orchestrator | Owns: enqueue/dequeue work, monitor active workers, keep
//	                     steady progress, never sit idle.
//
// PR 1 ships an in-memory implementation (InMemoryQueue). Follow-ups wire
// this to AGM sessions (each dispatched Task becomes an `agm session new`).
type Queue interface {
	// Pending returns tasks that are ready to dispatch (admitted to the
	// roadmap by the Meta-Orchestrator, no Worker assigned yet).
	Pending(ctx context.Context) ([]Task, error)

	// Dispatch records that taskID has been assigned to a Worker. The
	// Worker spawn itself is out of scope for PR 1; Dispatch only mutates
	// the queue's view of the task.
	Dispatch(ctx context.Context, taskID, worker string) error
}

// Task is one unit of work the Orchestrator may dispatch. Shape is
// deliberately small for PR 1; the real Task schema (with dependencies,
// priority, deadline, etc.) lands in follow-ups.
type Task struct {
	// ID uniquely identifies the task within a Queue.
	ID string

	// Title is a short human-readable summary.
	Title string

	// Worker is the name/role of the Worker to dispatch to. Free-form for
	// PR 1 (e.g. "coder", "code-reviewer", "researcher").
	Worker string

	// EnqueuedAt records when the task entered the queue. The InMemoryQueue
	// sets this on Enqueue if it is zero. Tests may set it directly to
	// control apparent age without a clock stub on the queue.
	EnqueuedAt time.Time
}

// idleEscalationThreshold is the number of consecutive idle ticks after which
// the Orchestrator emits a supervisor.orch.idle_escalation record. Seven
// consecutive no-dispatch ticks is the limit per the DEAR retro (ce-6as.7):
// an Orchestrator that never dispatches is either mis-configured or deadlocked
// and must surface the problem rather than silently spinning.
const idleEscalationThreshold = 7

// defaultStaleThreshold is the default age after which an undispatched task
// triggers supervisor.orch.stale_context_escalation. Per bead ce-6as.3:
// a task pending for >24h is a stall — the Orchestrator must take action.
const defaultStaleThreshold = 24 * time.Hour

// Orchestrator is the COO-analogue supervisor. Its Tick scans the work
// queue and dispatches pending tasks to Workers — one per task, no
// prioritisation beyond Queue ordering. Real prioritisation, capacity
// limiting, and worker-health monitoring land in follow-up PRs.
//
// The Orchestrator tracks consecutive idle ticks (Tick calls where no tasks
// were dispatched). When the streak reaches idleEscalationThreshold it emits
// a supervisor.orch.idle_escalation trail record so an operator or the
// Meta-Orchestrator can investigate. Each successful dispatch resets the
// streak to zero.
//
// Stale-context escalation (ce-6as.3): any task that has been pending for
// longer than staleThreshold without being dispatched triggers a
// supervisor.orch.stale_context_escalation trail record so the
// Meta-Orchestrator or operator can ask one specific question, document an
// assumption, or dispatch the context-free sub-tasks instead of letting
// the item block silently.
type Orchestrator struct {
	trail           decisiontrail.Trail
	queue           Queue
	consecutiveIdle int
	staleThreshold  time.Duration    // 0 → defaultStaleThreshold
	now             func() time.Time // nil → time.Now
}

// NewOrchestrator constructs the Orchestrator supervisor.
func NewOrchestrator(trail decisiontrail.Trail, queue Queue) (*Orchestrator, error) {
	if trail == nil {
		return nil, errors.New("supervisor: Orchestrator requires a Trail")
	}
	if queue == nil {
		return nil, errors.New("supervisor: Orchestrator requires a Queue")
	}
	return &Orchestrator{trail: trail, queue: queue}, nil
}

// Role implements Supervisor.
func (o *Orchestrator) Role() Role { return RoleOrchestrator }

// clock returns the Orchestrator's time source (defaults to time.Now).
func (o *Orchestrator) clock() time.Time {
	if o.now != nil {
		return o.now()
	}
	return time.Now()
}

// threshold returns the configured stale threshold (defaults to 24h).
func (o *Orchestrator) threshold() time.Duration {
	if o.staleThreshold > 0 {
		return o.staleThreshold
	}
	return defaultStaleThreshold
}

// Tick dispatches every pending task, then scans the non-dispatched tasks for
// staleness. When no tasks are pending it records a supervisor.orch.no_work
// entry and, if the no-work streak reaches idleEscalationThreshold,
// additionally emits supervisor.orch.idle_escalation.
func (o *Orchestrator) Tick(ctx context.Context) error {
	tasks, err := o.queue.Pending(ctx)
	if err != nil {
		return fmt.Errorf("orchestrator: list pending: %w", err)
	}

	dispatched := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := o.queue.Dispatch(ctx, t.ID, t.Worker); err != nil {
			_ = o.trail.Append(ctx, decisiontrail.Record{
				Role: string(RoleOrchestrator),
				Kind: "supervisor.orch.dispatch_failed",
				Payload: map[string]any{
					"task_id": t.ID,
					"worker":  t.Worker,
					"error":   err.Error(),
				},
			})
			continue
		}
		_ = o.trail.Append(ctx, decisiontrail.Record{
			Role: string(RoleOrchestrator),
			Kind: "supervisor.orch.dispatched",
			Payload: map[string]any{
				"task_id": t.ID,
				"title":   t.Title,
				"worker":  t.Worker,
			},
		})
		dispatched[t.ID] = true
	}

	// Stale-context scan: tasks that could not be dispatched and have been
	// pending longer than the threshold need operator attention (ce-6as.3).
	now := o.clock()
	thresh := o.threshold()
	for _, t := range tasks {
		if dispatched[t.ID] || t.EnqueuedAt.IsZero() {
			continue
		}
		if now.Sub(t.EnqueuedAt) >= thresh {
			_ = o.trail.Append(ctx, decisiontrail.Record{
				Role: string(RoleOrchestrator),
				Kind: "supervisor.orch.stale_context_escalation",
				Payload: map[string]any{
					"task_id":     t.ID,
					"title":       t.Title,
					"enqueued_at": t.EnqueuedAt.Format(time.RFC3339),
					"pending_for": now.Sub(t.EnqueuedAt).String(),
					"threshold":   thresh.String(),
					"action":      "ask one specific question, make documented assumption, or decompose",
				},
			})
		}
	}

	if len(dispatched) > 0 {
		o.consecutiveIdle = 0
		return nil
	}

	// No tasks dispatched this tick.
	o.consecutiveIdle++
	_ = o.trail.Append(ctx, decisiontrail.Record{
		Role: string(RoleOrchestrator),
		Kind: "supervisor.orch.no_work",
		Payload: map[string]any{
			"consecutive_idle": o.consecutiveIdle,
		},
	})

	if o.consecutiveIdle >= idleEscalationThreshold {
		_ = o.trail.Append(ctx, decisiontrail.Record{
			Role: string(RoleOrchestrator),
			Kind: "supervisor.orch.idle_escalation",
			Payload: map[string]any{
				"consecutive_idle": o.consecutiveIdle,
				"threshold":        idleEscalationThreshold,
				"action":           "investigate: queue empty or roadmap not admitting work",
			},
		})
	}
	return nil
}
