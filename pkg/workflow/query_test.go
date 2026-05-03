package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestStatus_RoundTrips(t *testing.T) {
	ss := openTestState(t)
	r := NewRunner(&fakeAI{})
	r.UseSQLiteState(ss)
	w := &Workflow{
		Name: "status-wf", Version: "1",
		Nodes: []Node{
			{ID: "alpha", Kind: KindBash, Bash: &BashNode{Cmd: "echo a"}},
			{ID: "beta", Kind: KindBash, Depends: []string{"alpha"}, Bash: &BashNode{Cmd: "echo b"}},
		},
	}
	if _, err := r.Run(context.Background(), w, map[string]string{"k": "v"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	st, err := Status(context.Background(), ss.DB(), ss.RunID())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.RunID != ss.RunID() {
		t.Errorf("RunID = %q, want %q", st.RunID, ss.RunID())
	}
	if st.Workflow != "status-wf" {
		t.Errorf("Workflow = %q, want status-wf", st.Workflow)
	}
	if st.State != RunStateSucceeded {
		t.Errorf("State = %q, want succeeded", st.State)
	}
	if len(st.Nodes) != 2 {
		t.Fatalf("Nodes len = %d, want 2", len(st.Nodes))
	}
	for _, n := range st.Nodes {
		if n.State != NodeStateSucceeded {
			t.Errorf("node %s state = %q, want succeeded", n.NodeID, n.State)
		}
	}

	text := FormatRunStatusText(st)
	if !strings.Contains(text, "alpha") || !strings.Contains(text, "beta") {
		t.Errorf("FormatRunStatusText missing node names:\n%s", text)
	}
}

func TestStatus_NotFound(t *testing.T) {
	ss := openTestState(t)
	_, err := Status(context.Background(), ss.DB(), "deadbeef")
	if !errors.Is(err, ErrRunNotFound) {
		t.Errorf("error = %v, want ErrRunNotFound", err)
	}
}

func TestList_FiltersByStateAndOrders(t *testing.T) {
	ss := openTestState(t)
	r := NewRunner(&fakeAI{})
	r.UseSQLiteState(ss)
	w := &Workflow{
		Name: "list-wf", Version: "1",
		Nodes: []Node{{ID: "n", Kind: KindBash, Bash: &BashNode{Cmd: "echo n"}}},
	}
	// Insert a few raw rows so we can filter by every state cleanly without
	// running multiple full workflows (the runner only produces succeeded
	// runs in this happy-path test). Workflow row first to satisfy the FK.
	if _, err := ss.DB().Exec(`
		INSERT INTO workflows (workflow_id, name, version, yaml_canonical, registered_at)
		VALUES ('wf1', 'manual', '1', '', datetime('now'))
	`); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	for _, state := range []string{"succeeded", "failed", "running"} {
		runID := "run-" + state
		if _, err := ss.DB().Exec(`
			INSERT INTO runs (run_id, workflow_id, state, inputs_json, started_at)
			VALUES (?, 'wf1', ?, '{}', datetime('now'))
		`, runID, state); err != nil {
			t.Fatalf("insert run %s: %v", state, err)
		}
	}
	// Add a real run via the runner too, so we exercise the production path.
	if _, err := r.Run(context.Background(), w, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	all, err := List(context.Background(), ss.DB(), ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) < 4 {
		t.Errorf("List returned %d rows, want >= 4", len(all))
	}

	failed, err := List(context.Background(), ss.DB(), ListOptions{State: RunStateFailed})
	if err != nil {
		t.Fatalf("List(failed): %v", err)
	}
	for _, s := range failed {
		if s.State != RunStateFailed {
			t.Errorf("got state %q in failed-filter result", s.State)
		}
	}
}

func TestCancel_MarksRunAndEmitsAudit(t *testing.T) {
	ss := openTestState(t)
	// Insert an in-flight run by hand.
	if _, err := ss.DB().Exec(`
		INSERT INTO workflows (workflow_id, name, version, yaml_canonical, registered_at)
		VALUES ('wfX', 'cancel-test', '1', '', datetime('now'))
	`); err != nil {
		t.Fatalf("insert wf: %v", err)
	}
	if _, err := ss.DB().Exec(`
		INSERT INTO runs (run_id, workflow_id, state, inputs_json, started_at)
		VALUES ('runX', 'wfX', 'running', '{}', datetime('now'))
	`); err != nil {
		t.Fatalf("insert run: %v", err)
	}

	if err := Cancel(context.Background(), ss.DB(), "runX", "user-aborted", "human:vbonnet"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	var state string
	if err := ss.DB().QueryRow(`SELECT state FROM runs WHERE run_id='runX'`).Scan(&state); err != nil {
		t.Fatalf("scan state: %v", err)
	}
	if state != "cancelled" {
		t.Errorf("runs.state = %q, want cancelled", state)
	}

	var auditCount int
	if err := ss.DB().QueryRow(
		`SELECT COUNT(*) FROM audit_events WHERE run_id='runX' AND to_state='cancelled'`,
	).Scan(&auditCount); err != nil {
		t.Fatalf("scan audit: %v", err)
	}
	if auditCount != 1 {
		t.Errorf("cancel audit row count = %d, want 1", auditCount)
	}

	// Cancelling again must not double-count or succeed.
	if err := Cancel(context.Background(), ss.DB(), "runX", "again", "human"); err == nil {
		t.Error("expected error cancelling already-terminal run, got nil")
	}
}

func TestCancel_NotFound(t *testing.T) {
	ss := openTestState(t)
	if err := Cancel(context.Background(), ss.DB(), "nope", "", ""); !errors.Is(err, ErrRunNotFound) {
		t.Errorf("error = %v, want ErrRunNotFound", err)
	}
}

func TestLogs_FiltersByNode(t *testing.T) {
	ss := openTestState(t)
	r := NewRunner(&fakeAI{})
	r.UseSQLiteState(ss)
	w := &Workflow{
		Name: "logs-wf", Version: "1",
		Nodes: []Node{
			{ID: "x", Kind: KindBash, Bash: &BashNode{Cmd: "echo x"}},
			{ID: "y", Kind: KindBash, Depends: []string{"x"}, Bash: &BashNode{Cmd: "echo y"}},
		},
	}
	if _, err := r.Run(context.Background(), w, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	all, err := Logs(context.Background(), ss.DB(), ss.RunID(), LogsOptions{})
	if err != nil {
		t.Fatalf("Logs(all): %v", err)
	}
	// 1 run-start + 1 run-finish + 2 nodes × 2 transitions = 6
	if len(all) != 6 {
		t.Errorf("all logs = %d, want 6", len(all))
	}

	xOnly, err := Logs(context.Background(), ss.DB(), ss.RunID(), LogsOptions{NodeID: "x"})
	if err != nil {
		t.Fatalf("Logs(x): %v", err)
	}
	if len(xOnly) != 2 {
		t.Errorf("x logs = %d, want 2", len(xOnly))
	}
	for _, ev := range xOnly {
		if ev.NodeID != "x" {
			t.Errorf("got NodeID %q in x-filter, want x", ev.NodeID)
		}
	}
}

func TestLogs_NotFound(t *testing.T) {
	ss := openTestState(t)
	if _, err := Logs(context.Background(), ss.DB(), "nope", LogsOptions{}); !errors.Is(err, ErrRunNotFound) {
		t.Errorf("error = %v, want ErrRunNotFound", err)
	}
}
