package workflow

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type defineFailureExecutor struct {
	calls atomic.Int32
}

func (e *defineFailureExecutor) Generate(context.Context, *AINode, map[string]string, map[string]string) (string, error) {
	e.calls.Add(1)
	return "unexpected", nil
}

type defineFailureFinish struct {
	runID string
	state RunState
	err   string
}

type defineFailureRecorder struct {
	begin        []RunRecord
	finish       []defineFailureFinish
	nodeCalls    int
	attemptCalls int
}

func (r *defineFailureRecorder) BeginRun(_ context.Context, rec RunRecord) error {
	r.begin = append(r.begin, rec)
	return nil
}

func (r *defineFailureRecorder) UpsertNode(context.Context, NodeRecord) error {
	r.nodeCalls++
	return nil
}

func (r *defineFailureRecorder) RecordAttempt(context.Context, AttemptRecord) error {
	r.attemptCalls++
	return nil
}

func (r *defineFailureRecorder) FinishRun(_ context.Context, runID string, state RunState, _ time.Time, errMsg string) error {
	r.finish = append(r.finish, defineFailureFinish{runID: runID, state: state, err: errMsg})
	return nil
}

type defineFailureState struct {
	snapshot  *Snapshot
	loadCalls int
	saveCalls int
}

func (s *defineFailureState) Load(context.Context) (*Snapshot, error) {
	s.loadCalls++
	return s.snapshot, nil
}

func (s *defineFailureState) Save(context.Context, Snapshot) error {
	s.saveCalls++
	return nil
}

func TestRunnerDefineFailureStopsRunAndResume(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		resume bool
	}{
		{name: "run"},
		{name: "resume", resume: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			executor := &defineFailureExecutor{}
			recorder := &defineFailureRecorder{}
			state := &defineFailureState{snapshot: &Snapshot{
				Workflow:  "define-failure",
				RunID:     "resumed-run-id",
				Inputs:    map[string]string{"source": "snapshot"},
				Outputs:   map[string]string{},
				Completed: map[string]bool{},
				Started:   time.Unix(1, 0),
			}}
			var defineCalls atomic.Int32
			var enforceCalls atomic.Int32
			var resolveCalls atomic.Int32
			runner := NewRunner(executor)
			runner.Recorder = recorder
			runner.Hooks = &Hooks{
				OnDefine: func(context.Context, DefinePayload) error {
					defineCalls.Add(1)
					return errors.New("policy rejected definition")
				},
				OnEnforce: func(context.Context, EnforcePayload) error {
					enforceCalls.Add(1)
					return nil
				},
				OnResolve: func(context.Context, ResolvePayload) error {
					resolveCalls.Add(1)
					return nil
				},
			}
			workflow := &Workflow{
				Name:    "define-failure",
				Version: "1",
				Nodes: []Node{{
					ID: "must-not-run", Kind: KindAI, AI: &AINode{Prompt: "must not execute"},
				}},
			}

			var (
				report *RunReport
				err    error
			)
			if tc.resume {
				report, err = runner.Resume(context.Background(), workflow, state)
			} else {
				report, err = runner.Run(context.Background(), workflow, map[string]string{"source": "run"})
			}

			if err == nil || !strings.Contains(err.Error(), "hook OnDefine: policy rejected definition") {
				t.Fatalf("run error = %v, want contextual OnDefine error", err)
			}
			if report == nil || report.Finished.IsZero() || report.Succeeded || len(report.Results) != 0 {
				t.Fatalf("report = %#v, want finished unsuccessful report with no results", report)
			}
			if got := executor.calls.Load(); got != 0 {
				t.Fatalf("executor calls = %d, want 0", got)
			}
			if got := defineCalls.Load(); got != 1 {
				t.Fatalf("OnDefine calls = %d, want 1", got)
			}
			if got := enforceCalls.Load(); got != 0 {
				t.Fatalf("OnEnforce calls = %d, want 0", got)
			}
			if got := resolveCalls.Load(); got != 0 {
				t.Fatalf("OnResolve calls = %d, want 0", got)
			}
			if len(recorder.begin) != 1 || len(recorder.finish) != 1 {
				t.Fatalf("run records: begin=%d finish=%d, want 1 each", len(recorder.begin), len(recorder.finish))
			}
			finished := recorder.finish[0]
			if finished.runID != recorder.begin[0].RunID || finished.state != RunStateFailed || finished.err != err.Error() {
				t.Fatalf("terminal record = %#v, begin=%#v, err=%v", finished, recorder.begin[0], err)
			}
			if recorder.nodeCalls != 0 || recorder.attemptCalls != 0 {
				t.Fatalf("node recording escaped definition gate: nodes=%d attempts=%d", recorder.nodeCalls, recorder.attemptCalls)
			}
			wantLoads := 0
			if tc.resume {
				wantLoads = 1
			}
			if state.loadCalls != wantLoads || state.saveCalls != 0 {
				t.Fatalf("state calls: load=%d save=%d, want load=%d save=0", state.loadCalls, state.saveCalls, wantLoads)
			}
		})
	}
}

func TestRunnerDefineFailurePersistsAfterHookCancelsContext(t *testing.T) {
	ss := openTestState(t)
	ctx, cancel := context.WithCancel(context.Background())
	runner := NewRunner(&defineFailureExecutor{})
	runner.UseSQLiteState(ss)
	runner.Hooks = &Hooks{
		OnDefine: func(context.Context, DefinePayload) error {
			cancel()
			return errors.New("policy rejected after cancellation")
		},
	}
	workflow := &Workflow{
		Name:    "cancelled-define-failure",
		Version: "1",
		Nodes: []Node{{
			ID: "must-not-run", Kind: KindAI, AI: &AINode{Prompt: "must not execute"},
		}},
	}

	report, runErr := runner.Run(ctx, workflow, nil)
	if runErr == nil || !strings.Contains(runErr.Error(), "hook OnDefine: policy rejected after cancellation") {
		t.Fatalf("run error = %v, want contextual OnDefine error", runErr)
	}
	if report == nil || report.Finished.IsZero() {
		t.Fatalf("report = %#v, want finished report", report)
	}

	status, err := Status(context.Background(), ss.DB(), ss.RunID())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.State != RunStateFailed || status.FinishedAt == nil || status.Error != runErr.Error() {
		t.Fatalf("status = %#v, want terminal failed state with error %q", status, runErr)
	}
	events, err := Logs(context.Background(), ss.DB(), ss.RunID(), LogsOptions{})
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	if len(events) != 2 || events[1].FromState != string(RunStateRunning) || events[1].ToState != string(RunStateFailed) {
		t.Fatalf("audit events = %#v, want running then failed", events)
	}
}
