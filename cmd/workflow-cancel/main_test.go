package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping in short mode (requires go build)")
	}
	out := filepath.Join(t.TempDir(), "workflow-cancel")
	if b, err := exec.Command("go", "build", "-o", out, ".").CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, b)
	}
	return out
}

func seedRun(t *testing.T, dbPath, runID string) *workflow.SQLiteState {
	t.Helper()
	ss, err := workflow.OpenSQLiteState(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	if err := ss.BeginRun(context.Background(), workflow.RunRecord{
		RunID:        runID,
		WorkflowName: "test-wf",
		State:        workflow.RunStatePending,
		InputsJSON:   "{}",
		StartedAt:    time.Now().UTC(),
		Trigger:      "test",
	}); err != nil {
		t.Fatalf("BeginRun: %v", err)
	}
	return ss
}

func TestBuild(t *testing.T) {
	buildBinary(t)
}

func TestCancel_NoRunIDArg(t *testing.T) {
	bin := buildBinary(t)
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	ss := seedRun(t, dbPath, "r1")
	defer ss.Close()

	cmd := exec.Command(bin, "-db", dbPath)
	if err := cmd.Run(); err == nil {
		t.Fatal("expected non-zero exit without run-id")
	}
	if code, ok := exitCode(cmd); ok && code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestCancel_UnknownRunID(t *testing.T) {
	bin := buildBinary(t)
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	ss := seedRun(t, dbPath, "r1")
	defer ss.Close()

	cmd := exec.Command(bin, "-db", dbPath, "no-such-run")
	if err := cmd.Run(); err == nil {
		t.Fatal("expected non-zero exit for unknown run")
	}
	if code, ok := exitCode(cmd); ok && code != 3 {
		t.Errorf("exit code = %d, want 3 (ErrRunNotFound)", code)
	}
}

func TestCancel_Success(t *testing.T) {
	bin := buildBinary(t)
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	ss := seedRun(t, dbPath, "r-cancel")
	defer ss.Close()

	cmd := exec.Command(bin, "-db", dbPath, "r-cancel")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cancel failed: %v\n%s", err, out)
	}
}

// exitCode extracts the exit code from a failed Cmd.Run.
func exitCode(cmd *exec.Cmd) (int, bool) {
	if cmd.ProcessState == nil {
		return 0, false
	}
	return cmd.ProcessState.ExitCode(), true
}
