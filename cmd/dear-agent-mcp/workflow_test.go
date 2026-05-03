package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	sqlitesource "github.com/vbonnet/dear-agent/pkg/source/sqlite"
	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// helper that opens a fresh DB and returns Server + cleanup. The Server
// gets a sources adapter wired against the same file so FetchSource /
// AddSource tools work in tests.
func newTestServer(t *testing.T) (*Server, *workflow.SQLiteState) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	ss, err := workflow.OpenSQLiteState(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	db, err := openDB(dbPath)
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	srcAdapter, err := sqlitesource.New(db)
	if err != nil {
		t.Fatalf("sqlitesource.New: %v", err)
	}

	return &Server{DB: db, DBPath: dbPath, Source: srcAdapter}, ss
}

func callTool(t *testing.T, srv *Server, name string, args map[string]any) rpcResponse {
	t.Helper()
	argBytes, _ := json.Marshal(args)
	params := toolCallParams{Name: name, Arguments: argBytes}
	pBytes, _ := json.Marshal(params)
	return srv.HandleRequest(context.Background(), rpcRequest{
		JSONRPC: "2.0", ID: 1, Method: "tools/call",
		Params: pBytes,
	})
}

func TestMCP_ToolsList_IncludesAllTools(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := srv.HandleRequest(context.Background(), rpcRequest{JSONRPC: "2.0", ID: 1, Method: "tools/list"})
	if resp.Error != nil {
		t.Fatalf("tools/list error: %+v", resp.Error)
	}
	res := resp.Result.(map[string]any)
	tools := res["tools"].([]map[string]any)
	if len(tools) != 7 {
		t.Errorf("got %d tools, want 7 (5 workflow + 2 source)", len(tools))
	}
	wantNames := map[string]bool{
		"workflow_run":     false,
		"workflow_status":  false,
		"workflow_approve": false,
		"workflow_reject":  false,
		"workflow_cancel":  false,
		"FetchSource":      false,
		"AddSource":        false,
	}
	for _, tool := range tools {
		wantNames[tool["name"].(string)] = true
	}
	for n, found := range wantNames {
		if !found {
			t.Errorf("missing tool %q", n)
		}
	}
}

func TestMCP_ToolsList_OmitsSourceToolsWhenAdapterDisabled(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.Source = nil
	resp := srv.HandleRequest(context.Background(), rpcRequest{JSONRPC: "2.0", ID: 1, Method: "tools/list"})
	if resp.Error != nil {
		t.Fatalf("tools/list error: %+v", resp.Error)
	}
	tools := resp.Result.(map[string]any)["tools"].([]map[string]any)
	for _, tool := range tools {
		name := tool["name"].(string)
		if name == "FetchSource" || name == "AddSource" {
			t.Errorf("source tool %q surfaced when adapter is nil", name)
		}
	}
}

func TestMCP_WorkflowApprove_RoundTrip(t *testing.T) {
	srv, ss := newTestServer(t)
	ctx := context.Background()

	if err := ss.BeginRun(ctx, workflow.RunRecord{
		RunID: "r1", WorkflowName: "wf", State: workflow.RunStateRunning, InputsJSON: "{}", StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("BeginRun: %v", err)
	}
	if err := ss.UpsertNode(ctx, workflow.NodeRecord{RunID: "r1", NodeID: "n1", State: workflow.NodeStateRunning}); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	approvalID, err := workflow.CreateHITLRequest(ctx, ss.DB(), "r1", "n1", "reviewer", "needs human", time.Now())
	if err != nil {
		t.Fatalf("CreateHITLRequest: %v", err)
	}

	resp := callTool(t, srv, "workflow_approve", map[string]any{
		"approval_id": approvalID,
		"approver":    "alice",
		"role":        "reviewer",
		"reason":      "lgtm",
	})
	if resp.Error != nil {
		t.Fatalf("workflow_approve error: %+v", resp.Error)
	}
	res := resp.Result.(map[string]any)
	if res["decision"] != "approve" {
		t.Errorf("decision=%v", res["decision"])
	}

	// Calling again should error with -32002 (already resolved).
	resp = callTool(t, srv, "workflow_approve", map[string]any{
		"approval_id": approvalID, "approver": "alice", "role": "reviewer",
	})
	if resp.Error == nil {
		t.Fatalf("expected error on second approve")
	}
	if resp.Error.Code != -32002 {
		t.Errorf("err code=%d, want -32002", resp.Error.Code)
	}
}

func TestMCP_WorkflowStatus_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := callTool(t, srv, "workflow_status", map[string]any{"run_id": "missing-run"})
	if resp.Error == nil {
		t.Fatal("expected error for missing run")
	}
	if resp.Error.Code != -32001 {
		t.Errorf("err code=%d, want -32001", resp.Error.Code)
	}
}

func TestMCP_WorkflowCancel_RoundTrip(t *testing.T) {
	srv, ss := newTestServer(t)
	ctx := context.Background()

	if err := ss.BeginRun(ctx, workflow.RunRecord{
		RunID: "r2", WorkflowName: "wf", State: workflow.RunStateRunning, InputsJSON: "{}", StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("BeginRun: %v", err)
	}
	resp := callTool(t, srv, "workflow_cancel", map[string]any{"run_id": "r2", "reason": "stop"})
	if resp.Error != nil {
		t.Fatalf("cancel error: %+v", resp.Error)
	}
	res := resp.Result.(map[string]any)
	if res["cancelled"] != "r2" {
		t.Errorf("cancelled=%v", res["cancelled"])
	}
}

func TestMCP_WorkflowRun_QueuesPlaceholder(t *testing.T) {
	srv, _ := newTestServer(t)
	tmp := filepath.Join(t.TempDir(), "wf.yaml")
	if err := writeStringFile(tmp, "name: x\nversion: '1'\nnodes: []\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	resp := callTool(t, srv, "workflow_run", map[string]any{
		"file":   tmp,
		"inputs": map[string]any{"k": "v"},
	})
	if resp.Error != nil {
		t.Fatalf("workflow_run error: %+v", resp.Error)
	}
	res := resp.Result.(map[string]any)
	runID, ok := res["run_id"].(string)
	if !ok || runID == "" {
		t.Errorf("missing run_id: %v", res)
	}
	// Confirm the placeholder run is in the DB and queryable via status.
	resp = callTool(t, srv, "workflow_status", map[string]any{"run_id": runID})
	if resp.Error != nil {
		t.Fatalf("status error: %+v", resp.Error)
	}
}

func writeStringFile(path, contents string) error {
	return os.WriteFile(path, []byte(contents), 0o644)
}
