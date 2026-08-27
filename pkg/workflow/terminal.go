package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"
)

const defaultTerminalPersistenceTimeout = 10 * time.Second

var errNestedTerminalPersistence = errors.New("workflow: nested terminal persistence failed")

// terminalEvidence is one required current-node write deferred because
// cancellation was already visible before the adapter call. Deferral lets the
// run-level terminal row and audit consume the cleanup budget first and avoids
// retrying a call that may already have committed.
type terminalEvidence struct {
	attempt *AttemptRecord
	node    *NodeRecord
	audit   *AuditEvent
}

// nodeOutcome keeps persistence policy out of the exported Result model. The
// runner exposes execution truth through result while carrying evidence errors
// and deferred terminal writes through nested private execution paths.
type nodeOutcome struct {
	result         Result
	persistenceErr error
	deferred       []terminalEvidence
}

func (o *nodeOutcome) absorb(child nodeOutcome) {
	o.persistenceErr = errors.Join(o.persistenceErr, child.persistenceErr)
	o.deferred = append(o.deferred, child.deferred...)
}

func contextDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func (o *nodeOutcome) deferAttempt(record AttemptRecord) {
	o.deferred = append(o.deferred, terminalEvidence{attempt: &record})
}

func (o *nodeOutcome) deferNode(record NodeRecord) {
	o.deferred = append(o.deferred, terminalEvidence{node: &record})
}

func (o *nodeOutcome) deferAudit(event AuditEvent) {
	o.deferred = append(o.deferred, terminalEvidence{audit: &event})
}

