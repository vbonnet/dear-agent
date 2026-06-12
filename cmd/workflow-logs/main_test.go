package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping in short mode (requires go build)")
	}
	out := filepath.Join(t.TempDir(), "workflow-logs")
	if b, err := exec.Command("go", "build", "-o", out, ".").CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, b)
	}
	return out
}

func seedWithAudit(t *testing.T, dbPath string) (string, *workflow.SQLiteState) {
	t.Helper()
	ss, err := workflow.OpenSQLiteState(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	const runID = "r-logs"
	if err := ss.BeginRun(ctx, workflow.RunRecord{
		RunID:        runID,
		WorkflowName: "w",
		State:        workflow.RunStatePending,
		InputsJSON:   "{}",
		StartedAt:    now,
		Trigger:      "test",
	}); err != nil {
		t.Fatalf("BeginRun: %v", err)
	}
	if err := ss.Emit(ctx, workflow.AuditEvent{
		RunID:      runID,
		ToState:    string(workflow.RunStatePending),
		Reason:     "run-created",
		Actor:      "test-actor",
		OccurredAt: now,
	}); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	return runID, ss
}

func TestBuild(t *testing.T) {
	buildBinary(t)
}

func TestLogs_NoRunIDArg(t *testing.T) {
	bin := buildBinary(t)
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	ss, err := workflow.OpenSQLiteState(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	defer ss.Close()

	cmd := exec.Command(bin, "-db", dbPath)
	if err := cmd.Run(); err == nil {
		t.Fatal("expected non-zero exit without run-id")
	}
	if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() != 2 {
		t.Errorf("exit code = %d, want 2", cmd.ProcessState.ExitCode())
	}
}

func TestLogs_ShowsAuditEvents(t *testing.T) {
	bin := buildBinary(t)
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	runID, ss := seedWithAudit(t, dbPath)
	defer ss.Close()

	out, err := exec.Command(bin, "-db", dbPath, runID).CombinedOutput()
	if err != nil {
		t.Fatalf("logs failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "run-created") {
		t.Errorf("output missing audit reason:\n%s", out)
	}
}

func TestLogs_UnknownRun_ExitsCode3(t *testing.T) {
	bin := buildBinary(t)
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	ss, err := workflow.OpenSQLiteState(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	defer ss.Close()

	cmd := exec.Command(bin, "-db", dbPath, "no-such-run")
	if err := cmd.Run(); err == nil {
		t.Fatal("expected non-zero exit for unknown run")
	}
	if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() != 3 {
		t.Errorf("exit code = %d, want 3", cmd.ProcessState.ExitCode())
	}
}

func TestLogs_JSONFlag(t *testing.T) {
	bin := buildBinary(t)
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	runID, ss := seedWithAudit(t, dbPath)
	defer ss.Close()

	out, err := exec.Command(bin, "-db", dbPath, "-json", runID).CombinedOutput()
	if err != nil {
		t.Fatalf("logs -json failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "run-created") {
		t.Errorf("JSON output missing audit reason:\n%s", out)
	}
}
