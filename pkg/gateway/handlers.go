package gateway

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// RunRequest mirrors api.RunRequest so handlers don't pull pkg/api into
// pkg/gateway (the dependency runs the other way: the HTTP adapter
// imports both).
type RunRequest struct {
	File   string
	Inputs map[string]string
}

// RunResponse is the success payload of a CmdRun handler.
type RunResponse struct {
	RunID    string
	Workflow string
	PID      int
}

// Runner is the spawn surface CmdRun delegates to. The default
// production implementation is api.ExecRunner. Tests pass a stub.
type Runner interface {
	Run(ctx context.Context, req RunRequest, caller Caller) (RunResponse, error)
}

// WorkflowHandlers builds a HandlerSet that wraps pkg/workflow against
// a single runs.db connection. Pass the result to gateway.New.
//
// Runner may be nil; CmdRun then returns CodeUnavailable.
func WorkflowHandlers(db *sql.DB, runner Runner) HandlerSet {
	return HandlerSet{
		Run:     runHandler(runner),
		Status:  statusHandler(db),
		List:    listHandler(db),
		Logs:    logsHandler(db),
		Gates:   gatesHandler(db),
		Approve: decisionHandler(db, workflow.HITLDecisionApprove),
		Reject:  decisionHandler(db, workflow.HITLDecisionReject),
		Cancel:  cancelHandler(db),
	}
}

func runHandler(runner Runner) Handler {
	return func(ctx context.Context, cmd Command) Response {
		if runner == nil {
			return errorResponse(cmd.ID, Errorf(CodeUnavailable, "runner not configured"))
		}
		file, ok := stringArg(cmd.Args, "file")
		if !ok || file == "" {
			return errorResponse(cmd.ID, Errorf(CodeInvalidArgs, "file is required"))
		}
		req := RunRequest{File: file}
		if inputs, ok := cmd.Args["inputs"].(map[string]string); ok {
			req.Inputs = inputs
		} else if anyMap, ok := cmd.Args["inputs"].(map[string]any); ok {
			req.Inputs = make(map[string]string, len(anyMap))
			for k, v := range anyMap {
				s, ok := v.(string)
				if !ok {
					return errorResponse(cmd.ID, Errorf(CodeInvalidArgs,
						"inputs[%q] is not a string", k))
				}
				req.Inputs[k] = s
			}
		}
		resp, err := runner.Run(ctx, req, cmd.Caller)
		if err != nil {
			return errorResponse(cmd.ID, WrapError(CodeInternal, "run", err))
		}
		return Response{CommandID: cmd.ID, Body: map[string]any{
			"run_id":   resp.RunID,
			"workflow": resp.Workflow,
			"pid":      resp.PID,
		}}
	}
}

func statusHandler(db *sql.DB) Handler {
	return func(ctx context.Context, cmd Command) Response {
		runID, ok := stringArg(cmd.Args, "run_id")
		if !ok || runID == "" {
			return errorResponse(cmd.ID, Errorf(CodeInvalidArgs, "run_id is required"))
		}
		st, err := workflow.Status(ctx, db, runID)
		if err != nil {
			if errors.Is(err, workflow.ErrRunNotFound) {
				return errorResponse(cmd.ID, WrapError(CodeNotFound,
					fmt.Sprintf("run %q not found", runID), err))
			}
			return errorResponse(cmd.ID, WrapError(CodeInternal, "status", err))
		}
		return Response{CommandID: cmd.ID, Body: map[string]any{"run": st}}
	}
}

func listHandler(db *sql.DB) Handler {
	return func(ctx context.Context, cmd Command) Response {
		opts := workflow.ListOptions{}
		if state, ok := stringArg(cmd.Args, "state"); ok {
			opts.State = workflow.RunState(state)
		}
		opts.Limit = intArg(cmd.Args, "limit", 50, 500)
		runs, err := workflow.List(ctx, db, opts)
		if err != nil {
			return errorResponse(cmd.ID, WrapError(CodeInternal, "list", err))
		}
		if runs == nil {
			runs = []workflow.RunSummary{}
		}
		return Response{CommandID: cmd.ID, Body: map[string]any{"runs": runs}}
	}
}