// beginRunRecord initialises the run-level rows and emits the run-start audit
// event. Initial recording stays best-effort for compatibility; once a run is
// begun, terminal persistence uses the required path below.
func (r *Runner) beginRunRecord(
	ctx context.Context,
	runID string,
	workflowName string,
	inputs map[string]string,
	started time.Time,
) {
	inputsJSON, _ := json.Marshal(inputs)
	if err := r.recorder().BeginRun(ctx, RunRecord{
		RunID:        runID,
		WorkflowName: workflowName,
		State:        RunStateRunning,
		InputsJSON:   string(inputsJSON),
		StartedAt:    started,
		Trigger:      r.triggerOrDefault(),
		TriggeredBy:  r.TriggeredBy,
		ModelVariant: r.ModelVariant,
	}); err != nil {
		r.Logger.Warn("recorder BeginRun failed", "run_id", runID, "err", err)
	}
	if err := r.emitAudit(ctx, AuditEvent{
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

type resumableRunRecorder interface {
	resumeRun(context.Context, string, AuditEvent) (RunState, error)
}

// resumeRunRecord makes SQLite re-entry explicit instead of emitting another
// false pending-to-running transition for an existing run. Other recorders
// retain the established BeginRun lifecycle call, but no transition is
// invented because RunRecorder cannot report the prior durable state.
func (r *Runner) resumeRunRecord(
	ctx context.Context,
	runID string,
	workflowName string,
	inputs map[string]string,
	started time.Time,
) error {
	recorder, ok := r.Recorder.(resumableRunRecorder)
	if !ok {
		inputsJSON, _ := json.Marshal(inputs)
		if err := r.recorder().BeginRun(ctx, RunRecord{
			RunID:        runID,
			WorkflowName: workflowName,
			State:        RunStateRunning,
			InputsJSON:   string(inputsJSON),
			StartedAt:    started,
			Trigger:      r.triggerOrDefault(),
			TriggeredBy:  r.TriggeredBy,
			ModelVariant: r.ModelVariant,
		}); err != nil {
			return fmt.Errorf("begin resumed run record: %w", err)
		}
		return nil
	}
	resumedAt := time.Now()
	event := AuditEvent{
		RunID:      runID,
		ToState:    string(RunStateRunning),
		Reason:     "run-resumed",
		Actor:      formatActor(r.TriggeredBy),
		OccurredAt: resumedAt,
	}
	from, err := recorder.resumeRun(ctx, runID, event)
	if err != nil {
		return fmt.Errorf("reopen run record: %w", err)
	}
	event.FromState = string(from)
	// SQLite persisted the canonical audit row atomically with the reopen.
	// External observers and OnAudit receive the original execution context
	// only after that transaction commits.
	durable, _ := r.Recorder.(AuditSink)
	r.emitObservationalAudit(ctx, r.Audit, durable, nil, event)
	r.notifyAudit(ctx, event)
	return nil
}

// recordNodeStarted emits the pending-to-running transition and a running
// node row once, before the first attempt.
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
	if err := r.emitAudit(nc.ctx, AuditEvent{
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

// recordNodeFinished persists or defers the running-to-terminal node fact and
// matching audit event.
func (r *Runner) recordNodeFinished(
	nc *nodeContext,
	node *Node,
	outcome *nodeOutcome,
	state NodeState,
	reason string,
) error {
	if nc.runID == "" {
		return nil
	}
	res := &outcome.result
	attempts, _ := res.Meta["attempts"].(int)
	record := NodeRecord{
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
	}
	from := NodeStateRunning
	if state == NodeStateSkipped {
		from = NodeStatePending
	}
	event := AuditEvent{
		RunID:      nc.runID,
		NodeID:     node.ID,
		FromState:  string(from),
		ToState:    string(state),
		Reason:     reason,
		Actor:      "system",
		OccurredAt: res.Finished,
	}

	if contextDone(nc.ctx) {
		outcome.deferNode(record)
		outcome.deferAudit(event)
		return nil
	}

	var persistErrs []error
	if err := r.recorder().UpsertNode(nc.ctx, record); err != nil {
		if nc.ctx.Err() != nil {
			// UpsertNode is keyed by run/node and is safe to repeat with the
			// same terminal fact after cancellation becomes visible.
			outcome.deferNode(record)
		} else {
			persistErrs = append(persistErrs, fmt.Errorf("record terminal node %q: %w", node.ID, err))
		}
	}
	if contextDone(nc.ctx) {
		outcome.deferAudit(event)
		return errors.Join(persistErrs...)
	}
	if err := r.emitTerminalAudit(nc.ctx, nc.ctx, event); err != nil {
		persistErrs = append(persistErrs, fmt.Errorf("emit terminal audit for node %q: %w", node.ID, err))
	}
	return errors.Join(persistErrs...)
}

// recordAttempt persists one attempt row, or defers immutable intent when
// cancellation is already visible before the adapter call.
func (r *Runner) recordAttempt(
	nc *nodeContext,
	node *Node,
	outcome *nodeOutcome,
	attemptNo int,
	state NodeState,
	started time.Time,
	finished time.Time,
	errClass string,
	errMsg string,
) error {
	if nc.runID == "" {
		return nil
	}
	record := AttemptRecord{
		RunID:        nc.runID,
		NodeID:       node.ID,
		AttemptNo:    attemptNo,
		State:        state,
		ModelUsed:    nodeModel(node),
		StartedAt:    started,
		FinishedAt:   finished,
		ErrorClass:   errClass,
		ErrorMessage: errMsg,
	}
	if contextDone(nc.ctx) {
		outcome.deferAttempt(record)
		return nil
	}
	return r.recorder().RecordAttempt(nc.ctx, record)
}

// terminalFinalization is the complete durable intent for closing one run.
// It is private so callers cannot detach execution contexts or sequence
// recorder and audit writes themselves.
type terminalFinalization struct {
	runID        string
	state        RunState
	finishedAt   time.Time
	errorMessage string
	evidence     []terminalEvidence
	skipped      []string
	skipReason   string
}

// finalizeRun persists the facts required to close a begun run. The cleanup
// context deliberately keeps caller values while replacing caller cancellation
// with one finite persistence deadline. It never reaches an executor or retry
// path. Run-level evidence is attempted before current-node detail and skips.
func (r *Runner) finalizeRun(runCtx context.Context, final terminalFinalization) error {
	persistCtx, cancel := r.newTerminalPersistenceContext(runCtx)
	defer cancel()

	var persistErrs []error
	if err := r.finishRunRecord(persistCtx, runCtx, final); err != nil {
		persistErrs = append(persistErrs, err)
	}
	if err := r.persistTerminalEvidence(persistCtx, runCtx, final.evidence); err != nil {
		persistErrs = append(persistErrs, err)
	}
	if err := r.markSkippedDownstream(persistCtx, runCtx, final); err != nil {
		persistErrs = append(persistErrs, err)
	}
	if err := errors.Join(persistErrs...); err != nil {
		return fmt.Errorf("workflow: persist terminal state for run %s: %w", final.runID, err)
	}
	return nil
}

func (r *Runner) finishRunRecord(
	persistCtx context.Context,
	hookCtx context.Context,
	final terminalFinalization,
) error {
	var persistErrs []error
	if err := r.recorder().FinishRun(
		persistCtx,
		final.runID,
		final.state,
		final.finishedAt,
		final.errorMessage,
	); err != nil {
		persistErrs = append(persistErrs, fmt.Errorf("finish run record: %w", err))
	}
	if err := r.emitTerminalAudit(persistCtx, hookCtx, AuditEvent{
		RunID:      final.runID,
		FromState:  string(RunStateRunning),
		ToState:    string(final.state),
		Reason:     final.errorMessage,
		Actor:      formatActor(r.TriggeredBy),
		OccurredAt: final.finishedAt,
	}); err != nil {
		persistErrs = append(persistErrs, fmt.Errorf("emit terminal audit: %w", err))
	}
	return errors.Join(persistErrs...)
}

func (r *Runner) persistTerminalEvidence(
	persistCtx context.Context,
	hookCtx context.Context,
	evidence []terminalEvidence,
) error {
	var persistErrs []error
	for _, item := range evidence {
		switch {
		case item.attempt != nil:
			if err := r.recorder().RecordAttempt(persistCtx, *item.attempt); err != nil {
				persistErrs = append(persistErrs, fmt.Errorf(
					"record terminal attempt for node %q: %w",
					item.attempt.NodeID,
					err,
				))
			}
		case item.node != nil:
			if err := r.recorder().UpsertNode(persistCtx, *item.node); err != nil {
				persistErrs = append(persistErrs, fmt.Errorf(
					"record terminal node %q: %w",
					item.node.NodeID,
					err,
				))
			}
		case item.audit != nil:
			if err := r.emitTerminalAudit(persistCtx, hookCtx, *item.audit); err != nil {
				persistErrs = append(persistErrs, fmt.Errorf(
					"emit terminal audit for node %q: %w",
					item.audit.NodeID,
					err,
				))
			}
		}
	}
	return errors.Join(persistErrs...)
}

// markSkippedDownstream persists pending-to-skipped evidence for nodes that
// cannot execute. The run-level terminal record is attempted first so a stalled
// lower-priority node adapter cannot strand the run as durably running.
func (r *Runner) markSkippedDownstream(
	persistCtx context.Context,
	hookCtx context.Context,
	final terminalFinalization,
) error {
	var persistErrs []error
	for _, id := range final.skipped {
		if err := r.recorder().UpsertNode(persistCtx, NodeRecord{
			RunID:      final.runID,
			NodeID:     id,
			State:      NodeStateSkipped,
			FinishedAt: final.finishedAt,
			Error:      final.skipReason,
		}); err != nil {
			persistErrs = append(persistErrs, fmt.Errorf("record skipped node %q: %w", id, err))
		}
		if err := r.emitTerminalAudit(persistCtx, hookCtx, AuditEvent{
			RunID:      final.runID,
			NodeID:     id,
			FromState:  string(NodeStatePending),
			ToState:    string(NodeStateSkipped),
			Reason:     final.skipReason,
			Actor:      "system",
			OccurredAt: final.finishedAt,
		}); err != nil {
			persistErrs = append(persistErrs, fmt.Errorf("emit skipped-node audit for %q: %w", id, err))
		}
	}
	return errors.Join(persistErrs...)
}

func (r *Runner) newTerminalPersistenceContext(parent context.Context) (context.Context, context.CancelFunc) {
	timeout := r.terminalPersistenceTimeout
	if timeout <= 0 {
		timeout = defaultTerminalPersistenceTimeout
	}
	return context.WithTimeout(context.WithoutCancel(parent), timeout)
}

// emitTerminalAudit separates the required local durability lane from Audit,
// which may fan out to Engram, OpenTelemetry, stdout, or another external
// observer. The required sink is resolved from current public wiring: Recorder
// must also implement AuditSink and appear directly in Audit or its fan-out.
// Only that sink receives cleanup authority; observers and OnAudit retain the
// original run context.
func (r *Runner) emitTerminalAudit(persistCtx, runCtx context.Context, event AuditEvent) error {
	durable := r.requiredAuditSink()
	var durableErr error
	if durable != nil {
		durableErr = durable.Emit(persistCtx, event)
	}
	r.emitObservationalAudit(runCtx, r.Audit, durable, durableErr, event)
	r.notifyAudit(runCtx, event)
	return durableErr
}

func (r *Runner) requiredAuditSink() AuditSink {
	durable, ok := r.Recorder.(AuditSink)
	if !ok || !auditContains(r.Audit, durable) {
		return nil
	}
	return durable
}

func auditContains(root, target AuditSink) bool {
	if sameAuditSink(root, target) {
		return true
	}
	multi, ok := root.(*MultiAuditSink)
	if !ok {
		return false
	}
	for _, child := range multi.Sinks {
		if auditContains(child, target) {
			return true
		}
	}
	return false
}

func (r *Runner) emitObservationalAudit(
	ctx context.Context,
	sink AuditSink,
	durable AuditSink,
	durableErr error,
	event AuditEvent,
) {
	switch current := sink.(type) {
	case nil:
		return
	case *MultiAuditSink:
		for _, child := range current.Sinks {
			if sameAuditSink(child, durable) {
				if durableErr != nil && current.OnError != nil {
					current.OnError(child, event, durableErr)
				}
				continue
			}
			if auditContains(child, durable) {
				r.emitObservationalAudit(ctx, child, durable, durableErr, event)
				continue
			}
			if err := child.Emit(ctx, event); err != nil && current.OnError != nil {
				current.OnError(child, event, err)
			}
		}
	default:
		if sameAuditSink(current, durable) {
			return
		}
		if err := current.Emit(ctx, event); err != nil {
			r.Logger.Warn("observational audit emit failed", "run_id", event.RunID, "node_id", event.NodeID, "err", err)
		}
	}
}

func sameAuditSink(left, right AuditSink) bool {
	if left == nil || right == nil {
		return false
	}
	leftType := reflect.TypeOf(left)
	if leftType != reflect.TypeOf(right) || !leftType.Comparable() {
		return false
	}
	return left == right
}

func uncompletedNodeIDs(ids []string, snap *Snapshot) []string {
	if len(ids) == 0 {
		return nil
	}
	remaining := make([]string, 0, len(ids))
	for _, id := range ids {
		if snap != nil && snap.Completed[id] {
			continue
		}
		remaining = append(remaining, id)
	}
	return remaining
}
