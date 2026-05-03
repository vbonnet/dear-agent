package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/vbonnet/dear-agent/pkg/workflow/roles"
)

// AIExecutor is the hook for invoking an LLM. Callers plug in their own —
// typically a thin wrapper around pkg/llm/provider or the agm-bus channel
// for Max-plan OAuth routing. Keeping this as an interface lets the
// workflow engine stay independent of any single LLM backend.
type AIExecutor interface {
	// Generate returns the LLM's response text. The runner has already
	// rendered templates in prompt/system before calling. model may be
	// empty; implementations should fall back to their default.
	Generate(ctx context.Context, node *AINode, inputs map[string]string, outputs map[string]string) (string, error)
}

// Runner executes workflows. Construct with NewRunner then call Run.
// A single Runner instance is safe for concurrent Run calls — each Run
// uses its own nodeContext and output map.
type Runner struct {
	// AI is called for every KindAI node. Required.
	AI AIExecutor
	// Logger is used for run-time logging. Defaults to slog.Default().
	Logger *slog.Logger
	// DefaultWorkingDir is the cwd for bash nodes that don't override it.
	DefaultWorkingDir string
	// DefaultBashShell is the shell used for bash nodes. Defaults to /bin/sh.
	DefaultBashShell string
	// SignalTimeout caps how long Gate nodes wait for a signal. Zero
	// means "wait forever" (or until the parent context is cancelled).
	SignalTimeout time.Duration
	// State, if non-nil, is used to checkpoint progress after each node.
	// Pass it to Resume to skip already-completed nodes on restart.
	State State

	// Recorder, if non-nil, receives per-run, per-node, and per-attempt
	// detail beyond the simple Snapshot. SQLiteState implements both this
	// and AuditSink; wiring all three at once is what UseSQLiteState does.
	Recorder RunRecorder

	// Audit, if non-nil, receives a structured event for every state
	// transition. The default SQLite-backed sink is the engine's
	// canonical audit log; users can fan out to multiple sinks via
	// MultiAuditSink.
	Audit AuditSink

	// Trigger labels how this run was started ("cli", "mcp", "sdk",
	// "cron", "trigger"). Recorded on the runs row. Empty defaults to
	// "sdk" (the most common embedding).
	Trigger string
	// TriggeredBy is a free-form actor label (user name, MCP client id,
	// schedule id) recorded on the runs row.
	TriggeredBy string

	// RoleResolver maps an AI node's role: declaration to a concrete
	// model id. Nil falls back to a Resolver backed by the built-in
	// registry — Phase 1's ship criterion is "switching Opus 4.7 →
	// Opus 5.0 is one line in roles.yaml", so any production caller
	// should plug in a real registry-backed resolver here.
	RoleResolver *roles.Resolver

	// Budget is the run-level budget meter. Wraps every AI call via
	// MeteredAIExecutor. Nil disables run-level budget enforcement;
	// per-node Budget caps still apply if a node declares them and a
	// MeteredAIExecutor is wired manually.
	Budget *Meter

	// Permissions is the bounded-permissions enforcer. Nil falls back
	// to DefaultPermissionEnforcer (permissive when no allowlist is
	// declared, strict when one is).
	Permissions PermissionEnforcer

	// OutputWriter materialises declared outputs and refuses to mark
	// the node succeeded if any are missing. Nil disables Phase 1.6 —
	// useful for tests that don't care about durability.
	OutputWriter *OutputWriter

	// WorkflowDir is the directory of the workflow YAML, threaded into
	// the OutputWriter and gate evaluator. Empty falls back to
	// DefaultWorkingDir.
	WorkflowDir string

	mu      sync.Mutex
	signals map[string]chan struct{}
}

// UseSQLiteState wires a SQLiteState as State, Recorder, and AuditSink in
// one call. Most callers want all three; this is the convenience.
func (r *Runner) UseSQLiteState(ss *SQLiteState) {
	r.State = ss
	r.Recorder = ss
	r.Audit = ss
}

// recorder returns the configured RunRecorder or a no-op fallback so the
// runner can call recorder methods unconditionally.
func (r *Runner) recorder() RunRecorder {
	if r.Recorder == nil {
		return noopRunRecorder{}
	}
	return r.Recorder
}

// audit returns the configured AuditSink or a no-op fallback.
func (r *Runner) audit() AuditSink {
	if r.Audit == nil {
		return noopAuditSink{}
	}
	return r.Audit
}

// permissions returns the configured PermissionEnforcer or the default.
func (r *Runner) permissions() PermissionEnforcer {
	if r.Permissions == nil {
		return DefaultPermissionEnforcer{}
	}
	return r.Permissions
}

// NewRunner returns a Runner with defaults applied. ai must be non-nil.
func NewRunner(ai AIExecutor) *Runner {
	return &Runner{
		AI:               ai,
		Logger:           slog.Default(),
		DefaultBashShell: "/bin/sh",
		signals:          make(map[string]chan struct{}),
	}
}

