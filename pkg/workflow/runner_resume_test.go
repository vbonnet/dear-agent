package workflow

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type cancelAfterLoadState struct {
	State
	cancel context.CancelFunc
}

func (s cancelAfterLoadState) Load(ctx context.Context) (*Snapshot, error) {
	snapshot, err := s.State.Load(ctx)
	s.cancel()
	return snapshot, err
}

type staticResumeState struct {
	snapshot *Snapshot
}

func (s staticResumeState) Load(context.Context) (*Snapshot, error) {
	return s.snapshot, nil
}

func (staticResumeState) Save(context.Context, Snapshot) error {
	return nil
}

func TestRunnerFailedResumeInitializationStopsExecution(t *testing.T) {
	resumeErr := errors.New("resume recorder unavailable")
	var finishCalls atomic.Int32
	recorder := &terminalRecorderProbe{
		begin: func(context.Context, RunRecord) error { return resumeErr },
		finish: func(context.Context) error {
			finishCalls.Add(1)
			return nil
		},
	}
	ai := &cancellationTestAI{}
	runner := NewRunner(ai)
	runner.Recorder = recorder
	state := staticResumeState{snapshot: &Snapshot{
		Workflow:  "failed-resume-initialization",
		RunID:     "failed-resume-run",
		Inputs:    map[string]string{},
		Outputs:   map[string]string{},
		Completed: map[string]bool{},
		Started:   time.Now().Add(-time.Minute),
	}}

	report, runErr := runner.Resume(context.Background(), cancellationWorkflow("failed-resume-initialization"), state)
	if !errors.Is(runErr, resumeErr) || !strings.Contains(runErr.Error(), "begin resumed run record") {
		t.Fatalf("Resume error = %v, want surfaced initialization failure", runErr)
	}
	if report == nil || report.Finished.IsZero() || len(report.Results) != 0 {
		t.Fatalf("report = %#v, want finished report without execution", report)
	}
	if got := ai.calls.Load(); got != 0 {
		t.Fatalf("executor calls = %d, want 0", got)
	}
	if got := finishCalls.Load(); got != 0 {
		t.Fatalf("FinishRun calls = %d, want no finalization after failed resume initialization", got)
	}
}

func TestRunnerCancelledResumeInitializationPreservesTerminalRun(t *testing.T) {
	ss, runID := seedTerminalResumeRun(t, "cancel-resume-initialization")
	ctx, cancel := context.WithCancel(context.Background())
	ai := &cancellationTestAI{}
	runner := NewRunner(ai)
	runner.UseSQLiteState(ss)

	report, runErr := runner.Resume(ctx, cancellationWorkflow("cancel-resume-initialization"), cancelAfterLoadState{
		State:  ss,
		cancel: cancel,
	})
	if !errors.Is(runErr, context.Canceled) || !strings.Contains(runErr.Error(), "reopen run record") {
		t.Fatalf("Resume error = %v, want surfaced cancelled reopen error", runErr)
	}
	if report == nil || report.Finished.IsZero() || len(report.Results) != 0 {
		t.Fatalf("report = %#v, want finished report without execution", report)
	}
	if got := ai.calls.Load(); got != 0 {
		t.Fatalf("executor calls = %d, want 0", got)
	}

	status, err := Status(context.Background(), ss.DB(), runID)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.State != RunStateSucceeded || status.FinishedAt == nil || status.Error != "" {
		t.Fatalf("status = %#v, want original succeeded terminal state", status)
	}
	assertRunTransitions(t, ss, runID, map[string]int{
		"pending->running":   1,
		"running->succeeded": 1,
	})
}

