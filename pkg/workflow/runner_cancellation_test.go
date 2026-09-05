package workflow

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type cancellationTestAI struct {
	calls    atomic.Int32
	generate func(context.Context, *AINode) (string, error)
}

func (a *cancellationTestAI) Generate(
	ctx context.Context,
	node *AINode,
	_ map[string]string,
	_ map[string]string,
) (string, error) {
	a.calls.Add(1)
	if a.generate != nil {
		return a.generate(ctx, node)
	}
	return "ok", nil
}

func TestRunnerPersistsCancellationBeforeFirstNode(t *testing.T) {
	ss := openTestState(t)
	ctx, cancel := context.WithCancel(context.Background())
	ai := &cancellationTestAI{}
	runner := NewRunner(ai)
	runner.UseSQLiteState(ss)
	runner.Hooks = &Hooks{OnDefine: func(context.Context, DefinePayload) error {
		cancel()
		return nil
	}}
	wf := cancellationWorkflow("cancel-before-first")

	report, runErr := runner.Run(ctx, wf, nil)
	if !errors.Is(runErr, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", runErr)
	}
	if report == nil || report.Finished.IsZero() || report.Succeeded || len(report.Results) != 0 {
		t.Fatalf("report = %#v, want finished unsuccessful report without results", report)
	}
	if got := ai.calls.Load(); got != 0 {
		t.Fatalf("executor calls = %d, want 0", got)
	}

	status := requireCancellationStatus(t, ss, "context canceled")
	if len(status.Nodes) != 2 {
		t.Fatalf("nodes = %#v, want two skipped nodes", status.Nodes)
	}
	for _, node := range status.Nodes {
		if node.State != NodeStateSkipped || node.FinishedAt == nil || node.Error != "context-cancelled" {
			t.Errorf("node %q = %#v, want finished skipped/context-cancelled", node.NodeID, node)
		}
		requireNodeTerminalAudit(t, ss, node, NodeStatePending, NodeStateSkipped)
	}
	requireRunTerminalAudit(t, ss, RunStateCancelled)
}

func TestRunnerCancellationAtEnforceBoundaryStopsDispatch(t *testing.T) {
	ss := openTestState(t)
	ctx, cancel := context.WithCancel(context.Background())
	ai := &cancellationTestAI{}
	runner := NewRunner(ai)
	runner.UseSQLiteState(ss)
	runner.Hooks = &Hooks{OnEnforce: func(context.Context, EnforcePayload) error {
		cancel()
		return nil
	}}
	wf := cancellationWorkflow("cancel-at-enforce")
	wf.Nodes[0].Retry = &RetryPolicy{MaxAttempts: 3}

	report, runErr := runner.Run(ctx, wf, nil)
	if !errors.Is(runErr, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", runErr)
	}
	if report == nil || report.Finished.IsZero() || report.Succeeded || len(report.Results) != 1 {
		t.Fatalf("report = %#v, want one cancelled result and unsuccessful finish", report)
	}
	if got := ai.calls.Load(); got != 0 {
		t.Fatalf("executor calls = %d, want 0", got)
	}

	status := requireCancellationStatus(t, ss, "context canceled")
	first := requireNodeStatus(t, status, "first")
	if first.State != NodeStateFailed || first.FinishedAt == nil || !strings.Contains(first.Error, "context canceled") {
		t.Errorf("first node = %#v, want finished failed context error", first)
	}
	requireNodeTerminalAudit(t, ss, first, NodeStateRunning, NodeStateFailed)
	second := requireNodeStatus(t, status, "second")
	if second.State != NodeStateSkipped || second.FinishedAt == nil || second.Error != "context-cancelled" {
		t.Errorf("second node = %#v, want finished skipped/context-cancelled", second)
	}
	requireNodeTerminalAudit(t, ss, second, NodeStatePending, NodeStateSkipped)
	requireRunTerminalAudit(t, ss, RunStateCancelled)
}