// Signal wakes any Gate node currently waiting on name. Calling Signal
// for a name no gate is waiting on is safe — the signal is buffered and
// the next Gate with that name will consume it immediately.
func (r *Runner) Signal(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch, ok := r.signals[name]
	if !ok {
		ch = make(chan struct{}, 1)
		r.signals[name] = ch
	}
	select {
	case ch <- struct{}{}:
	default: // already signaled; Gate will consume
	}
}

// gateChannel returns (and creates on first use) the buffered channel
// for signal name. Callers hold a channel they can select on; the Runner
// keeps the same channel so later Signal calls land here.
func (r *Runner) gateChannel(name string) chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch, ok := r.signals[name]
	if !ok {
		ch = make(chan struct{}, 1)
		r.signals[name] = ch
	}
	return ch
}

// Run executes a workflow end-to-end. Returns a RunReport even on failure
// so the caller can inspect partial state. An error return is for
// unrecoverable issues (validation, unresolvable inputs, cycles).
func (r *Runner) Run(ctx context.Context, w *Workflow, inputs map[string]string) (*RunReport, error) {
	return r.run(ctx, w, inputs, nil)
}

// Resume loads a previously saved Snapshot and re-executes the workflow,
// skipping any node listed in Snapshot.Completed. Outputs from completed
// nodes are restored from the snapshot so downstream nodes can reference
// them via {{ .Outputs.<id> }}.
//
// Only idempotent DAGs are safely resumable. The saved outputs are
// trusted as-is; nodes are not re-run to verify them.
func (r *Runner) Resume(ctx context.Context, w *Workflow, st State) (*RunReport, error) {
	snap, err := st.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("resume: load snapshot: %w", err)
	}
	if snap == nil {
		// No checkpoint — run from scratch.
		return r.run(ctx, w, nil, nil)
	}
	return r.run(ctx, w, snap.Inputs, snap)
}

// run is the shared implementation of Run and Resume.
//
//nolint:gocyclo // sequential node loop with checkpoint + audit; splitting further hurts readability
func (r *Runner) run(ctx context.Context, w *Workflow, inputs map[string]string, snap *Snapshot) (*RunReport, error) {
	if err := w.Validate(); err != nil {
		return nil, err
	}
	merged, err := mergeInputs(w.Inputs, inputs)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	rep := &RunReport{
		Workflow: w.Name,
		Inputs:   merged,
		Started:  now,
	}

	// Generate a stable run_id for this invocation. Threaded through every
	// audit event and per-attempt record so the substrate can join across
	// tables. Resume re-uses the run_id stored on the snapshot when present.
	runID := uuid.NewString()
	if snap != nil && snap.RunID != "" {
		runID = snap.RunID
	}

	nc := &nodeContext{
		ctx:     ctx,
		inputs:  merged,
		outputs: make(map[string]string, len(w.Nodes)),
		env:     w.Env,
		runID:   runID,
	}

	// Restore outputs from a prior snapshot so downstream templates resolve.
	if snap != nil {
		for id, out := range snap.Outputs {
			nc.outputs[id] = out
		}
	}

	snapStarted := now
	if snap != nil {
		snapStarted = snap.Started
	}

	r.beginRunRecord(ctx, runID, w.Name, merged, snapStarted)

	order, err := topoOrder(w.Nodes)
	if err != nil {
		r.finishRunRecord(ctx, runID, RunStateFailed, err.Error())
		return rep, err
	}

	finalState := RunStateSucceeded
	var runErr error
	executed := 0
	for i, id := range order {
		if ctx.Err() != nil {
			rep.Finished = time.Now()
			r.markSkippedDownstream(ctx, runID, w.Nodes, order[i:], "context-cancelled")
			r.finishRunRecord(ctx, runID, RunStateCancelled, ctx.Err().Error())
			return rep, ctx.Err()
		}

		// Skip already-completed nodes when resuming. The audit log for
		// the original run already captured their transitions; emitting
		// duplicates here would corrupt the timeline.
		if snap != nil && snap.Completed[id] {
			r.Logger.Debug("node skipped (completed in snapshot)", "node", id)
			continue
		}

		node := findNode(w.Nodes, id)
		res := r.executeNode(nc, node)
		rep.Results = append(rep.Results, res)
		executed++
		if res.Error != nil {
			rep.Finished = time.Now()
			runErr = fmt.Errorf("node %q: %w", node.ID, res.Error)
			finalState = RunStateFailed
			r.markSkippedDownstream(ctx, runID, w.Nodes, order[i+1:], "upstream-failed")
			r.finishRunRecord(ctx, runID, finalState, runErr.Error())
			return rep, runErr
		}
		nc.outputs[node.ID] = res.Output

		snap = r.checkpoint(ctx, w.Name, runID, merged, snapStarted, snap, node.ID, res.Output)
	}
	rep.Finished = time.Now()
	rep.Succeeded = executed > 0
	r.finishRunRecord(ctx, runID, finalState, "")
	return rep, nil
}

