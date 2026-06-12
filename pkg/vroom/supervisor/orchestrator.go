package supervisor

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

// PermissionChecker is the seam the Orchestrator uses to pre-validate tool
// access before dispatching a Task. If any required permission is denied the
// Orchestrator emits supervisor.orch.permission_blocked and skips that Task —
// it does NOT stall the whole queue on a single blocked item (ce-6as.4).
//
// Implementations may check file-system access, MCP token presence, or any
// other precondition. A nil PermissionChecker allows all tasks.
type PermissionChecker interface {
	// Check returns an error describing which permission is missing, or nil
	// if all required permissions are satisfied.
	Check(ctx context.Context, requiredPerms []string) error
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

	// RequiredPerms lists permission tokens the Worker needs (e.g.
	// "fs:write", "mcp:github", "token:ANTHROPIC_API_KEY"). If the
	// Orchestrator has a PermissionChecker and any token is denied, the
	// task is skipped and a supervisor.orch.permission_blocked trail record
	// is emitted. Empty means no pre-validation.
	RequiredPerms []string
}

// Orchestrator is the COO-analogue supervisor. Its Tick scans the work
// queue and dispatches pending tasks to Workers — one per task, no
// prioritisation beyond Queue ordering. Real prioritisation, capacity
// limiting, and worker-health monitoring land in follow-up PRs.
//
// Permission decoupling (ce-6as.4): before dispatching each task the
// Orchestrator consults its PermissionChecker. Blocked tasks are logged and
// skipped so the remaining queue can proceed — the Orchestrator never stalls
// all autonomous work because one task lacks a permission.
type Orchestrator struct {
	trail decisiontrail.Trail
	queue Queue
	perm  PermissionChecker // nil → allow all
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

// WithPermissionChecker attaches a PermissionChecker so the Orchestrator
// pre-validates tool access before dispatching each Task. Calling this
// method is optional; without it all tasks dispatch unconditionally.
func (o *Orchestrator) WithPermissionChecker(perm PermissionChecker) *Orchestrator {
	o.perm = perm
	return o
}

// Role implements Supervisor.
func (o *Orchestrator) Role() Role { return RoleOrchestrator }

// Tick dispatches every pending task. Tasks whose RequiredPerms cannot be
// satisfied are logged as supervisor.orch.permission_blocked and skipped;
// the rest of the queue is dispatched normally (ce-6as.4).
func (o *Orchestrator) Tick(ctx context.Context) error {
	tasks, err := o.queue.Pending(ctx)
	if err != nil {
		return fmt.Errorf("orchestrator: list pending: %w", err)
	}
	for _, t := range tasks {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Pre-dispatch permission check.
		if o.perm != nil && len(t.RequiredPerms) > 0 {
			if permErr := o.perm.Check(ctx, t.RequiredPerms); permErr != nil {
				_ = o.trail.Append(ctx, decisiontrail.Record{
					Role: string(RoleOrchestrator),
					Kind: "supervisor.orch.permission_blocked",
					Payload: map[string]any{
						"task_id":        t.ID,
						"title":          t.Title,
						"worker":         t.Worker,
						"required_perms": strings.Join(t.RequiredPerms, ","),
						"error":          permErr.Error(),
						"action":         "task skipped — grant permission or remove requirement",
					},
				})
				continue
			}
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
	return nil
}