func TestRunnerPersistsCancellationDuringNode(t *testing.T) {
	ss := openTestState(t)
	ctx, cancel := context.WithCancel(context.Background())
	ai := &cancellationTestAI{generate: func(callCtx context.Context, _ *AINode) (string, error) {
		cancel()
		return "", callCtx.Err()
	}}
	runner := NewRunner(ai)
	runner.UseSQLiteState(ss)
	wf := cancellationWorkflow("cancel-during-node")
	wf.Nodes[0].Retry = &RetryPolicy{MaxAttempts: 3}

	report, runErr := runner.Run(ctx, wf, nil)
	if !errors.Is(runErr, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", runErr)
	}
	if report == nil || report.Finished.IsZero() || report.Succeeded || len(report.Results) != 1 {
		t.Fatalf("report = %#v, want one failed result and unsuccessful finish", report)
	}
	if got := ai.calls.Load(); got != 1 {
		t.Fatalf("executor calls = %d, want 1", got)
	}
	var attemptCount int
	if err := ss.DB().QueryRow(`SELECT COUNT(*) FROM node_attempts WHERE run_id = ? AND node_id = 'first'`, ss.RunID()).Scan(&attemptCount); err != nil {
		t.Fatalf("count attempts: %v", err)
	}
	if attemptCount != 1 {
		t.Fatalf("attempt rows = %d, want 1 despite retry policy", attemptCount)
	}

	status := requireCancellationStatus(t, ss, "context canceled")
	first := requireNodeStatus(t, status, "first")
	if first.State != NodeStateFailed || first.FinishedAt == nil || !strings.Contains(first.Error, "context canceled") {
		t.Errorf("first node = %#v, want finished failed context error", first)
	}
	requireNodeTerminalAudit(t, ss, first, NodeStateRunning, NodeStateFailed)
	second := requireNodeStatus(t, status, "second")
	if second.State != NodeStateSkipped || second.FinishedAt == nil || second.Error != "context-cancelled" {
		t.Errorf("second node = %#v, want finished skipped/context-cancelled", second)
	}
	requireNodeTerminalAudit(t, ss, second, NodeStatePending, NodeStateSkipped)
	requireRunTerminalAudit(t, ss, RunStateCancelled)
}

func TestRunnerCancellationAfterFinalNodeCannotReportSuccess(t *testing.T) {
	ss := openTestState(t)
	ctx, cancel := context.WithCancel(context.Background())
	ai := &cancellationTestAI{}
	runner := NewRunner(ai)
	runner.UseSQLiteState(ss)
	runner.Hooks = &Hooks{OnAudit: func(_ context.Context, payload AuditPayload) error {
		if payload.Event.NodeID == "only" && payload.Event.ToState == string(NodeStateSucceeded) {
			cancel()
		}
		return nil
	}}
	wf := &Workflow{
		Name: "cancel-after-final", Version: "1",
		Nodes: []Node{{ID: "only", Kind: KindAI, AI: &AINode{Prompt: "one"}}},
	}

	report, runErr := runner.Run(ctx, wf, nil)
	if !errors.Is(runErr, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", runErr)
	}
	if report == nil || report.Finished.IsZero() || !report.Succeeded || len(report.Results) != 1 {
		t.Fatalf("report = %#v, want one successfully executed node and cancelled run error", report)
	}
	if got := ai.calls.Load(); got != 1 {
		t.Fatalf("executor calls = %d, want 1", got)
	}

	status := requireCancellationStatus(t, ss, "context canceled")
	only := requireNodeStatus(t, status, "only")
	if only.State != NodeStateSucceeded || only.FinishedAt == nil {
		t.Errorf("only node = %#v, want durable success before run cancellation", only)
	}
	requireNodeTerminalAudit(t, ss, only, NodeStateRunning, NodeStateSucceeded)
	requireRunTerminalAudit(t, ss, RunStateCancelled)
}

func TestRunnerCancellationDuringTerminalNodeWriteFlushesDeferredAudit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var records []NodeRecord
	var events []AuditEvent
	durable := &terminalDurableProbe{
		terminalRecorderProbe: &terminalRecorderProbe{upsert: func(_ context.Context, record NodeRecord) error {
			records = append(records, record)
			if record.NodeID == "only" && record.State == NodeStateSucceeded {
				cancel()
			}
			return nil
		}},
		emit: func(_ context.Context, event AuditEvent) error {
			events = append(events, event)
			return nil
		},
	}
	ai := &cancellationTestAI{}
	runner := NewRunner(ai)
	runner.Recorder = durable
	runner.Audit = durable
	wf := &Workflow{
		Name: "cancel-during-terminal-node-write", Version: "1",
		Nodes: []Node{{ID: "only", Kind: KindAI, AI: &AINode{Prompt: "one"}}},
	}

	report, runErr := runner.Run(ctx, wf, nil)
	if !errors.Is(runErr, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", runErr)
	}
	if report == nil || !report.Succeeded || len(report.Results) != 1 || report.Results[0].Error != nil {
		t.Fatalf("report = %#v, want successful node execution plus cancelled run", report)
	}
	if got := ai.calls.Load(); got != 1 {
		t.Fatalf("executor calls = %d, want 1", got)
	}
	assertNodeEvidenceReason(t, records, events, "only", NodeStateSucceeded, "")
}

