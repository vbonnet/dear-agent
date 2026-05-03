package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// TestEndToEnd builds the four workflow CLI binaries, seeds a runs.db
// with one full run via the SDK, then exercises each binary in the same
// way a user would. Catches CLI wiring bugs (flag parsing, exit codes,
// the SQLite open string) that unit tests on pkg/workflow miss.
func TestEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "runs.db")

	// 1. Seed a real run.
	ss, err := workflow.OpenSQLiteState(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	r := workflow.NewRunner(nil)
	r.UseSQLiteState(ss)
	w := &workflow.Workflow{
		Name: "cli-end-to-end", Version: "1",
		Nodes: []workflow.Node{
			{ID: "alpha", Kind: workflow.KindBash, Bash: &workflow.BashNode{Cmd: "echo a"}},
			{ID: "beta", Kind: workflow.KindBash, Depends: []string{"alpha"}, Bash: &workflow.BashNode{Cmd: "echo b"}},
		},
	}
	if _, err := r.Run(context.Background(), w, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	runID := ss.RunID()
	_ = ss.Close()

	// 2. Build all four binaries into the temp dir.
	bins := buildBinaries(t, dir)

	// 3. workflow-status — text output.
	out, err := runBin(bins["workflow-status"], "-db", dbPath, runID)
	if err != nil {
		t.Fatalf("workflow-status: %v\n%s", err, out)
	}
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Errorf("workflow-status output missing nodes:\n%s", out)
	}
	if !strings.Contains(out, "succeeded") {
		t.Errorf("workflow-status output missing 'succeeded':\n%s", out)
	}

	// 4. workflow-status --json — JSON output.
	out, err = runBin(bins["workflow-status"], "-db", dbPath, "-json", runID)
	if err != nil {
		t.Fatalf("workflow-status --json: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"workflow": "cli-end-to-end"`) {
		t.Errorf("workflow-status --json missing workflow name:\n%s", out)
	}

	// 5. workflow-list — should include the run.
	out, err = runBin(bins["workflow-list"], "-db", dbPath)
	if err != nil {
		t.Fatalf("workflow-list: %v\n%s", err, out)
	}
	if !strings.Contains(out, runID) {
		t.Errorf("workflow-list output missing run id %s:\n%s", runID, out)
	}

	// 6. workflow-logs — should show 6 transitions.
	out, err = runBin(bins["workflow-logs"], "-db", dbPath, runID)
	if err != nil {
		t.Fatalf("workflow-logs: %v\n%s", err, out)
	}
	if !strings.Contains(out, "running→succeeded") {
		t.Errorf("workflow-logs output missing terminal transition:\n%s", out)
	}

	// 7. workflow-cancel on a terminal run should fail.
	out, err = runBin(bins["workflow-cancel"], "-db", dbPath, runID)
	if err == nil {
		t.Errorf("workflow-cancel on terminal run should fail; got success:\n%s", out)
	}

	// 8. workflow-status with an unknown id exits with code 3.
	cmd := exec.Command(bins["workflow-status"], "-db", dbPath, "no-such-run")
	out2, _ := cmd.CombinedOutput()
	exit := cmd.ProcessState.ExitCode()
	if exit != 3 {
		t.Errorf("unknown-id exit code = %d, want 3 (output: %s)", exit, string(out2))
	}
}

func buildBinaries(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, name := range []string{"workflow-status", "workflow-list", "workflow-cancel", "workflow-logs"} {
		bin := filepath.Join(dir, name)
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/"+name)
		cmd.Env = append(os.Environ(), "GOWORK=off")
		// `go build` runs from the repo root; navigate up from cmd/workflow-status.
		cmd.Dir = mustRepoRoot(t)
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", name, err, string(b))
		}
		out[name] = bin
	}
	return out
}

func mustRepoRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	b, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	return strings.TrimSpace(string(b))
}

func runBin(bin string, args ...string) (string, error) {
	cmd := exec.Command(bin, args...)
	b, err := cmd.CombinedOutput()
	return string(b), err
}