// beginRunRecord initialises the run-level rows and emits the run-start
// audit event. Errors are logged but do not abort the run — the substrate
// goal is "best-effort recording, never block execution".
func (r *Runner) beginRunRecord(ctx context.Context, runID, workflowName string, inputs map[string]string, started time.Time) {
	inputsJSON, _ := json.Marshal(inputs)
	if err := r.recorder().BeginRun(ctx, RunRecord{
		RunID:        runID,
		WorkflowName: workflowName,
		State:        RunStateRunning,
		InputsJSON:   string(inputsJSON),
		StartedAt:    started,
		Trigger:      r.triggerOrDefault(),
		TriggeredBy:  r.TriggeredBy,
	}); err != nil {
		r.Logger.Warn("recorder BeginRun failed", "run_id", runID, "err", err)
	}
	if err := r.audit().Emit(ctx, AuditEvent{
		RunID:      runID,
		FromState:  string(RunStatePending),
		ToState:    string(RunStateRunning),
		Reason:     "run-started",
		Actor:      formatActor(r.TriggeredBy),
		OccurredAt: started,
	}); err != nil {
		r.Logger.Warn("audit emit failed", "run_id", runID, "err", err)
	}
}

// finishRunRecord marks the run terminal in the recorder + audit log.
func (r *Runner) finishRunRecord(ctx context.Context, runID string, state RunState, errMsg string) {
	now := time.Now()
	if err := r.recorder().FinishRun(ctx, runID, state, now, errMsg); err != nil {
		r.Logger.Warn("recorder FinishRun failed", "run_id", runID, "err", err)
	}
	if err := r.audit().Emit(ctx, AuditEvent{
		RunID:      runID,
		FromState:  string(RunStateRunning),
		ToState:    string(state),
		Reason:     errMsg,
		Actor:      formatActor(r.TriggeredBy),
		OccurredAt: now,
	}); err != nil {
		r.Logger.Warn("audit emit failed", "run_id", runID, "err", err)
	}
}

// markSkippedDownstream emits pending→skipped events for every node that
// will not execute because of an upstream failure or a cancelled run. The
// nodes table reflects the same skip state, so `workflow status` shows
// the full picture rather than a truncated DAG.
func (r *Runner) markSkippedDownstream(ctx context.Context, runID string, all []Node, remaining []string, reason string) {
	now := time.Now()
	for _, id := range remaining {
		_ = r.recorder().UpsertNode(ctx, NodeRecord{
			RunID:       runID,
			NodeID:      id,
			State:       NodeStateSkipped,
			FinishedAt:  now,
			Error:       reason,
		})
		_ = r.audit().Emit(ctx, AuditEvent{
			RunID:      runID,
			NodeID:     id,
			FromState:  string(NodeStatePending),
			ToState:    string(NodeStateSkipped),
			Reason:     reason,
			Actor:      "system",
			OccurredAt: now,
		})
	}
	_ = all // reserved for future cycle-detection diagnostics
}

func (r *Runner) triggerOrDefault() string {
	if r.Trigger == "" {
		return "sdk"
	}
	return r.Trigger
}

// checkpoint saves the completed node to State (if configured). It returns
// the updated snapshot so the caller can carry it forward.
func (r *Runner) checkpoint(ctx context.Context, wfName, runID string, inputs map[string]string, started time.Time, snap *Snapshot, nodeID, output string) *Snapshot {
	if r.State == nil {
		return snap
	}
	if snap == nil {
		snap = &Snapshot{
			Workflow:  wfName,
			RunID:     runID,
			Inputs:    inputs,
			Outputs:   make(map[string]string),
			Completed: make(map[string]bool),
			Started:   started,
		}
	}
	if snap.Outputs == nil {
		snap.Outputs = make(map[string]string)
	}
	if snap.Completed == nil {
		snap.Completed = make(map[string]bool)
	}
	if snap.RunID == "" {
		snap.RunID = runID
	}
	snap.Outputs[nodeID] = output
	snap.Completed[nodeID] = true
	snap.UpdatedAt = time.Now()
	if saveErr := r.State.Save(ctx, *snap); saveErr != nil {
		r.Logger.Warn("state save failed", "node", nodeID, "err", saveErr)
	}
	return snap
}

