package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// seedDB writes one run with two nodes + one audit event into a fresh
// runs.db at dbPath, returning the (RunID, *SQLiteState).
func seedDB(t *testing.T, dbPath string) (string, *workflow.SQLiteState) {
	t.Helper()
	ss, err := workflow.OpenSQLiteState(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	ctx := context.Background()
	runID := "test-run-001"
	now := time.Now().UTC()
	if err := ss.BeginRun(ctx, workflow.RunRecord{
		RunID:        runID,
		WorkflowName: "demo",
		State:        workflow.RunStateRunning,
		InputsJSON:   "{}",
		StartedAt:    now,
		Trigger:      "test",
	}); err != nil {
		t.Fatalf("BeginRun: %v", err)
	}
	for _, id := range []string{"a", "b"} {
		if err := ss.UpsertNode(ctx, workflow.NodeRecord{
			RunID:     runID,
			NodeID:    id,
			State:     workflow.NodeStateSucceeded,
			Output:    "ok",
			StartedAt: now,
		}); err != nil {
			t.Fatalf("UpsertNode: %v", err)
		}
	}
	if err := ss.Emit(ctx, workflow.AuditEvent{
		RunID:      runID,
		FromState:  string(workflow.RunStatePending),
		ToState:    string(workflow.RunStateRunning),
		Reason:     "run-started",
		Actor:      "test",
		OccurredAt: now,
	}); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if err := ss.FinishRun(ctx, runID, workflow.RunStateSucceeded, now.Add(time.Second), ""); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}
	return runID, ss
}

func TestServer_RunListIncludesSeededRun(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	_, ss := seedDB(t, dbPath)
	defer ss.Close()

	srv := NewServer(ss.DB())
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "test-run-001") {
		t.Errorf("body missing run id:\n%s", body)
	}
	if !strings.Contains(body, "<table>") {
		t.Errorf("body missing table:\n%s", body)
	}
}

func TestServer_RunDetailRendersNodesAndAudit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	runID, ss := seedDB(t, dbPath)
	defer ss.Close()

	srv := NewServer(ss.DB())
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/run/"+runID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{runID, "Audit timeline", "run-started"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}

func TestServer_HealthCheck(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	_, ss := seedDB(t, dbPath)
	defer ss.Close()
	srv := NewServer(ss.DB())
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("healthz = %d", rec.Code)
	}
}

func TestServer_NotFoundForUnknownRun(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	_, ss := seedDB(t, dbPath)
	defer ss.Close()
	srv := NewServer(ss.DB())
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/run/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestServer_NotFoundForUnknownPath(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	_, ss := seedDB(t, dbPath)
	defer ss.Close()
	srv := NewServer(ss.DB())
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/some/random/path", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestServer_StateFilter(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	_, ss := seedDB(t, dbPath)
	defer ss.Close()
	srv := NewServer(ss.DB())
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/?state=failed", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Runs — failed") {
		t.Errorf("expected filter heading:\n%s", body)
	}
	if strings.Contains(body, "test-run-001") {
		t.Errorf("seeded run should not appear under failed filter:\n%s", body)
	}
}

func TestFmtTime(t *testing.T) {
	now := time.Date(2026, 5, 3, 10, 11, 12, 0, time.UTC)
	if got := fmtTime(now); got != "2026-05-03 10:11:12Z" {
		t.Errorf("fmtTime time.Time = %q", got)
	}
	ptr := &now
	if got := fmtTime(ptr); got != "2026-05-03 10:11:12Z" {
		t.Errorf("fmtTime *time.Time = %q", got)
	}
	if got := fmtTime(time.Time{}); got != "—" {
		t.Errorf("fmtTime zero = %q", got)
	}
	var zero *time.Time
	if got := fmtTime(zero); got != "—" {
		t.Errorf("fmtTime nil ptr = %q", got)
	}
}