func TestRunnerResumeAuditFailureRollsBackReopen(t *testing.T) {
	ss, runID := seedTerminalResumeRun(t, "resume-audit-rollback")
	if _, err := ss.DB().Exec(`
		CREATE TRIGGER fail_resumed_run_audit
		BEFORE INSERT ON audit_events
		WHEN NEW.reason = 'run-resumed'
		BEGIN
			SELECT RAISE(ABORT, 'resume audit denied');
		END
	`); err != nil {
		t.Fatalf("create audit failure trigger: %v", err)
	}
	ai := &cancellationTestAI{}
	runner := NewRunner(ai)
	runner.UseSQLiteState(ss)

	report, runErr := runner.Resume(context.Background(), cancellationWorkflow("resume-audit-rollback"), ss)
	if runErr == nil || !strings.Contains(runErr.Error(), "audit transition") ||
		!strings.Contains(runErr.Error(), "resume audit denied") {
		t.Fatalf("Resume error = %v, want surfaced atomic audit failure", runErr)
	}
	if report == nil || report.Finished.IsZero() || len(report.Results) != 0 {
		t.Fatalf("report = %#v, want finished report without execution", report)
	}
	if got := ai.calls.Load(); got != 0 {
		t.Fatalf("executor calls = %d, want 0", got)
	}

	status, err := Status(context.Background(), ss.DB(), runID)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.State != RunStateSucceeded || status.FinishedAt == nil || status.Error != "" {
		t.Fatalf("status = %#v, want original succeeded terminal state after rollback", status)
	}
	assertRunTransitions(t, ss, runID, map[string]int{
		"pending->running":   1,
		"running->succeeded": 1,
	})
}

func TestRunnerResumePersistsRequiredAuditBeforeObservers(t *testing.T) {
	ss, runID := seedTerminalResumeRun(t, "resume-audit-order")
	ctx, cancel := context.WithCancel(context.Background())
	var observerErr error
	resumeAuditVisible := false
	ai := &cancellationTestAI{}
	runner := NewRunner(ai)
	runner.UseSQLiteState(ss)
	runner.Audit = &MultiAuditSink{Sinks: []AuditSink{
		&terminalAuditProbe{emit: func(_ context.Context, event AuditEvent) error {
			if event.Reason == "run-resumed" {
				var count int
				observerErr = ss.DB().QueryRow(`
					SELECT COUNT(*) FROM audit_events
					WHERE run_id = ? AND from_state = 'succeeded'
					  AND to_state = 'running' AND reason = 'run-resumed'
				`, runID).Scan(&count)
				resumeAuditVisible = count == 1
				cancel()
			}
			return nil
		}},
		ss,
	}}

	_, runErr := runner.Resume(ctx, cancellationWorkflow("resume-audit-order"), ss)
	if !errors.Is(runErr, context.Canceled) {
		t.Fatalf("Resume error = %v, want context.Canceled", runErr)
	}
	if got := ai.calls.Load(); got != 0 {
		t.Fatalf("executor calls = %d, want 0", got)
	}
	if observerErr != nil || !resumeAuditVisible {
		t.Fatalf("resume audit visible inside observer = %t, query error = %v; want committed first", resumeAuditVisible, observerErr)
	}
	assertRunTransitions(t, ss, runID, map[string]int{
		"pending->running":   1,
		"running->succeeded": 1,
		"succeeded->running": 1,
		"running->cancelled": 1,
	})
}