// executeNode dispatches to the kind-specific executor. All executors
// receive the same nodeContext so they can read inputs + upstream outputs.
func (r *Runner) executeNode(nc *nodeContext, node *Node) Result {
	res := Result{NodeID: node.ID, Started: time.Now(), Meta: make(map[string]any)}

	// Evaluate the When guard before timeout setup or child-ctx
	// allocation — a skipped node should be observationally identical
	// to one that was never run. A malformed When expression is a user
	// error and fails the node (not the opposite: silently running it
	// would mask the typo).
	if node.When != "" {
		shouldRun, err := evalCondition(node.When, nc)
		if err != nil {
			res.Error = fmt.Errorf("when: %w", err)
			res.Finished = time.Now()
			r.recordNodeFinished(nc, node, &res, NodeStateFailed, "when-eval-failed")
			return res
		}
		if !shouldRun {
			r.Logger.Debug("node skipped by when clause", "node", node.ID, "when", node.When)
			res.Meta["skipped"] = true
			res.Finished = time.Now()
			r.recordNodeFinished(nc, node, &res, NodeStateSkipped, "when-false")
			return res
		}
	}

	r.recordNodeStarted(nc, node, &res)

	ctx := nc.ctx
	if node.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(nc.ctx, node.Timeout)
		defer cancel()
	}
	childNC := &nodeContext{ctx: ctx, inputs: nc.inputs, outputs: nc.outputs, env: nc.env, runID: nc.runID}

	// Gate nodes never retry — a gate waiting on a signal should keep
	// waiting, not re-enter on each attempt.
	policy := node.Retry
	if node.Kind == KindGate {
		policy = nil
	}

	r.runWithRetry(childNC, node, &res, policy)

	// Phase 1 substrate gate: exit gates run after the node body
	// succeeded. Any failing gate transitions the node to failed.
	if res.Error == nil && len(node.ExitGate) > 0 {
		gctx := r.exitGateContext(nc, node, &res)
		if err := EvaluateExitGates(childNC.ctx, node.ExitGate, gctx); err != nil {
			res.Error = err
		}
	}

	// Phase 1 substrate gate: declared outputs must exist (for non-
	// ephemeral durability tiers) before we mark the node succeeded.
	// This is what makes "succeeded" a contract — operators see a
	// failure rather than a green check that hides a missing artifact.
	if res.Error == nil && len(node.Outputs) > 0 && r.OutputWriter != nil {
		if err := r.OutputWriter.MaterialiseOutputs(childNC.ctx, nc.runID, node.ID, node.Outputs, nc); err != nil {
			res.Error = err
		}
	}

	res.Finished = time.Now()

	state := NodeStateSucceeded
	reason := ""
	if res.Error != nil {
		state = NodeStateFailed
		reason = res.Error.Error()
	}
	r.recordNodeFinished(nc, node, &res, state, reason)
	return res
}

// exitGateContext builds the ExitGateContext the gate evaluator
// receives. Outputs are reified as a typed map keyed by node id so
// gates can target outputs.<node-id> without a second naming layer.
func (r *Runner) exitGateContext(nc *nodeContext, node *Node, res *Result) ExitGateContext {
	outs := make(map[string]any, len(nc.outputs)+1)
	for id, v := range nc.outputs {
		outs[id] = v
	}
	// The current node's primary output is also exposed as outputs.<this-id>.
	outs[node.ID] = res.Output
	return ExitGateContext{
		NodeID:      node.ID,
		RunID:       nc.runID,
		Inputs:      nc.inputs,
		Outputs:     outs,
		WorkflowDir: r.WorkflowDir,
		Env:         nc.env,
	}
}

// recordNodeStarted emits the pending → running transition and a
// running-state nodes row. The runner calls this once, before the first
// attempt; later attempts append to node_attempts only.
func (r *Runner) recordNodeStarted(nc *nodeContext, node *Node, res *Result) {
	if nc.runID == "" {
		return
	}
	if err := r.recorder().UpsertNode(nc.ctx, NodeRecord{
		RunID:     nc.runID,
		NodeID:    node.ID,
		State:     NodeStateRunning,
		StartedAt: res.Started,
		RoleUsed:  nodeRole(node),
		ModelUsed: nodeModel(node),
	}); err != nil {
		r.Logger.Warn("recorder UpsertNode(running) failed", "node", node.ID, "err", err)
	}
	if err := r.audit().Emit(nc.ctx, AuditEvent{
		RunID:      nc.runID,
		NodeID:     node.ID,
		FromState:  string(NodeStatePending),
		ToState:    string(NodeStateRunning),
		Actor:      "system",
		OccurredAt: res.Started,
	}); err != nil {
		r.Logger.Warn("audit emit(running) failed", "node", node.ID, "err", err)
	}
}

// recordNodeFinished emits the running → terminal transition and updates
// the nodes row with attempt count, output, and any error.
func (r *Runner) recordNodeFinished(nc *nodeContext, node *Node, res *Result, state NodeState, reason string) {
	if nc.runID == "" {
		return
	}
	attempts, _ := res.Meta["attempts"].(int)
	if err := r.recorder().UpsertNode(nc.ctx, NodeRecord{
		RunID:      nc.runID,
		NodeID:     node.ID,
		State:      state,
		Attempts:   attempts,
		RoleUsed:   nodeRole(node),
		ModelUsed:  nodeModel(node),
		Output:     res.Output,
		StartedAt:  res.Started,
		FinishedAt: res.Finished,
		Error:      reason,
	}); err != nil {
		r.Logger.Warn("recorder UpsertNode(finish) failed", "node", node.ID, "err", err)
	}
	from := NodeStateRunning
	if state == NodeStateSkipped {
		from = NodeStatePending
	}
	if err := r.audit().Emit(nc.ctx, AuditEvent{
		RunID:      nc.runID,
		NodeID:     node.ID,
		FromState:  string(from),
		ToState:    string(state),
		Reason:     reason,
		Actor:      "system",
		OccurredAt: res.Finished,
	}); err != nil {
		r.Logger.Warn("audit emit(finish) failed", "node", node.ID, "err", err)
	}
}

