package gateway_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/gateway"
	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// withDB opens a fresh SQLite runs.db under t.TempDir() and returns
// the *workflow.SQLiteState. We deliberately do not seed any rows —
// each test that needs them inserts them via state.BeginRun /
// workflow.CreateHITLRequest.
func withDB(t *testing.T) *workflow.SQLiteState {
	t.Helper()
	path := filepath.Join(t.TempDir(), "runs.db")
	state, err := workflow.OpenSQLiteState(path)
	if err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	t.Cleanup(func() { _ = state.Close() })
	return state
}

// stubRunner records the command it received and returns canned data.
type stubRunner struct {
	lastReq    gateway.RunRequest
	lastCaller gateway.Caller
	resp       gateway.RunResponse
	err        error
}

func (s *stubRunner) Run(_ context.Context, req gateway.RunRequest, caller gateway.Caller) (gateway.RunResponse, error) {
	s.lastReq = req
	s.lastCaller = caller
	if s.err != nil {
		return gateway.RunResponse{}, s.err
	}
	return s.resp, nil
}

func TestRunHandler_HappyPath(t *testing.T) {
	state := withDB(t)
	stub := &stubRunner{resp: gateway.RunResponse{RunID: "r1", PID: 4242}}
	gw := gateway.New(gateway.WorkflowHandlers(state.DB(), stub))

	resp := gw.Dispatch(context.Background(), gateway.Command{
		Type:   gateway.CmdRun,
		Caller: gateway.Caller{LoginName: "alice"},
		Args:   map[string]any{"file": "wf.yaml", "inputs": map[string]any{"k": "v"}},
	})
	if resp.Err != nil {
		t.Fatalf("err: %v", resp.Err)
	}
	if stub.lastReq.File != "wf.yaml" {
		t.Errorf("file: got %q", stub.lastReq.File)
	}
	if stub.lastReq.Inputs["k"] != "v" {
		t.Errorf("inputs: got %+v", stub.lastReq.Inputs)
	}
	if stub.lastCaller.LoginName != "alice" {
		t.Errorf("caller: got %q", stub.lastCaller.LoginName)
	}
	if got, _ := resp.Body["run_id"].(string); got != "r1" {
		t.Errorf("run_id: got %q", got)
	}
}

func TestRunHandler_MissingFile(t *testing.T) {
	state := withDB(t)
	gw := gateway.New(gateway.WorkflowHandlers(state.DB(), &stubRunner{}))
	resp := gw.Dispatch(context.Background(), gateway.Command{Type: gateway.CmdRun})
	if resp.Err == nil || resp.Err.Code != gateway.CodeInvalidArgs {
		t.Fatalf("want CodeInvalidArgs, got %+v", resp.Err)
	}
}

func TestRunHandler_NoRunner(t *testing.T) {
	state := withDB(t)
	gw := gateway.New(gateway.WorkflowHandlers(state.DB(), nil))
	resp := gw.Dispatch(context.Background(), gateway.Command{
		Type: gateway.CmdRun,
		Args: map[string]any{"file": "wf.yaml"},
	})
	if resp.Err == nil || resp.Err.Code != gateway.CodeUnavailable {
		t.Fatalf("want CodeUnavailable, got %+v", resp.Err)
	}
}

func TestRunHandler_RunnerErrorBecomesInternal(t *testing.T) {
	state := withDB(t)
	stub := &stubRunner{err: errors.New("spawn failed")}
	gw := gateway.New(gateway.WorkflowHandlers(state.DB(), stub))
	resp := gw.Dispatch(context.Background(), gateway.Command{
		Type: gateway.CmdRun,
		Args: map[string]any{"file": "wf.yaml"},
	})
	if resp.Err == nil || resp.Err.Code != gateway.CodeInternal {
		t.Fatalf("want CodeInternal, got %+v", resp.Err)
	}
	if !errors.Is(resp.Err, stub.err) {
		t.Errorf("Unwrap should reach the runner error")
	}
}

func TestRunHandler_TypedInputsAlsoAccepted(t *testing.T) {
	// The CLI adapter unmarshals JSON into map[string]any; HTTP and
	// in-process callers may pass map[string]string directly. Both
	// must work.
	state := withDB(t)
	stub := &stubRunner{}
	gw := gateway.New(gateway.WorkflowHandlers(state.DB(), stub))
	resp := gw.Dispatch(context.Background(), gateway.Command{
		Type: gateway.CmdRun,
		Args: map[string]any{
			"file":   "wf.yaml",
			"inputs": map[string]string{"k": "v"},
		},
	})
	if resp.Err != nil {
		t.Fatalf("err: %v", resp.Err)
	}
	if stub.lastReq.Inputs["k"] != "v" {
		t.Errorf("inputs: got %+v", stub.lastReq.Inputs)
	}
}

