package api_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/api"
	"github.com/vbonnet/dear-agent/pkg/audit"
	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// fixture wires a *Server with an in-memory audit store and a fresh
// SQLite runs.db. Caller is "alice".
type fixture struct {
	srv      *api.Server
	ts       *httptest.Server
	runsDB   *sql.DB
	state    *workflow.SQLiteState
	auditMem *audit.MemoryStore
	stubRun  *stubRunner
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	state, err := workflow.OpenSQLiteState(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	t.Cleanup(func() { _ = state.Close() })

	auditMem := audit.NewMemoryStore()
	stub := &stubRunner{}
	srv := api.New(api.Server{
		RunsDB:     state.DB(),
		AuditStore: auditMem,
		Identifier: api.AnonymousIdentifier("alice"),
		Runner:     stub,
		Version:    "test",
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return &fixture{
		srv:      srv,
		ts:       ts,
		runsDB:   state.DB(),
		state:    state,
		auditMem: auditMem,
		stubRun:  stub,
	}
}

// stubRunner records the last request and returns a canned response.
type stubRunner struct {
	lastReq    api.RunRequest
	lastCaller api.Caller
	resp       api.RunResponse
	err        error
}

func (s *stubRunner) Run(_ context.Context, req api.RunRequest, c api.Caller) (api.RunResponse, error) {
	s.lastReq = req
	s.lastCaller = c
	if s.err != nil {
		return api.RunResponse{}, s.err
	}
	return s.resp, nil
}

func TestStatus(t *testing.T) {
	f := newFixture(t)
	resp, err := http.Get(f.ts.URL + "/status")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}
	var body struct {
		OK      bool       `json:"ok"`
		Version string     `json:"version"`
		Caller  api.Caller `json:"caller"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.OK {
		t.Errorf("ok = false, want true")
	}
	if body.Version != "test" {
		t.Errorf("version = %q, want test", body.Version)
	}
	if body.Caller.LoginName != "alice" {
		t.Errorf("caller = %q, want alice", body.Caller.LoginName)
	}
}

func TestListWorkflows_Empty(t *testing.T) {
	f := newFixture(t)
	resp, err := http.Get(f.ts.URL + "/workflows")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Runs []workflow.RunSummary `json:"runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Runs == nil {
		t.Fatalf("runs is nil; want []")
	}
	if len(body.Runs) != 0 {
		t.Fatalf("runs = %d, want 0", len(body.Runs))
	}
}

func TestGetWorkflow_NotFound(t *testing.T) {
	f := newFixture(t)
	resp, err := http.Get(f.ts.URL + "/workflows/does-not-exist")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["code"] != "run_not_found" {
		t.Errorf("code = %q, want run_not_found", body["code"])
	}
}

func TestListGates_Empty(t *testing.T) {
	f := newFixture(t)
	resp, err := http.Get(f.ts.URL + "/gates")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Gates []workflow.HITLRequest `json:"gates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Gates == nil || len(body.Gates) != 0 {
		t.Fatalf("gates = %v, want []", body.Gates)
	}
}

// TestApproveGate_HappyPath drives a real HITL workflow: register an
// approval row, hit POST /gates/{id}/approve, and verify the row is
// resolved with the caller's identity recorded.
func TestApproveGate_HappyPath(t *testing.T) {
	f := newFixture(t)

	// Seed a run + node so the foreign-key constraints accept the
	// approvals row. The simplest path is to drive the runner once
	// against a workflow that pauses on HITL — but that's heavy for
	// this unit test, so we insert minimal substrate rows directly.
	runID, nodeID := seedHITLRow(t, f.runsDB)
	approvalID, err := workflow.CreateHITLRequest(context.Background(), f.runsDB, runID, nodeID, "reviewer", "test", time.Now())
	if err != nil {
		t.Fatalf("CreateHITLRequest: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"role": "reviewer", "reason": "looks good"})
	resp, err := http.Post(f.ts.URL+"/gates/"+approvalID+"/approve", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(resp.Body)
		t.Fatalf("status = %d, body=%s", resp.StatusCode, buf.String())
	}

	// Verify the approval is resolved and the actor is "alice".
	var (
		approver string
		decision string
	)
	err = f.runsDB.QueryRow(`SELECT approver, decision FROM approvals WHERE approval_id = ?`, approvalID).Scan(&approver, &decision)
	if err != nil {
		t.Fatalf("query approvals: %v", err)
	}
	if approver != "alice" {
		t.Errorf("approver = %q, want alice", approver)
	}
	if decision != "approve" {
		t.Errorf("decision = %q, want approve", decision)
	}
}

func TestRejectGate_HappyPath(t *testing.T) {
	f := newFixture(t)
	runID, nodeID := seedHITLRow(t, f.runsDB)
	approvalID, err := workflow.CreateHITLRequest(context.Background(), f.runsDB, runID, nodeID, "", "test", time.Now())
	if err != nil {
		t.Fatalf("CreateHITLRequest: %v", err)
	}
	resp, err := http.Post(f.ts.URL+"/gates/"+approvalID+"/reject", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var decision string
	err = f.runsDB.QueryRow(`SELECT decision FROM approvals WHERE approval_id = ?`, approvalID).Scan(&decision)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if decision != "reject" {
		t.Errorf("decision = %q, want reject", decision)
	}
}

func TestApproveGate_NotFound(t *testing.T) {
	f := newFixture(t)
	resp, err := http.Post(f.ts.URL+"/gates/missing-id/approve", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestApproveGate_RoleMismatch(t *testing.T) {
	f := newFixture(t)
	runID, nodeID := seedHITLRow(t, f.runsDB)
	approvalID, err := workflow.CreateHITLRequest(context.Background(), f.runsDB, runID, nodeID, "owner", "test", time.Now())
	if err != nil {
		t.Fatalf("CreateHITLRequest: %v", err)
	}
	body, _ := json.Marshal(map[string]string{"role": "intern"})
	resp, err := http.Post(f.ts.URL+"/gates/"+approvalID+"/approve", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestApproveGate_NoCaller(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	state, err := workflow.OpenSQLiteState(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	defer state.Close()

	srv := api.New(api.Server{
		RunsDB: state.DB(),
		Identifier: api.IdentifierFunc(func(_ context.Context, _ *http.Request) (api.Caller, error) {
			return api.Caller{}, errors.New("no peer")
		}),
		Runner:  &stubRunner{},
		Version: "test",
	})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/gates/anything/approve", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestListFindings(t *testing.T) {
	f := newFixture(t)
	// Seed an audit run and finding so the response is non-empty.
	rec := audit.AuditRunRecord{
		AuditRunID: "run-1",
		Repo:       "dear-agent",
		Cadence:    audit.CadenceDaily,
		StartedAt:  time.Now(),
		State:      audit.AuditRunRunning,
	}
	if err := f.auditMem.BeginAuditRun(context.Background(), rec); err != nil {
		t.Fatalf("BeginAuditRun: %v", err)
	}
	finding := audit.Finding{
		Repo:        "dear-agent",
		CheckID:     "test.check",
		Fingerprint: "fp1",
		Severity:    audit.SeverityP1,
		Title:       "example finding",
		Detail:      "detail",
	}
	if _, err := f.auditMem.UpsertFinding(context.Background(), finding); err != nil {
		t.Fatalf("UpsertFinding: %v", err)
	}

	resp, err := http.Get(f.ts.URL + "/audit/findings?repo=dear-agent")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Findings []audit.Finding `json:"findings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(body.Findings))
	}
	if body.Findings[0].Title != "example finding" {
		t.Errorf("title = %q", body.Findings[0].Title)
	}
}

func TestListFindings_BadSeverity(t *testing.T) {
	f := newFixture(t)
	resp, err := http.Get(f.ts.URL + "/audit/findings?severity=P9")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestListFindings_AuditDisabled(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	state, err := workflow.OpenSQLiteState(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	defer state.Close()
	srv := api.New(api.Server{
		RunsDB:     state.DB(),
		AuditStore: nil, // disabled
		Identifier: api.AnonymousIdentifier("alice"),
		Runner:     &stubRunner{},
	})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/audit/findings")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
}

func TestRun_HappyPath(t *testing.T) {
	f := newFixture(t)
	f.stubRun.resp = api.RunResponse{PID: 4242}

	body, _ := json.Marshal(api.RunRequest{
		File:   "workflows/foo.yaml",
		Inputs: map[string]string{"key": "value"},
	})
	resp, err := http.Post(f.ts.URL+"/run", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}
	var rr api.RunResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rr.PID != 4242 {
		t.Errorf("pid = %d, want 4242", rr.PID)
	}
	if f.stubRun.lastReq.File != "workflows/foo.yaml" {
		t.Errorf("file = %q", f.stubRun.lastReq.File)
	}
	if f.stubRun.lastCaller.LoginName != "alice" {
		t.Errorf("caller = %q", f.stubRun.lastCaller.LoginName)
	}
}

func TestRun_MissingFile(t *testing.T) {
	f := newFixture(t)
	resp, err := http.Post(f.ts.URL+"/run", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestRoutingRejectsUnknownPath(t *testing.T) {
	f := newFixture(t)
	resp, err := http.Get(f.ts.URL + "/nope")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	f := newFixture(t)
	// /workflows is GET; POST should be 405.
	resp, err := http.Post(f.ts.URL+"/workflows", "application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

// seedHITLRow inserts the minimum runs/nodes rows that
// CreateHITLRequest's foreign keys require, then returns the run and
// node ids. The schema lives in pkg/workflow/schema.sql; this avoids
// driving the runner just to test HTTP handlers.
func seedHITLRow(t *testing.T, db *sql.DB) (string, string) {
	t.Helper()
	ctx := context.Background()
	// Insert a workflow row.
	wfID := fmt.Sprintf("wf-%d", time.Now().UnixNano())
	if _, err := db.ExecContext(ctx, `
		INSERT INTO workflows (workflow_id, name, version, yaml_canonical, registered_at)
		VALUES (?, ?, ?, ?, ?)
	`, wfID, "test", "1", "name: test", time.Now()); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	// Insert a run row.
	runID := fmt.Sprintf("run-%d", time.Now().UnixNano())
	if _, err := db.ExecContext(ctx, `
		INSERT INTO runs (run_id, workflow_id, state, inputs_json, started_at)
		VALUES (?, ?, 'running', '{}', ?)
	`, runID, wfID, time.Now()); err != nil {
		t.Fatalf("insert run: %v", err)
	}
	// Insert a node row in awaiting_hitl.
	nodeID := "review"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO nodes (run_id, node_id, state, attempts, started_at)
		VALUES (?, ?, 'awaiting_hitl', 0, ?)
	`, runID, nodeID, time.Now()); err != nil {
		t.Fatalf("insert node: %v", err)
	}
	return runID, nodeID
}