type terminalRecorderProbe struct {
	begin         func(context.Context, RunRecord) error
	finish        func(context.Context) error
	upsert        func(context.Context, NodeRecord) error
	recordAttempt func(context.Context, AttemptRecord) error
}

func (p *terminalRecorderProbe) BeginRun(ctx context.Context, record RunRecord) error {
	if p.begin != nil {
		return p.begin(ctx, record)
	}
	return nil
}

func (p *terminalRecorderProbe) UpsertNode(ctx context.Context, record NodeRecord) error {
	if p.upsert != nil {
		return p.upsert(ctx, record)
	}
	return nil
}

func (p *terminalRecorderProbe) RecordAttempt(ctx context.Context, record AttemptRecord) error {
	if p.recordAttempt != nil {
		return p.recordAttempt(ctx, record)
	}
	return nil
}

func (p *terminalRecorderProbe) FinishRun(
	ctx context.Context,
	_ string,
	_ RunState,
	_ time.Time,
	_ string,
) error {
	if p.finish == nil {
		return nil
	}
	return p.finish(ctx)
}

type terminalAuditProbe struct {
	emit func(context.Context, AuditEvent) error
}

func (p *terminalAuditProbe) Emit(ctx context.Context, event AuditEvent) error {
	if p.emit == nil {
		return nil
	}
	return p.emit(ctx, event)
}

type terminalDurableProbe struct {
	*terminalRecorderProbe
	emit func(context.Context, AuditEvent) error
}

func (p *terminalDurableProbe) Emit(ctx context.Context, event AuditEvent) error {
	if p.emit == nil {
		return nil
	}
	return p.emit(ctx, event)
}

type stallingSQLiteAdapter struct {
	*SQLiteState
	finishCalled     bool
	finishContextErr error
	attemptAfterRun  bool
}

func (a *stallingSQLiteAdapter) FinishRun(
	ctx context.Context,
	runID string,
	state RunState,
	finishedAt time.Time,
	errMsg string,
) error {
	a.finishCalled = true
	a.finishContextErr = ctx.Err()
	return a.SQLiteState.FinishRun(ctx, runID, state, finishedAt, errMsg)
}

func (a *stallingSQLiteAdapter) RecordAttempt(ctx context.Context, _ AttemptRecord) error {
	a.attemptAfterRun = a.finishCalled
	<-ctx.Done()
	return ctx.Err()
}

type terminalContextValueKey struct{}