// nodeModel returns the model id for AI nodes. For non-AI nodes it
// returns "" — the column allows NULL.
//
// Phase 1 introduces role-based resolution: the model field on an
// AINode may be empty when role is set (resolution happens at run
// time). The runner records the resolved model on the nodes row via
// the recorder; this helper is the static-time fallback for nodes that
// haven't run yet.
func nodeModel(n *Node) string {
	if n.Kind == KindAI && n.AI != nil {
		if n.AI.Model != "" {
			return n.AI.Model
		}
	}
	return ""
}

// nodeRole returns the declared role for AI nodes, or empty for any
// other kind. Phase 1 records this on the nodes table so audit
// queries can group by "what role was this node?" without inferring
// from the model id.
func nodeRole(n *Node) string {
	if n.Kind == KindAI && n.AI != nil {
		return n.AI.Role
	}
	return ""
}

// runWithRetry executes the node's kind-specific logic, retrying on failure
// according to policy. It mutates res in place.
//
//nolint:gocyclo // retry loop + kind dispatch + per-attempt recording
func (r *Runner) runWithRetry(nc *nodeContext, node *Node, res *Result, policy *RetryPolicy) {
	attempts := 0
	for {
		attempts++
		attemptStart := time.Now()
		r.dispatchKind(nc, node, res)
		attemptFinish := time.Now()
		res.Meta["attempts"] = attempts

		state := NodeStateSucceeded
		errClass := ""
		errMsg := ""
		if res.Error != nil {
			state = NodeStateFailed
			errClass = classifyError(res.Error)
			errMsg = res.Error.Error()
		}
		r.recordAttempt(nc, node, attempts, state, attemptStart, attemptFinish, errClass, errMsg)

		if res.Error == nil || policy == nil {
			return
		}

		maxAttempts := policy.MaxAttempts
		if maxAttempts <= 0 {
			maxAttempts = 1
		}

		if !retryAllowedByKinds(policy, node.Kind) || attempts >= maxAttempts {
			return
		}

		delay := retryDelay(policy, attempts)
		r.Logger.Warn("node failed, retrying", "node", node.ID, "attempt", attempts, "delay", delay, "err", res.Error)

		select {
		case <-nc.ctx.Done():
			res.Error = nc.ctx.Err()
			return
		case <-time.After(delay):
		}
	}
}

// recordAttempt writes one node_attempts row. Called once per attempt
// regardless of outcome — the row carries the attempt's state so retry
// stats are queryable.
func (r *Runner) recordAttempt(nc *nodeContext, node *Node, attemptNo int, state NodeState, started, finished time.Time, errClass, errMsg string) {
	if nc.runID == "" {
		return
	}
	if err := r.recorder().RecordAttempt(nc.ctx, AttemptRecord{
		RunID:        nc.runID,
		NodeID:       node.ID,
		AttemptNo:    attemptNo,
		State:        state,
		ModelUsed:    nodeModel(node),
		StartedAt:    started,
		FinishedAt:   finished,
		ErrorClass:   errClass,
		ErrorMessage: errMsg,
	}); err != nil {
		r.Logger.Warn("recorder RecordAttempt failed", "node", node.ID, "attempt", attemptNo, "err", err)
	}
}

// dispatchKind calls the appropriate executor for node.Kind and stores
// outputs and errors in res.
func (r *Runner) dispatchKind(nc *nodeContext, node *Node, res *Result) {
	switch node.Kind {
	case KindAI:
		out, err := r.executeAI(nc, node)
		res.Output, res.Error = out, err
	case KindBash:
		out, code, err := r.executeBash(nc, node)
		res.Output = out
		res.Meta["exit_code"] = code
		res.Error = err
	case KindGate:
		res.Error = r.executeGate(nc, node)
	case KindLoop:
		iters, err := r.executeLoop(nc, node)
		res.Meta["iterations"] = iters
		res.Error = err
	default:
		res.Error = fmt.Errorf("unknown kind %q", node.Kind)
	}
}

// retryAllowedByKinds returns true if the retry policy applies to kind.
// Empty OnlyKinds means all kinds are eligible.
func retryAllowedByKinds(policy *RetryPolicy, kind NodeKind) bool {
	if len(policy.OnlyKinds) == 0 {
		return true
	}
	for _, k := range policy.OnlyKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// retryDelay computes the exponential backoff delay for a given attempt (1-based).
func retryDelay(policy *RetryPolicy, attempt int) time.Duration {
	backoff := policy.Backoff
	if backoff <= 0 {
		backoff = time.Second
	}
	maxBackoff := policy.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = 30 * time.Second
	}
	delay := backoff
	for i := 1; i < attempt; i++ {
		delay *= 2
	}
	if delay > maxBackoff {
		delay = maxBackoff
	}
	return delay
}