func TestRunHandler_NonStringInputValue(t *testing.T) {
	state := withDB(t)
	gw := gateway.New(gateway.WorkflowHandlers(state.DB(), &stubRunner{}))
	resp := gw.Dispatch(context.Background(), gateway.Command{
		Type: gateway.CmdRun,
		Args: map[string]any{
			"file":   "wf.yaml",
			"inputs": map[string]any{"k": 42}, // not a string
		},
	})
	if resp.Err == nil || resp.Err.Code != gateway.CodeInvalidArgs {
		t.Fatalf("want CodeInvalidArgs, got %+v", resp.Err)
	}
}

func TestStatusHandler_NotFound(t *testing.T) {
	state := withDB(t)
	gw := gateway.New(gateway.WorkflowHandlers(state.DB(), nil))
	resp := gw.Dispatch(context.Background(), gateway.Command{
		Type: gateway.CmdStatus,
		Args: map[string]any{"run_id": "ghost"},
	})
	if resp.Err == nil || resp.Err.Code != gateway.CodeNotFound {
		t.Fatalf("want CodeNotFound, got %+v", resp.Err)
	}
}

func TestStatusHandler_HappyPath(t *testing.T) {
	state := withDB(t)
	if err := state.BeginRun(context.Background(), workflow.RunRecord{
		RunID:        "r1",
		WorkflowName: "wf",
		State:        workflow.RunStateRunning,
		InputsJSON:   "{}",
		StartedAt:    time.Now(),
	}); err != nil {
		t.Fatalf("BeginRun: %v", err)
	}

	gw := gateway.New(gateway.WorkflowHandlers(state.DB(), nil))
	resp := gw.Dispatch(context.Background(), gateway.Command{
		Type: gateway.CmdStatus,
		Args: map[string]any{"run_id": "r1"},
	})
	if resp.Err != nil {
		t.Fatalf("err: %v", resp.Err)
	}
	st, ok := resp.Body["run"].(*workflow.RunStatus)
	if !ok {
		t.Fatalf("body[run]: got %T (%+v)", resp.Body["run"], resp.Body["run"])
	}
	if st.RunID != "r1" {
		t.Errorf("run_id: got %q", st.RunID)
	}
}

func TestStatusHandler_MissingRunID(t *testing.T) {
	state := withDB(t)
	gw := gateway.New(gateway.WorkflowHandlers(state.DB(), nil))
	resp := gw.Dispatch(context.Background(), gateway.Command{Type: gateway.CmdStatus})
	if resp.Err == nil || resp.Err.Code != gateway.CodeInvalidArgs {
		t.Fatalf("want CodeInvalidArgs, got %+v", resp.Err)
	}
}

func TestListHandler_EmptyDBReturnsEmptySlice(t *testing.T) {
	state := withDB(t)
	gw := gateway.New(gateway.WorkflowHandlers(state.DB(), nil))
	resp := gw.Dispatch(context.Background(), gateway.Command{Type: gateway.CmdList})
	if resp.Err != nil {
		t.Fatalf("err: %v", resp.Err)
	}
	runs, ok := resp.Body["runs"].([]workflow.RunSummary)
	if !ok {
		t.Fatalf("body[runs]: got %T", resp.Body["runs"])
	}
	if len(runs) != 0 {
		t.Errorf("len(runs) = %d; want 0", len(runs))
	}
}

func TestListHandler_FloatLimitFromJSON(t *testing.T) {
	// JSON decoders produce float64 for numbers. The handler must accept
	// that and not silently drop the limit.
	state := withDB(t)
	gw := gateway.New(gateway.WorkflowHandlers(state.DB(), nil))
	resp := gw.Dispatch(context.Background(), gateway.Command{
		Type: gateway.CmdList,
		Args: map[string]any{"limit": float64(7)},
	})
	if resp.Err != nil {
		t.Fatalf("err: %v", resp.Err)
	}
}