func TestRunnerTerminalPersistenceContextIsBounded(t *testing.T) {
	for _, adapter := range []string{"recorder", "audit"} {
		t.Run(adapter, func(t *testing.T) {
			const contextValue = "terminal-value"
			var (
				observedValue    any
				observedDeadline bool
				observedEntryErr error
				hookContextErr   error
				observerCtxErr   error
				fanoutErr        error
			)
			stall := func(ctx context.Context) error {
				observedValue = ctx.Value(terminalContextValueKey{})
				_, observedDeadline = ctx.Deadline()
				observedEntryErr = ctx.Err()
				<-ctx.Done()
				return ctx.Err()
			}

			runner := NewRunner(&cancellationTestAI{})
			runner.terminalPersistenceTimeout = 100 * time.Millisecond
			durable := &terminalDurableProbe{terminalRecorderProbe: &terminalRecorderProbe{}}
			if adapter == "recorder" {
				durable.finish = stall
			} else {
				durable.emit = func(ctx context.Context, event AuditEvent) error {
					if event.NodeID == "" && event.FromState == string(RunStateRunning) {
						return stall(ctx)
					}
					return nil
				}
			}
			observer := &terminalAuditProbe{emit: func(ctx context.Context, event AuditEvent) error {
				if event.NodeID == "" && event.ToState == string(RunStateCancelled) {
					observerCtxErr = ctx.Err()
				}
				return nil
			}}
			runner.Recorder = durable
			runner.Audit = &MultiAuditSink{
				Sinks: []AuditSink{durable, observer},
				OnError: func(_ AuditSink, _ AuditEvent, err error) {
					fanoutErr = err
				},
			}

			parent := context.WithValue(context.Background(), terminalContextValueKey{}, contextValue)
			ctx, cancel := context.WithCancel(parent)
			runner.Hooks = &Hooks{
				OnDefine: func(context.Context, DefinePayload) error {
					cancel()
					return nil
				},
				OnAudit: func(hookCtx context.Context, payload AuditPayload) error {
					if payload.Event.NodeID == "" && payload.Event.ToState == string(RunStateCancelled) {
						hookContextErr = hookCtx.Err()
					}
					return nil
				},
			}

			started := time.Now()
			_, runErr := runner.Run(ctx, cancellationWorkflow("bounded-"+adapter), nil)
			elapsed := time.Since(started)
			if !errors.Is(runErr, context.Canceled) || !errors.Is(runErr, context.DeadlineExceeded) {
				t.Fatalf("Run error = %v, want cancellation and cleanup deadline", runErr)
			}
			if elapsed > 2*time.Second {
				t.Fatalf("Run elapsed = %s, want bounded return under 2s", elapsed)
			}
			if observedValue != contextValue || !observedDeadline || observedEntryErr != nil {
				t.Fatalf("cleanup context value=%v deadline=%v entryErr=%v", observedValue, observedDeadline, observedEntryErr)
			}
			if !errors.Is(hookContextErr, context.Canceled) {
				t.Fatalf("terminal OnAudit context error = %v, want original cancellation", hookContextErr)
			}
			if !errors.Is(observerCtxErr, context.Canceled) {
				t.Fatalf("observational Audit context error = %v, want original cancellation", observerCtxErr)
			}
			if adapter == "audit" && !errors.Is(fanoutErr, context.DeadlineExceeded) {
				t.Fatalf("MultiAuditSink OnError = %v, want durable deadline failure", fanoutErr)
			}
		})
	}
}

func TestRunnerPrioritizesRunTerminalStateBeforeStalledNodeEvidence(t *testing.T) {
	ss := openTestState(t)
	adapter := &stallingSQLiteAdapter{SQLiteState: ss}
	ctx, cancel := context.WithCancel(context.Background())
	ai := &cancellationTestAI{generate: func(callCtx context.Context, _ *AINode) (string, error) {
		cancel()
		return "", callCtx.Err()
	}}
	runner := NewRunner(ai)
	runner.State = ss
	runner.Recorder = adapter
	runner.Audit = adapter
	runner.terminalPersistenceTimeout = 100 * time.Millisecond
	wf := cancellationWorkflow("run-first-before-stall")
	wf.Nodes[0].Retry = &RetryPolicy{MaxAttempts: 3}

	started := time.Now()
	_, runErr := runner.Run(ctx, wf, nil)
	elapsed := time.Since(started)
	if !errors.Is(runErr, context.Canceled) || !errors.Is(runErr, context.DeadlineExceeded) {
		t.Fatalf("Run error = %v, want cancellation and cleanup deadline", runErr)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Run elapsed = %s, want one bounded cleanup window under 2s", elapsed)
	}
	if !adapter.finishCalled || adapter.finishContextErr != nil || !adapter.attemptAfterRun {
		t.Fatalf("finishCalled=%v finishContextErr=%v attemptAfterRun=%v", adapter.finishCalled, adapter.finishContextErr, adapter.attemptAfterRun)
	}
	if got := ai.calls.Load(); got != 1 {
		t.Fatalf("executor calls = %d, want 1 despite retry policy", got)
	}
	requireCancellationStatus(t, ss, "context canceled")
	requireRunTerminalAudit(t, ss, RunStateCancelled)
}