// executeAI renders templates and delegates to Runner.AI. When a role
// resolver is configured the model is resolved from node.AI.Role
// before the executor is invoked; the resolved model is written back
// into the rendered AINode so the underlying executor sees one
// canonical field. Resolution failure is a node failure — a workflow
// declaring an unknown role should fail loudly.
func (r *Runner) executeAI(nc *nodeContext, node *Node) (string, error) {
	prompt, err := renderTemplate(node.AI.Prompt, nc)
	if err != nil {
		return "", fmt.Errorf("render prompt: %w", err)
	}
	system, err := renderTemplate(node.AI.System, nc)
	if err != nil {
		return "", fmt.Errorf("render system: %w", err)
	}
	rendered := *node.AI
	rendered.Prompt = prompt
	rendered.System = system

	if r.RoleResolver != nil && (node.AI.Role != "" || node.AI.ModelOverride != "") {
		req := roles.Request{
			Role:                 node.AI.Role,
			Model:                node.AI.Model,
			ModelOverride:        node.AI.ModelOverride,
			RequiredCapabilities: node.AI.RequiredCapabilities,
		}
		if node.Budget != nil && node.Budget.MaxDollars > 0 {
			req.MaxDollars = node.Budget.MaxDollars
		}
		resolved, err := r.RoleResolver.Resolve(req)
		if err != nil {
			return "", fmt.Errorf("resolve role %q: %w", node.AI.Role, err)
		}
		rendered.Model = resolved.Model
		if rendered.Effort == "" {
			rendered.Effort = resolved.Effort
		}
		// Hand the budget meter the per-node context so it can
		// attribute spend to the right row when the executor returns.
		if mx, ok := r.AI.(*MeteredAIExecutor); ok {
			mx.CurrentNode = node
			mx.CurrentNodeStarted = time.Now()
			defer func() { mx.CurrentNode = nil; mx.CurrentNodeStarted = time.Time{} }()
		}
	} else if mx, ok := r.AI.(*MeteredAIExecutor); ok {
		// Even when no role is set, a node with a Budget block needs
		// the meter to know which node it's charging.
		mx.CurrentNode = node
		mx.CurrentNodeStarted = time.Now()
		defer func() { mx.CurrentNode = nil; mx.CurrentNodeStarted = time.Time{} }()
	}

	return r.AI.Generate(nc.ctx, &rendered, nc.inputs, nc.outputs)
}

// executeBash runs the rendered command under /bin/sh -c and returns its
// stdout + exit code. Stderr is captured into the Meta map.
//
// WARNING — injection risk: {{.Inputs.foo}} in the Cmd template interpolates
// values directly into the shell command string. If those values contain shell
// metacharacters (;, &&, $(...), backticks) arbitrary commands can execute.
// Prefer the safe alternatives:
//   - Reference values as $INPUT_FOO / $OUTPUT_FOO env vars (auto-injected).
//   - Use the {{shq .Inputs.foo}} template function for explicit shell-quoting.
func (r *Runner) executeBash(nc *nodeContext, node *Node) (string, int, error) {
	rendered, err := renderTemplate(node.Bash.Cmd, nc)
	if err != nil {
		return "", 0, fmt.Errorf("render cmd: %w", err)
	}
	shell := r.DefaultBashShell
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.CommandContext(nc.ctx, shell, "-c", rendered)
	cmd.Dir = node.Bash.WorkingDir
	if cmd.Dir == "" {
		cmd.Dir = r.DefaultWorkingDir
	}
	// Phase 1 permission gate: the working dir must be on the
	// fs_write allowlist when one is declared. The check is
	// permissive when no allowlist exists — see DefaultPermissionEnforcer.
	if node.Permissions != nil {
		enf := r.permissions()
		if cmd.Dir != "" {
			if err := enf.CheckPath(node.Permissions, cmd.Dir, AccessWrite); err != nil {
				return "", 0, err
			}
		}
	}
	// Always start from a clean copy of the parent environment, then layer:
	// 1. node-declared env overrides, 2. INPUT_* / OUTPUT_* from workflow state.
	env := cmd.Environ()
	if len(node.Bash.Env) > 0 {
		env = append(env, envSlice(node.Bash.Env)...)
	}
	// Auto-expose workflow inputs/outputs so scripts can use $INPUT_FOO /
	// $OUTPUT_FOO without interpolating untrusted values into the command string.
	for k, v := range nc.inputs {
		env = append(env, envVarKey("INPUT_", k)+"="+v)
	}
	for k, v := range nc.outputs {
		env = append(env, envVarKey("OUTPUT_", k)+"="+v)
	}
	cmd.Env = env
	out, err := cmd.Output()
	code := cmd.ProcessState.ExitCode()
	if err != nil {
		// Non-zero exit is wrapped into ExitError. If the node allows it,
		// succeed with the exit code in Meta.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && node.Bash.AllowNonzeroExit {
			return string(out), code, nil
		}
		return string(out), code, fmt.Errorf("bash: %w", err)
	}
	return string(out), code, nil
}

// executeGate blocks until the signal arrives or the context/Timeout expires.
func (r *Runner) executeGate(nc *nodeContext, node *Node) error {
	ch := r.gateChannel(node.Gate.Name)
	r.Logger.Info("gate waiting", "node", node.ID, "signal", node.Gate.Name)
	var timeout <-chan time.Time
	if r.SignalTimeout > 0 {
		timer := time.NewTimer(r.SignalTimeout)
		defer timer.Stop()
		timeout = timer.C
	}
	select {
	case <-ch:
		return nil
	case <-nc.ctx.Done():
		return nc.ctx.Err()
	case <-timeout:
		return fmt.Errorf("gate %q timed out after %s", node.Gate.Name, r.SignalTimeout)
	}
}

