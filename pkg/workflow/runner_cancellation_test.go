package workflow

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
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
	}
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

	status := requireCancellationStatus(t, ss, "context canceled")
	first := requireNodeStatus(t, status, "first")
	if first.State != NodeStateFailed || first.FinishedAt == nil || !strings.Contains(first.Error, "context canceled") {
		t.Errorf("first node = %#v, want finished failed context error", first)
	}
	second := requireNodeStatus(t, status, "second")
	if second.State != NodeStateSkipped || second.FinishedAt == nil || second.Error != "context-cancelled" {
		t.Errorf("second node = %#v, want finished skipped/context-cancelled", second)
	}
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
	requireRunTerminalAudit(t, ss, RunStateCancelled)
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
	events, err := Logs(context.Background(), ss.DB(), ss.RunID(), LogsOptions{})
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	matching := 0
	for _, event := range events {
		if event.NodeID == "" && event.FromState == string(RunStateRunning) && event.ToState == string(state) {
			matching++
		}
	}
	if matching != 1 {
		t.Fatalf("terminal audit count = %d, want 1; events=%#v", matching, events)
	}
}