func TestRunnerReturnsAllTerminalPersistenceFailures(t *testing.T) {
	recorderErr := errors.New("terminal recorder unavailable")
	auditErr := errors.New("terminal audit unavailable")
	var recorderCalls, auditCalls atomic.Int32
	runner := NewRunner(&cancellationTestAI{})
	durable := &terminalDurableProbe{
		terminalRecorderProbe: &terminalRecorderProbe{finish: func(context.Context) error {
			recorderCalls.Add(1)
			return recorderErr
		}},
		emit: func(_ context.Context, event AuditEvent) error {
			if event.NodeID == "" && event.FromState == string(RunStateRunning) {
				auditCalls.Add(1)
				return auditErr
			}
			return nil
		},
	}
	runner.Recorder = durable
	runner.Audit = durable
	ctx, cancel := context.WithCancel(context.Background())
	runner.Hooks = &Hooks{OnDefine: func(context.Context, DefinePayload) error {
		cancel()
		return nil
	}}

	_, runErr := runner.Run(ctx, cancellationWorkflow("terminal-errors"), nil)
	for name, want := range map[string]error{
		"cancellation": context.Canceled,
		"recorder":     recorderErr,
		"audit":        auditErr,
	} {
		if !errors.Is(runErr, want) {
			t.Errorf("Run error = %v, want %s cause %v", runErr, name, want)
		}
	}
	if recorderCalls.Load() != 1 || auditCalls.Load() != 1 {
		t.Fatalf("terminal calls recorder=%d audit=%d, want 1 each", recorderCalls.Load(), auditCalls.Load())
	}
}

func TestRunnerReturnsCurrentNodePersistenceFailuresWithoutRetry(t *testing.T) {
	attemptErr := errors.New("attempt persistence unavailable")
	nodeErr := errors.New("node persistence unavailable")
	nodeAuditErr := errors.New("node audit persistence unavailable")
	skipErr := errors.New("skip persistence unavailable")
	skipAuditErr := errors.New("skip audit persistence unavailable")
	ctx, cancel := context.WithCancel(context.Background())
	ai := &cancellationTestAI{generate: func(callCtx context.Context, _ *AINode) (string, error) {
		cancel()
		return "", callCtx.Err()
	}}
	runner := NewRunner(ai)
	durable := &terminalDurableProbe{
		terminalRecorderProbe: &terminalRecorderProbe{
			recordAttempt: func(context.Context, AttemptRecord) error { return attemptErr },
			upsert: func(_ context.Context, record NodeRecord) error {
				switch {
				case record.NodeID == "first" && record.State == NodeStateFailed:
					return nodeErr
				case record.NodeID == "second" && record.State == NodeStateSkipped:
					return skipErr
				default:
					return nil
				}
			},
		},
		emit: func(_ context.Context, event AuditEvent) error {
			switch {
			case event.NodeID == "first" && event.ToState == string(NodeStateFailed):
				return nodeAuditErr
			case event.NodeID == "second" && event.ToState == string(NodeStateSkipped):
				return skipAuditErr
			default:
				return nil
			}
		},
	}
	runner.Recorder = durable
	runner.Audit = durable
	wf := cancellationWorkflow("node-terminal-errors")
	wf.Nodes[0].Retry = &RetryPolicy{MaxAttempts: 3}

	_, runErr := runner.Run(ctx, wf, nil)
	for name, want := range map[string]error{
		"cancellation": context.Canceled,
		"attempt":      attemptErr,
		"node":         nodeErr,
		"node audit":   nodeAuditErr,
		"skip":         skipErr,
		"skip audit":   skipAuditErr,
	} {
		if !errors.Is(runErr, want) {
			t.Errorf("Run error = %v, want %s cause %v", runErr, name, want)
		}
	}
	if got := ai.calls.Load(); got != 1 {
		t.Fatalf("executor calls = %d, want 1 despite retry policy", got)
	}
}