// executeLoop runs the nested DAG in either sequential or parallel mode.
//
// Sequential mode (default): runs until Until evaluates true, bounded by
// MaxIters.
//
// Parallel mode: runs exactly MaxIters iterations concurrently, limited
// to Concurrency goroutines. Each iteration gets an independent copy of
// the nodeContext so iterations don't share mutable output maps. The
// {{ .Iter }} template variable contains the 0-based iteration index.
func (r *Runner) executeLoop(nc *nodeContext, node *Node) (int, error) {
	if node.Loop.Parallel {
		return r.executeLoopParallel(nc, node)
	}
	return r.executeLoopSequential(nc, node)
}

// executeLoopSequential runs the nested DAG until Until is true or MaxIters
// is reached.
func (r *Runner) executeLoopSequential(nc *nodeContext, node *Node) (int, error) {
	maxIters := node.Loop.MaxIters
	if maxIters <= 0 {
		maxIters = 100
	}
	for i := 0; i < maxIters; i++ {
		if nc.ctx.Err() != nil {
			return i, nc.ctx.Err()
		}
		// Execute the nested DAG once.
		order, err := topoOrder(node.Loop.Nodes)
		if err != nil {
			return i, fmt.Errorf("loop nested topo: %w", err)
		}
		for _, id := range order {
			child := findNode(node.Loop.Nodes, id)
			res := r.executeNode(nc, child)
			nc.outputs[child.ID] = res.Output
			if res.Error != nil {
				return i + 1, fmt.Errorf("iter %d node %q: %w", i, child.ID, res.Error)
			}
		}
		done, err := evalCondition(node.Loop.Until, nc)
		if err != nil {
			return i + 1, fmt.Errorf("eval until: %w", err)
		}
		if done {
			return i + 1, nil
		}
	}
	return maxIters, fmt.Errorf("loop exceeded max_iters=%d", maxIters)
}

