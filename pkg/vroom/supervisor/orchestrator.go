package supervisor

import (
	"context"
	"errors"
	"fmt"

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
}

// orchIdleEscalationThreshold is the number of consecutive empty-queue ticks
// after which the Orchestrator emits supervisor.orch.idle_escalation.
// Mirrors metaoIdleEscalationThreshold: a supervisor that silently does
// nothing for 7 ticks is either misconfigured or starved of work — that must
// surface. CONTEXT.md §"The three supervisors": "never sit idle."
const orchIdleEscalationThreshold = 7

// Orchestrator is the COO-analogue supervisor. Its Tick scans the work
// queue and dispatches pending tasks to Workers — one per task, no
// prioritisation beyond Queue ordering. Real prioritisation, capacity
// limiting, and worker-health monitoring land in follow-up PRs.
//
// Idle escalation (ce-6as.80): when the queue is empty for 7 consecutive
// ticks the Orchestrator emits supervisor.orch.idle_escalation so an
// operator can investigate whether work has stalled upstream.
type Orchestrator struct {
	trail           decisiontrail.Trail
	queue           Queue
	consecutiveIdle int
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

// Tick dispatches every pending task. When no tasks are pending it emits
// supervisor.orch.no_work and, after orchIdleEscalationThreshold consecutive
// idle ticks, additionally emits supervisor.orch.idle_escalation (ce-6as.80).
func (o *Orchestrator) Tick(ctx context.Context) error {
	tasks, err := o.queue.Pending(ctx)
	if err != nil {
		return fmt.Errorf("orchestrator: list pending: %w", err)
	}

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
	}

	if len(tasks) > 0 {
		// Pending tasks existed this tick (even if dispatch errored) — not idle.
		o.consecutiveIdle = 0
		return nil
	}

	// Empty queue tick.
	o.consecutiveIdle++
	_ = o.trail.Append(ctx, decisiontrail.Record{
		Role: string(RoleOrchestrator),
		Kind: "supervisor.orch.no_work",
		Payload: map[string]any{
			"consecutive_idle": o.consecutiveIdle,
		},
	})

	if o.consecutiveIdle >= orchIdleEscalationThreshold {
		_ = o.trail.Append(ctx, decisiontrail.Record{
			Role: string(RoleOrchestrator),
			Kind: "supervisor.orch.idle_escalation",
			Payload: map[string]any{
				"consecutive_idle": o.consecutiveIdle,
				"threshold":        orchIdleEscalationThreshold,
				"action":           "investigate: queue empty or Meta-Orchestrator not admitting proposals",
			},
		})
	}
	return nil
}