func TestRunnerGenericResumePreservesBeginRunWithoutSyntheticTransition(t *testing.T) {
	var beginCalls atomic.Int32
	var finishCalls atomic.Int32
	var events []AuditEvent
	recorder := &terminalRecorderProbe{
		begin: func(_ context.Context, record RunRecord) error {
			beginCalls.Add(1)
			if record.RunID != "generic-resume-run" {
				t.Fatalf("BeginRun RunID = %q, want snapshot identity", record.RunID)
			}
			return nil
		},
		finish: func(context.Context) error {
			finishCalls.Add(1)
			return nil
		},
	}
	ai := &cancellationTestAI{}
	runner := NewRunner(ai)
	runner.Recorder = recorder
	runner.Audit = &terminalAuditProbe{emit: func(_ context.Context, event AuditEvent) error {
		events = append(events, event)
		return nil
	}}
	state := staticResumeState{snapshot: &Snapshot{
		Workflow: "generic-resume", RunID: "generic-resume-run",
		Inputs:  map[string]string{},
		Outputs: map[string]string{"first": "one", "second": "two"},
		Completed: map[string]bool{
			"first":  true,
			"second": true,
		},
		Started: time.Now().Add(-time.Minute),
	}}

	report, runErr := runner.Resume(context.Background(), cancellationWorkflow("generic-resume"), state)
	if runErr != nil {
		t.Fatalf("Resume: %v", runErr)
	}
	if report == nil || report.Finished.IsZero() || len(report.Results) != 0 {
		t.Fatalf("report = %#v, want completed resume without re-execution", report)
	}
	if got := ai.calls.Load(); got != 0 {
		t.Fatalf("executor calls = %d, want 0", got)
	}
	if got := beginCalls.Load(); got != 1 {
		t.Fatalf("BeginRun calls = %d, want preserved lifecycle callback", got)
	}
	if got := finishCalls.Load(); got != 1 {
		t.Fatalf("FinishRun calls = %d, want 1", got)
	}
	if len(events) != 1 {
		t.Fatalf("generic recorder audit events = %#v, want only terminal transition", events)
	}
	event := events[0]
	if event.NodeID != "" || event.FromState != string(RunStateRunning) ||
		event.ToState != string(RunStateSucceeded) || event.Reason != "" {
		t.Fatalf("generic recorder audit event = %#v, want only running-to-succeeded terminal transition", event)
	}
}

func seedTerminalResumeRun(t *testing.T, workflowName string) (*SQLiteState, string) {
	t.Helper()
	ss := openTestState(t)
	const runID = "terminal-resume-run"
	started := time.Now().Add(-time.Minute)
	finished := started.Add(time.Second)
	if err := ss.BeginRun(context.Background(), RunRecord{
		RunID:        runID,
		WorkflowName: workflowName,
		State:        RunStateRunning,
		InputsJSON:   "{}",
		StartedAt:    started,
	}); err != nil {
		t.Fatalf("BeginRun: %v", err)
	}
	for _, event := range []AuditEvent{
		{
			RunID:      runID,
			FromState:  string(RunStatePending),
			ToState:    string(RunStateRunning),
			Reason:     "run-started",
			Actor:      "system",
			OccurredAt: started,
		},
		{
			RunID:      runID,
			FromState:  string(RunStateRunning),
			ToState:    string(RunStateSucceeded),
			Actor:      "system",
			OccurredAt: finished,
		},
	} {
		if err := ss.Emit(context.Background(), event); err != nil {
			t.Fatalf("Emit: %v", err)
		}
	}
	if err := ss.UpsertNode(context.Background(), NodeRecord{
		RunID:      runID,
		NodeID:     "first",
		State:      NodeStateSucceeded,
		Attempts:   1,
		Output:     "already-done",
		StartedAt:  started,
		FinishedAt: finished,
	}); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	if err := ss.FinishRun(context.Background(), runID, RunStateSucceeded, finished, ""); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}
	return ss, runID
}

func assertRunTransitions(t *testing.T, ss *SQLiteState, runID string, want map[string]int) {
	t.Helper()
	events, err := Logs(context.Background(), ss.DB(), runID, LogsOptions{})
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	got := map[string]int{}
	for _, event := range events {
		if event.NodeID == "" {
			got[event.FromState+"->"+event.ToState]++
		}
	}
	if len(got) != len(want) {
		t.Fatalf("run transitions = %v, want %v", got, want)
	}
	for transition, count := range want {
		if got[transition] != count {
			t.Fatalf("run transition %s count = %d, want %d; all=%v", transition, got[transition], count, got)
		}
	}
}