// executeLoopParallel runs exactly MaxIters iterations of the nested DAG
// concurrently. Iterations don't share mutable state; each gets its own
// output map seeded from the parent context. Concurrency limits the number
// of simultaneous goroutines; 0 means runtime.NumCPU().
func (r *Runner) executeLoopParallel(nc *nodeContext, node *Node) (int, error) {
	maxIters := node.Loop.MaxIters
	if maxIters <= 0 {
		return 0, nil
	}

	concurrency := node.Loop.Concurrency
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}

	order, err := topoOrder(node.Loop.Nodes)
	if err != nil {
		return 0, fmt.Errorf("loop nested topo: %w", err)
	}

	// Use a semaphore channel to cap concurrency.
	sem := make(chan struct{}, concurrency)
	eg, gCtx := errgroup.WithContext(nc.ctx)

	for i := 0; i < maxIters; i++ {
		iter := i
		eg.Go(func() error {
			// Acquire semaphore slot.
			select {
			case sem <- struct{}{}:
			case <-gCtx.Done():
				return gCtx.Err()
			}
			defer func() { <-sem }()

			// Build an isolated nodeContext for this iteration.
			// Seed from parent outputs so upstream nodes are visible.
			iterOutputs := make(map[string]string, len(nc.outputs)+1)
			for k, v := range nc.outputs {
				iterOutputs[k] = v
			}
			// Expose iteration index for templates.
			iterOutputs["Iter"] = fmt.Sprintf("%d", iter)
			iterNC := &nodeContext{
				ctx:    gCtx,
				inputs: nc.inputs,
				// Merge parent inputs + Iter into inputs so {{ .Inputs }} is
				// not polluted; expose Iter via a dedicated key in outputs.
				outputs: iterOutputs,
				env:     nc.env,
				runID:   nc.runID,
			}

			for _, id := range order {
				child := findNode(node.Loop.Nodes, id)
				res := r.executeNode(iterNC, child)
				iterNC.outputs[child.ID] = res.Output
				if res.Error != nil {
					return fmt.Errorf("parallel iter %d node %q: %w", iter, child.ID, res.Error)
				}
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return maxIters, err
	}
	return maxIters, nil
}

// mergeInputs fills in defaults and rejects missing required inputs.
func mergeInputs(spec []InputSpec, given map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(spec))
	for _, s := range spec {
		if v, ok := given[s.Name]; ok {
			out[s.Name] = v
			continue
		}
		if s.Default != "" {
			out[s.Name] = s.Default
			continue
		}
		if s.Required {
			return nil, fmt.Errorf("missing required input %q", s.Name)
		}
	}
	// Also allow undeclared inputs — the caller may pass through extras.
	for k, v := range given {
		if _, already := out[k]; !already {
			out[k] = v
		}
	}
	return out, nil
}

// topoOrder returns node IDs in topological (dependency-respecting) order.
// Nodes with no dependencies come first. Stable across runs so reports
// are easy to diff.
func topoOrder(nodes []Node) ([]string, error) {
	if err := detectCycle(nodes); err != nil {
		return nil, err
	}
	remaining := make(map[string]map[string]struct{}, len(nodes))
	idOrder := make([]string, len(nodes))
	for i := range nodes {
		idOrder[i] = nodes[i].ID
		rem := make(map[string]struct{}, len(nodes[i].Depends))
		for _, d := range nodes[i].Depends {
			rem[d] = struct{}{}
		}
		remaining[nodes[i].ID] = rem
	}
	var out []string
	for len(remaining) > 0 {
		progress := false
		for _, id := range idOrder {
			rem, present := remaining[id]
			if !present {
				continue
			}
			if len(rem) == 0 {
				out = append(out, id)
				delete(remaining, id)
				for other := range remaining {
					delete(remaining[other], id)
				}
				progress = true
			}
		}
		if !progress {
			// Should not happen after cycle detection, but guard anyway.
			return out, fmt.Errorf("topo: no progress (possible hidden cycle)")
		}
	}
	return out, nil
}

func findNode(nodes []Node, id string) *Node {
	for i := range nodes {
		if nodes[i].ID == id {
			return &nodes[i]
		}
	}
	return nil
}

// renderTemplate applies Go text/template substitution over t using the
// node's inputs + upstream outputs + env. Template functions are minimal
// by design — users embedding DSL logic into prompts encourages drift.
//
// Available template functions:
//   - shq <value>: shell-quote a value so it is safe to interpolate into a
//     shell command string (wraps in single quotes, escapes embedded quotes).
func renderTemplate(t string, nc *nodeContext) (string, error) {
	if !strings.Contains(t, "{{") {
		return t, nil // fast path: no templating needed
	}
	tpl, err := template.New("node").Option("missingkey=zero").Funcs(template.FuncMap{
		"shq": shellQuote,
	}).Parse(t)
	if err != nil {
		return "", err
	}
	data := map[string]any{
		"Inputs":  nc.inputs,
		"Outputs": nc.outputs,
		"Env":     nc.env,
		// Iter is set to the iteration index in parallel loop children.
		// It is surfaced in Outputs["Iter"] but also at top-level for
		// ergonomic {{ .Iter }} usage.
		"Iter": nc.outputs["Iter"],
	}
	var sb strings.Builder
	if err := tpl.Execute(&sb, data); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// evalCondition is a tiny grammar for loop.until. Supported forms:
//   - "Outputs.<id> == <literal>"     (equality)
//   - "Outputs.<id> != <literal>"     (inequality)
//   - "Outputs.<id> > <literal>"      (numeric comparison)
//   - "Outputs.<id>"                   (truthy: non-empty, non-"false", non-"0")
//
// Whitespace around the operator is optional. Literals are bare words
// (no quoting) — the goal is a tiny human-readable DSL, not a full
// expression language.
func evalCondition(cond string, nc *nodeContext) (bool, error) {
	cond = strings.TrimSpace(cond)
	if cond == "" {
		return false, fmt.Errorf("empty condition")
	}
	// Try binary operators in order of length (so == doesn't match first as =).
	for _, op := range []string{"==", "!=", ">"} {
		if idx := strings.Index(cond, op); idx > 0 {
			lhs := strings.TrimSpace(cond[:idx])
			rhs := strings.TrimSpace(cond[idx+len(op):])
			lv, err := resolvePath(lhs, nc)
			if err != nil {
				return false, err
			}
			switch op {
			case "==":
				return lv == rhs, nil
			case "!=":
				return lv != rhs, nil
			case ">":
				return compareNumeric(lv, rhs, func(a, b float64) bool { return a > b })
			}
		}
	}
	// No operator → truthy test.
	v, err := resolvePath(cond, nc)
	if err != nil {
		return false, err
	}
	return v != "" && !strings.EqualFold(v, "false") && v != "0", nil
}

// resolvePath looks up a dotted path like Outputs.stage3 or Inputs.name.
func resolvePath(path string, nc *nodeContext) (string, error) {
	parts := strings.SplitN(path, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("path %q must be <scope>.<name>", path)
	}
	switch parts[0] {
	case "Outputs":
		return nc.outputs[parts[1]], nil
	case "Inputs":
		return nc.inputs[parts[1]], nil
	case "Env":
		return nc.env[parts[1]], nil
	}
	return "", fmt.Errorf("unknown scope %q", parts[0])
}

func compareNumeric(a, b string, op func(x, y float64) bool) (bool, error) {
	var af, bf float64
	if _, err := fmt.Sscanf(a, "%f", &af); err != nil {
		return false, fmt.Errorf("not numeric: %q", a)
	}
	if _, err := fmt.Sscanf(b, "%f", &bf); err != nil {
		return false, fmt.Errorf("not numeric: %q", b)
	}
	return op(af, bf), nil
}

func envSlice(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

// envVarKey converts a workflow key name to a safe env var name with the
// given prefix. Non-alphanumeric characters are replaced with underscores.
func envVarKey(prefix, name string) string {
	var b strings.Builder
	b.WriteString(prefix)
	for _, r := range strings.ToUpper(name) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// shellQuote wraps s in single quotes so it is safe to interpolate into a
// shell command. Single quotes inside s are escaped by ending the current
// quoted string, inserting an escaped quote, and reopening.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
