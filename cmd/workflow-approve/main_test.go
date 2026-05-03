package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// Tests cover the three subcommands end-to-end against a real SQLite DB.
// Each helper isolates the run by re-using a temp dir + capturing stdout.

func setupApproval(t *testing.T) (dbPath, approvalID string) {
	t.Helper()
	dbPath = filepath.Join(t.TempDir(), "runs.db")
	ss, err := workflow.OpenSQLiteState(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	ctx := context.Background()
	if err := ss.BeginRun(ctx, workflow.RunRecord{
		RunID:        "r1",
		WorkflowName: "wf",
		State:        workflow.RunStateRunning,
		InputsJSON:   "{}",
		StartedAt:    time.Now(),
	}); err != nil {
		t.Fatalf("BeginRun: %v", err)
	}
	if err := ss.UpsertNode(ctx, workflow.NodeRecord{
		RunID:  "r1",
		NodeID: "n1",
		State:  workflow.NodeStateRunning,
	}); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	approvalID, err = workflow.CreateHITLRequest(ctx, ss.DB(), "r1", "n1", "reviewer", "needs human", time.Now())
	if err != nil {
		t.Fatalf("CreateHITLRequest: %v", err)
	}
	return dbPath, approvalID
}

func TestRunDecision_ApprovesPendingRow(t *testing.T) {
	dbPath, approvalID := setupApproval(t)
	got := runDecision([]string{"--db", dbPath, "--as", "reviewer", "--actor", "alice", approvalID}, workflow.HITLDecisionApprove)
	if got != 0 {
		t.Errorf("exit code = %d, want 0", got)
	}

	// Verify the approval row is now resolved with decision=approve.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	var decision string
	if err := db.QueryRow(`SELECT decision FROM approvals WHERE approval_id = ?`, approvalID).Scan(&decision); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if decision != "approve" {
		t.Errorf("decision = %q, want approve", decision)
	}
}

func TestRunDecision_RoleMismatch_NonZeroExit(t *testing.T) {
	dbPath, approvalID := setupApproval(t)
	got := runDecision([]string{"--db", dbPath, "--as", "scribe", "--actor", "alice", approvalID}, workflow.HITLDecisionApprove)
	if got != 5 {
		t.Errorf("exit code = %d, want 5 (role mismatch)", got)
	}
}

func TestRunDecision_UnknownApprovalID(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	if _, err := workflow.OpenSQLiteState(dbPath); err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	got := runDecision([]string{"--db", dbPath, "--actor", "alice", "no-such-id"}, workflow.HITLDecisionReject)
	if got != 3 {
		t.Errorf("exit code = %d, want 3 (not-found)", got)
	}
}

func TestRunList_PrintsPending(t *testing.T) {
	dbPath, approvalID := setupApproval(t)

	// Capture stdout to confirm the listing contains the approval id.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	got := runList([]string{"--db", dbPath})
	_ = w.Close()
	os.Stdout = old
	out, _ := readAll(r)

	if got != 0 {
		t.Errorf("exit code = %d, want 0", got)
	}
	if !contains(out, approvalID) {
		t.Errorf("list output missing approval id %q; got %q", approvalID, out)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func readAll(r *os.File) (string, error) {
	buf := make([]byte, 4096)
	out := ""
	for {
		n, err := r.Read(buf)
		if n > 0 {
			out += string(buf[:n])
		}
		if err != nil {
			return out, nil
		}
	}
}
