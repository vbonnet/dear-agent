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
	out := filepath.Join(t.TempDir(), "workflow-list")
	if b, err := exec.Command("go", "build", "-o", out, ".").CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, b)
	}
	return out
}

func seedDB(t *testing.T, dbPath string) *workflow.SQLiteState {
	t.Helper()
	ss, err := workflow.OpenSQLiteState(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	if err := ss.BeginRun(ctx, workflow.RunRecord{
		RunID:        "run-001",
		WorkflowName: "demo",
		State:        workflow.RunStatePending,
		InputsJSON:   "{}",
		StartedAt:    now,
		Trigger:      "test",
	}); err != nil {
		t.Fatalf("BeginRun: %v", err)
	}
	return ss
}

func TestBuild(t *testing.T) {
	buildBinary(t)
}

func TestList_ShowsSeededRun(t *testing.T) {
	bin := buildBinary(t)
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	ss := seedDB(t, dbPath)
	defer ss.Close()

	out, err := exec.Command(bin, "-db", dbPath).CombinedOutput()
	if err != nil {
		t.Fatalf("list failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "run-001") {
		t.Errorf("output missing run-001:\n%s", out)
	}
}

func TestList_JSONFlag(t *testing.T) {
	bin := buildBinary(t)
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	ss := seedDB(t, dbPath)
	defer ss.Close()

	out, err := exec.Command(bin, "-db", dbPath, "-json").CombinedOutput()
	if err != nil {
		t.Fatalf("list -json failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"run_id"`) && !strings.Contains(string(out), `"RunID"`) {
		t.Errorf("JSON output missing run_id field:\n%s", out)
	}
}

func TestList_StateFilter_NoMatch(t *testing.T) {
	bin := buildBinary(t)
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	ss := seedDB(t, dbPath)
	defer ss.Close()

	// Filter for succeeded — the seeded run is pending, so no match.
	cmd := exec.Command(bin, "-db", dbPath, "-state", "succeeded")
	out, _ := cmd.CombinedOutput()
	if strings.Contains(string(out), "run-001") {
		t.Errorf("pending run should not appear with -state=succeeded:\n%s", out)
	}
}