func TestGatesHandler_EmptyDBReturnsEmptySlice(t *testing.T) {
	state := withDB(t)
	gw := gateway.New(gateway.WorkflowHandlers(state.DB(), nil))
	resp := gw.Dispatch(context.Background(), gateway.Command{Type: gateway.CmdGates})
	if resp.Err != nil {
		t.Fatalf("err: %v", resp.Err)
	}
	gates, ok := resp.Body["gates"].([]workflow.HITLRequest)
	if !ok {
		t.Fatalf("body[gates]: got %T", resp.Body["gates"])
	}
	if len(gates) != 0 {
		t.Errorf("len(gates) = %d; want 0", len(gates))
	}
}

func TestApproveHandler_RequiresCaller(t *testing.T) {
	state := withDB(t)
	gw := gateway.New(gateway.WorkflowHandlers(state.DB(), nil))
	resp := gw.Dispatch(context.Background(), gateway.Command{
		Type: gateway.CmdApprove,
		Args: map[string]any{"approval_id": "a1"},
	})
	if resp.Err == nil || resp.Err.Code != gateway.CodeUnauthorized {
		t.Fatalf("want CodeUnauthorized, got %+v", resp.Err)
	}
}

func TestApproveHandler_RequiresApprovalID(t *testing.T) {
	state := withDB(t)
	gw := gateway.New(gateway.WorkflowHandlers(state.DB(), nil))
	resp := gw.Dispatch(context.Background(), gateway.Command{
		Type:   gateway.CmdApprove,
		Caller: gateway.Caller{LoginName: "alice"},
	})
	if resp.Err == nil || resp.Err.Code != gateway.CodeInvalidArgs {
		t.Fatalf("want CodeInvalidArgs, got %+v", resp.Err)
	}
}

func TestApproveHandler_NotFound(t *testing.T) {
	state := withDB(t)
	gw := gateway.New(gateway.WorkflowHandlers(state.DB(), nil))
	resp := gw.Dispatch(context.Background(), gateway.Command{
		Type:   gateway.CmdApprove,
		Caller: gateway.Caller{LoginName: "alice"},
		Args:   map[string]any{"approval_id": "ghost"},
	})
	if resp.Err == nil || resp.Err.Code != gateway.CodeNotFound {
		t.Fatalf("want CodeNotFound, got %+v", resp.Err)
	}
}

func TestApproveHandler_HappyPath(t *testing.T) {
	state := withDB(t)
	ctx := context.Background()
	if err := state.BeginRun(ctx, workflow.RunRecord{
		RunID:        "r1",
		WorkflowName: "wf",
		State:        workflow.RunStateRunning,
		InputsJSON:   "{}",
		StartedAt:    time.Now(),
	}); err != nil {
		t.Fatalf("BeginRun: %v", err)
	}
	if err := state.UpsertNode(ctx, workflow.NodeRecord{RunID: "r1", NodeID: "n1", State: workflow.NodeStateRunning}); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	approvalID, err := workflow.CreateHITLRequest(ctx, state.DB(), "r1", "n1", "", "review", time.Now())
	if err != nil {
		t.Fatalf("CreateHITLRequest: %v", err)
	}

	gw := gateway.New(gateway.WorkflowHandlers(state.DB(), nil))
	resp := gw.Dispatch(ctx, gateway.Command{
		Type:   gateway.CmdApprove,
		Caller: gateway.Caller{LoginName: "alice"},
		Args:   map[string]any{"approval_id": approvalID, "reason": "lgtm"},
	})
	if resp.Err != nil {
		t.Fatalf("err: %v", resp.Err)
	}
	if got, _ := resp.Body["decision"].(string); got != string(workflow.HITLDecisionApprove) {
		t.Errorf("decision: got %q", got)
	}
	if got, _ := resp.Body["actor"].(string); got != "alice" {
		t.Errorf("actor: got %q", got)
	}

	// Second approve must conflict.
	resp = gw.Dispatch(ctx, gateway.Command{
		Type:   gateway.CmdApprove,
		Caller: gateway.Caller{LoginName: "alice"},
		Args:   map[string]any{"approval_id": approvalID},
	})
	if resp.Err == nil || resp.Err.Code != gateway.CodeConflict {
		t.Fatalf("want CodeConflict on second approve, got %+v", resp.Err)
	}
}

func TestCancelHandler_PlaceholderUnavailable(t *testing.T) {
	state := withDB(t)
	gw := gateway.New(gateway.WorkflowHandlers(state.DB(), nil))
	resp := gw.Dispatch(context.Background(), gateway.Command{
		Type: gateway.CmdCancel,
		Args: map[string]any{"run_id": "r1"},
	})
	if resp.Err == nil || resp.Err.Code != gateway.CodeUnavailable {
		t.Fatalf("want CodeUnavailable (placeholder), got %+v", resp.Err)
	}
}