func TestRunnerPersistenceOnlyFailurePreservesExecutionTruth(t *testing.T) {
	attemptErr := errors.New("attempt evidence unavailable")
	var records []NodeRecord
	var events []AuditEvent
	durable := &terminalDurableProbe{
		terminalRecorderProbe: &terminalRecorderProbe{
			recordAttempt: func(context.Context, AttemptRecord) error { return attemptErr },
			upsert: func(_ context.Context, record NodeRecord) error {
				records = append(records, record)
				return nil
			},
		},
		emit: func(_ context.Context, event AuditEvent) error {
			events = append(events, event)
			return nil
		},
	}
	ai := &cancellationTestAI{}
	runner := NewRunner(ai)
	runner.Recorder = durable
	runner.Audit = durable

	report, runErr := runner.Run(context.Background(), cancellationWorkflow("persistence-only"), nil)
	if !errors.Is(runErr, attemptErr) {
		t.Fatalf("Run error = %v, want attempt persistence cause", runErr)
	}
	if report == nil || !report.Succeeded || len(report.Results) != 1 || report.Results[0].Error != nil {
		t.Fatalf("report = %#v, want one successful execution plus returned persistence error", report)
	}
	if got := ai.calls.Load(); got != 1 {
		t.Fatalf("executor calls = %d, want 1", got)
	}
	assertNodeEvidenceReason(t, records, events, "first", NodeStateFailed, "terminal-persistence-failed")
	assertNodeEvidenceReason(t, records, events, "second", NodeStateSkipped, "terminal-persistence-failed")
}

func assertNodeEvidenceReason(
	t *testing.T,
	records []NodeRecord,
	events []AuditEvent,
	nodeID string,
	state NodeState,
	reason string,
) {
	t.Helper()
	recordMatches := 0
	for _, record := range records {
		if record.NodeID == nodeID && record.State == state && record.Error == reason {
			recordMatches++
		}
	}
	eventMatches := 0
	for _, event := range events {
		if event.NodeID == nodeID && event.ToState == string(state) && event.Reason == reason {
			eventMatches++
		}
	}
	if recordMatches != 1 || eventMatches != 1 {
		t.Fatalf("node %q evidence records=%d audits=%d, want one %s/%q each", nodeID, recordMatches, eventMatches, state, reason)
	}
}

func TestRunnerCancellationOnResumePreservesCompletedNodes(t *testing.T) {
	ss := openTestState(t)
	const runID = "resume-run"
	started := time.Now().Add(-time.Minute)
	if err := ss.BeginRun(context.Background(), RunRecord{
		RunID:        runID,
		WorkflowName: "cancel-resume",
		State:        RunStateRunning,
		InputsJSON:   "{}",
		StartedAt:    started,
	}); err != nil {
		t.Fatalf("BeginRun: %v", err)
	}
	if err := ss.Emit(context.Background(), AuditEvent{
		RunID:      runID,
		FromState:  string(RunStatePending),
		ToState:    string(RunStateRunning),
		Reason:     "run-started",
		Actor:      "system",
		OccurredAt: started,
	}); err != nil {
		t.Fatalf("Emit run start: %v", err)
	}
	if err := ss.UpsertNode(context.Background(), NodeRecord{
		RunID:      runID,
		NodeID:     "first",
		State:      NodeStateSucceeded,
		Attempts:   1,
		Output:     "already-done",
		StartedAt:  started,
		FinishedAt: started.Add(time.Second),
	}); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	snapshot, err := ss.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if snapshot == nil || snapshot.RunID != runID || !snapshot.Completed["first"] {
		t.Fatalf("snapshot = %#v, want run %q with first completed", snapshot, runID)
	}

	ai := &cancellationTestAI{}
	runner := NewRunner(ai)
	runner.UseSQLiteState(ss)
	ctx, cancel := context.WithCancel(context.Background())
	runner.Hooks = &Hooks{OnDefine: func(context.Context, DefinePayload) error {
		cancel()
		return nil
	}}

	_, runErr := runner.Resume(ctx, cancellationWorkflow("cancel-resume"), ss)
	if !errors.Is(runErr, context.Canceled) {
		t.Fatalf("Resume error = %v, want context.Canceled", runErr)
	}
	if got := ai.calls.Load(); got != 0 {
		t.Fatalf("executor calls = %d, want 0", got)
	}
	status := requireCancellationStatus(t, ss, "context canceled")
	first := requireNodeStatus(t, status, "first")
	if first.State != NodeStateSucceeded || first.Output != "already-done" {
		t.Fatalf("first node = %#v, want preserved success", first)
	}
	second := requireNodeStatus(t, status, "second")
	if second.State != NodeStateSkipped || second.FinishedAt == nil {
		t.Fatalf("second node = %#v, want finished skip", second)
	}
	var runCount int
	if err := ss.DB().QueryRow(`SELECT COUNT(*) FROM runs`).Scan(&runCount); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if runCount != 1 {
		t.Fatalf("runs = %d, want original resumed run only", runCount)
	}
	events, err := Logs(context.Background(), ss.DB(), runID, LogsOptions{})
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	transitionCounts := map[string]int{}
	for _, event := range events {
		if event.NodeID == "" {
			transitionCounts[event.FromState+"->"+event.ToState]++
		}
	}
	for transition, want := range map[string]int{
		"pending->running":   1,
		"running->running":   1,
		"running->cancelled": 1,
	} {
		if got := transitionCounts[transition]; got != want {
			t.Errorf("run transition %s count = %d, want %d; all=%v", transition, got, want, transitionCounts)
		}
	}
	if len(transitionCounts) != 3 {
		t.Errorf("run transitions = %v, want only start, explicit resume, and cancellation", transitionCounts)
	}
}