func logsHandler(db *sql.DB) Handler {
	return func(ctx context.Context, cmd Command) Response {
		runID, ok := stringArg(cmd.Args, "run_id")
		if !ok || runID == "" {
			return errorResponse(cmd.ID, Errorf(CodeInvalidArgs, "run_id is required"))
		}
		opts := workflow.LogsOptions{Limit: intArg(cmd.Args, "limit", 200, 1000)}
		events, err := workflow.Logs(ctx, db, runID, opts)
		if err != nil {
			return errorResponse(cmd.ID, WrapError(CodeInternal, "logs", err))
		}
		if events == nil {
			events = []workflow.AuditEvent{}
		}
		return Response{CommandID: cmd.ID, Body: map[string]any{"events": events}}
	}
}

func gatesHandler(db *sql.DB) Handler {
	return func(ctx context.Context, cmd Command) Response {
		gates, err := workflow.ListPendingHITLRequests(ctx, db)
		if err != nil {
			return errorResponse(cmd.ID, WrapError(CodeInternal, "gates", err))
		}
		if gates == nil {
			gates = []workflow.HITLRequest{}
		}
		return Response{CommandID: cmd.ID, Body: map[string]any{"gates": gates}}
	}
}

func decisionHandler(db *sql.DB, dec workflow.HITLDecision) Handler {
	return func(ctx context.Context, cmd Command) Response {
		approvalID, ok := stringArg(cmd.Args, "approval_id")
		if !ok || approvalID == "" {
			return errorResponse(cmd.ID, Errorf(CodeInvalidArgs, "approval_id is required"))
		}
		if cmd.Caller.LoginName == "" {
			return errorResponse(cmd.ID, Errorf(CodeUnauthorized,
				"caller identity required for %s", dec))
		}
		role, _ := stringArg(cmd.Args, "role")
		reason, _ := stringArg(cmd.Args, "reason")
		err := workflow.RecordHITLDecision(ctx, db, approvalID, dec,
			cmd.Caller.LoginName, role, reason, time.Now())
		if err != nil {
			switch {
			case errors.Is(err, workflow.ErrApprovalNotFound):
				return errorResponse(cmd.ID, WrapError(CodeNotFound,
					fmt.Sprintf("approval %q not found", approvalID), err))
			case errors.Is(err, workflow.ErrApprovalAlreadyResolved):
				return errorResponse(cmd.ID, WrapError(CodeConflict,
					"approval already resolved", err))
			case errors.Is(err, workflow.ErrApproverRoleMismatch):
				return errorResponse(cmd.ID, WrapError(CodeUnauthorized,
					"approver role mismatch", err))
			default:
				return errorResponse(cmd.ID, WrapError(CodeInternal, "decide", err))
			}
		}
		return Response{CommandID: cmd.ID, Body: map[string]any{
			"approval_id": approvalID,
			"decision":    string(dec),
			"actor":       cmd.Caller.LoginName,
		}}
	}
}

// cancelHandler is a placeholder; pkg/workflow does not yet expose a
// Cancel function. The handler returns CodeUnavailable so callers get
// a clean structured error rather than nil-deref. ADR-017 calls this
// out in §D2 ("Phase 2 placeholder").
func cancelHandler(_ *sql.DB) Handler {
	return func(_ context.Context, cmd Command) Response {
		return errorResponse(cmd.ID, Errorf(CodeUnavailable,
			"cancel not yet implemented (see ADR-017 D2)"))
	}
}

// stringArg reads a string from args[key]. Returns ("", false) if the
// key is absent or the value is not a string.
func stringArg(args map[string]any, key string) (string, bool) {
	v, ok := args[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// intArg reads an int from args[key], clamped to [1, ceiling]. Falls
// back to def when missing, non-numeric, or zero/negative.
//
// JSON decoding produces float64 for numbers; we accept both int and
// float64 to keep adapter wire formats simple.
func intArg(args map[string]any, key string, def, ceiling int) int {
	v, ok := args[key]
	if !ok {
		return def
	}
	var n int
	switch x := v.(type) {
	case int:
		n = x
	case int64:
		n = int(x)
	case float64:
		n = int(x)
	default:
		return def
	}
	if n <= 0 {
		return def
	}
	if n > ceiling {
		return ceiling
	}
	return n
}