func cancellationWorkflow(name string) *Workflow {
	return &Workflow{
		Name: name, Version: "1",
		Nodes: []Node{
			{ID: "first", Kind: KindAI, AI: &AINode{Prompt: "first"}},
			{ID: "second", Kind: KindAI, Depends: []string{"first"}, AI: &AINode{Prompt: "second"}},
		},
	}
}

func requireCancellationStatus(t *testing.T, ss *SQLiteState, errorSubstring string) *RunStatus {
	t.Helper()
	status, err := Status(context.Background(), ss.DB(), ss.RunID())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.State != RunStateCancelled || status.FinishedAt == nil || !strings.Contains(status.Error, errorSubstring) {
		t.Fatalf("status = %#v, want finished cancelled run containing error %q", status, errorSubstring)
	}
	return status
}

func requireNodeStatus(t *testing.T, status *RunStatus, nodeID string) NodeStatus {
	t.Helper()
	for _, node := range status.Nodes {
		if node.NodeID == nodeID {
			return node
		}
	}
	t.Fatalf("node %q absent from status %#v", nodeID, status.Nodes)
	return NodeStatus{}
}

func requireRunTerminalAudit(t *testing.T, ss *SQLiteState, state RunState) {
	t.Helper()
	status, err := Status(context.Background(), ss.DB(), ss.RunID())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	events, err := Logs(context.Background(), ss.DB(), ss.RunID(), LogsOptions{})
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	matching := 0
	terminal := 0
	for _, event := range events {
		if event.NodeID == "" && isTerminalRunState(RunState(event.ToState)) {
			terminal++
		}
		if event.NodeID == "" && event.FromState == string(RunStateRunning) && event.ToState == string(state) {
			if status.FinishedAt == nil || event.Reason != status.Error || !event.OccurredAt.Equal(*status.FinishedAt) {
				t.Errorf("terminal audit = %#v, status = %#v; want matching reason and finish time", event, status)
			}
			matching++
		}
	}
	if matching != 1 {
		t.Fatalf("terminal audit count = %d, want 1; events=%#v", matching, events)
	}
	if terminal != 1 {
		t.Fatalf("all run-terminal audit count = %d, want 1; events=%#v", terminal, events)
	}
}

func isTerminalRunState(state RunState) bool {
	return state == RunStateSucceeded || state == RunStateFailed || state == RunStateCancelled
}

func requireNodeTerminalAudit(
	t *testing.T,
	ss *SQLiteState,
	node NodeStatus,
	from NodeState,
	to NodeState,
) {
	t.Helper()
	events, err := Logs(context.Background(), ss.DB(), ss.RunID(), LogsOptions{NodeID: node.NodeID})
	if err != nil {
		t.Fatalf("Logs(%s): %v", node.NodeID, err)
	}
	matching := 0
	for _, event := range events {
		if event.FromState == string(from) && event.ToState == string(to) {
			if node.FinishedAt == nil || event.Reason != node.Error || !event.OccurredAt.Equal(*node.FinishedAt) {
				t.Errorf("node audit = %#v, node = %#v; want matching reason and finish time", event, node)
			}
			matching++
		}
	}
	if matching != 1 {
		t.Fatalf("node %q terminal audit count = %d, want 1; events=%#v", node.NodeID, matching, events)
	}
}
