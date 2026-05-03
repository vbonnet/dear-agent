package workflow

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// TestRecorderRunsNodesAttempts verifies the BACKLOG ticket 0.3 acceptance:
// after a 5-node run with one retry, the nodes table holds 5 rows and the
// node_attempts table holds 6 rows.
func TestRecorderRunsNodesAttempts(t *testing.T) {
	ss := openTestState(t)

	// Node "c" fails on its first attempt and succeeds on its second —
	// flakyAI tracks calls so we can produce that exact pattern.
	ai := &flakyAI{
		failOnce: map[string]bool{"please-fail-once": true},
	}
	r := NewRunner(ai)
	r.UseSQLiteState(ss)

	w := &Workflow{
		Name: "five-node-retry", Version: "1",
		Nodes: []Node{
			{ID: "a", Kind: KindBash, Bash: &BashNode{Cmd: "echo a"}},
			{ID: "b", Kind: KindBash, Depends: []string{"a"}, Bash: &BashNode{Cmd: "echo b"}},
			{
				ID: "c", Kind: KindAI, Depends: []string{"b"},
				AI:    &AINode{Prompt: "please-fail-once"},
				Retry: &RetryPolicy{MaxAttempts: 2},
			},
			{ID: "d", Kind: KindBash, Depends: []string{"c"}, Bash: &BashNode{Cmd: "echo d"}},
			{ID: "e", Kind: KindBash, Depends: []string{"d"}, Bash: &BashNode{Cmd: "echo e"}},
		},
	}

	if _, err := r.Run(context.Background(), w, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// runs: exactly one row, succeeded.
	var (
		runID   string
		state   string
	)
	if err := ss.DB().QueryRow(`SELECT run_id, state FROM runs`).Scan(&runID, &state); err != nil {
		t.Fatalf("query runs: %v", err)
	}
	if state != "succeeded" {
		t.Errorf("runs.state = %q, want succeeded", state)
	}

	// nodes: 5 rows.
	var nodeCount int
	if err := ss.DB().QueryRow(`SELECT COUNT(*) FROM nodes WHERE run_id = ?`, runID).Scan(&nodeCount); err != nil {
		t.Fatalf("count nodes: %v", err)
	}
	if nodeCount != 5 {
		t.Errorf("nodes count = %d, want 5", nodeCount)
	}

	// All five nodes succeeded.
	var succeeded int
	if err := ss.DB().QueryRow(
		`SELECT COUNT(*) FROM nodes WHERE run_id = ? AND state = 'succeeded'`, runID,
	).Scan(&succeeded); err != nil {
		t.Fatalf("count succeeded: %v", err)
	}
	if succeeded != 5 {
		t.Errorf("succeeded nodes = %d, want 5", succeeded)
	}

	// node_attempts: 6 rows (a:1, b:1, c:2, d:1, e:1).
	var attemptCount int
	if err := ss.DB().QueryRow(
		`SELECT COUNT(*) FROM node_attempts WHERE run_id = ?`, runID,
	).Scan(&attemptCount); err != nil {
		t.Fatalf("count node_attempts: %v", err)
	}
	if attemptCount != 6 {
		t.Errorf("node_attempts count = %d, want 6", attemptCount)
	}

	// node "c" specifically has two attempts: first failed, second succeeded.
	rows, err := ss.DB().Query(
		`SELECT attempt_no, state FROM node_attempts WHERE run_id = ? AND node_id = 'c' ORDER BY attempt_no`,
		runID,
	)
	if err != nil {
		t.Fatalf("query c attempts: %v", err)
	}
	defer rows.Close()
	var got []struct {
		no    int
		state string
	}
	for rows.Next() {
		var no int
		var state string
		if err := rows.Scan(&no, &state); err != nil {
			t.Fatalf("scan c attempt: %v", err)
		}
		got = append(got, struct {
			no    int
			state string
		}{no, state})
	}
	if len(got) != 2 {
		t.Fatalf("c attempts = %d, want 2", len(got))
	}
	if got[0].no != 1 || got[0].state != "failed" {
		t.Errorf("c attempt 1 = %+v, want {no=1 state=failed}", got[0])
	}
	if got[1].no != 2 || got[1].state != "succeeded" {
		t.Errorf("c attempt 2 = %+v, want {no=2 state=succeeded}", got[1])
	}
}

// TestAuditEventsEveryTransition verifies the BACKLOG ticket 0.4 acceptance:
// every state transition produces an audit_events row. For a 2-node bash
// workflow we expect:
//   - run-level: pending→running, running→succeeded (2 rows)
//   - per node: pending→running, running→succeeded (2 rows × 2 nodes = 4)
// Total: 6.
func TestAuditEventsEveryTransition(t *testing.T) {
	ss := openTestState(t)

	r := NewRunner(&fakeAI{})
	r.UseSQLiteState(ss)
	w := &Workflow{
		Name: "audit-test", Version: "1",
		Nodes: []Node{
			{ID: "a", Kind: KindBash, Bash: &BashNode{Cmd: "echo a"}},
			{ID: "b", Kind: KindBash, Depends: []string{"a"}, Bash: &BashNode{Cmd: "echo b"}},
		},
	}
	if _, err := r.Run(context.Background(), w, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var runID string
	if err := ss.DB().QueryRow(`SELECT run_id FROM runs`).Scan(&runID); err != nil {
		t.Fatalf("query run: %v", err)
	}

	var total int
	if err := ss.DB().QueryRow(
		`SELECT COUNT(*) FROM audit_events WHERE run_id = ?`, runID,
	).Scan(&total); err != nil {
		t.Fatalf("count audit_events: %v", err)
	}
	if total != 6 {
		t.Errorf("audit_events count = %d, want 6", total)
	}

	// Per-state count.
	type tally struct {
		fromState string
		toState   string
		count     int
	}
	rows, err := ss.DB().Query(
		`SELECT COALESCE(from_state,''), to_state, COUNT(*) FROM audit_events
		 WHERE run_id = ? GROUP BY from_state, to_state ORDER BY from_state, to_state`,
		runID,
	)
	if err != nil {
		t.Fatalf("group audit_events: %v", err)
	}
	defer rows.Close()
	var got []tally
	for rows.Next() {
		var ta tally
		if err := rows.Scan(&ta.fromState, &ta.toState, &ta.count); err != nil {
			t.Fatalf("scan tally: %v", err)
		}
		got = append(got, ta)
	}
	want := map[string]int{
		"pending→running":   3, // 1 run + 2 nodes
		"running→succeeded": 3, // 1 run + 2 nodes
	}
	for _, g := range got {
		key := g.fromState + "→" + g.toState
		if want[key] != g.count {
			t.Errorf("transition %s = %d, want %d", key, g.count, want[key])
		}
	}
}

// TestAuditEventsRunFailureMarksDownstream verifies that nodes downstream
// of a failure are recorded as skipped — not silently dropped — so the
// nodes table reflects the full DAG.
func TestAuditEventsRunFailureMarksDownstream(t *testing.T) {
	ss := openTestState(t)

	ai := &flakyAI{
		// Make node "a" fail unconditionally.
		alwaysFail: map[string]bool{"will-fail": true},
	}
	r := NewRunner(ai)
	r.UseSQLiteState(ss)

	w := &Workflow{
		Name: "downstream-skip", Version: "1",
		Nodes: []Node{
			{ID: "a", Kind: KindAI, AI: &AINode{Prompt: "will-fail"}},
			{ID: "b", Kind: KindBash, Depends: []string{"a"}, Bash: &BashNode{Cmd: "echo b"}},
			{ID: "c", Kind: KindBash, Depends: []string{"b"}, Bash: &BashNode{Cmd: "echo c"}},
		},
	}
	if _, err := r.Run(context.Background(), w, nil); err == nil {
		t.Fatal("expected Run to return error, got nil")
	}

	var runID string
	var runState string
	if err := ss.DB().QueryRow(`SELECT run_id, state FROM runs`).Scan(&runID, &runState); err != nil {
		t.Fatalf("query run: %v", err)
	}
	if runState != "failed" {
		t.Errorf("runs.state = %q, want failed", runState)
	}

	// b and c were never executed but should appear as skipped.
	var skipped int
	if err := ss.DB().QueryRow(
		`SELECT COUNT(*) FROM nodes WHERE run_id = ? AND state = 'skipped'`, runID,
	).Scan(&skipped); err != nil {
		t.Fatalf("count skipped: %v", err)
	}
	if skipped != 2 {
		t.Errorf("skipped nodes = %d, want 2", skipped)
	}

	// a should be failed.
	var aState string
	if err := ss.DB().QueryRow(
		`SELECT state FROM nodes WHERE run_id = ? AND node_id = 'a'`, runID,
	).Scan(&aState); err != nil {
		t.Fatalf("query a: %v", err)
	}
	if aState != "failed" {
		t.Errorf("nodes.a.state = %q, want failed", aState)
	}
}

// TestMultiAuditSinkFanout verifies that one event reaches every wired sink
// and that a failing sink does not block the others.
func TestMultiAuditSinkFanout(t *testing.T) {
	a := &captureSink{}
	b := &captureSink{}
	failer := &captureSink{err: errors.New("nope")}
	var caught error
	multi := &MultiAuditSink{
		Sinks: []AuditSink{a, failer, b},
		OnError: func(_ AuditSink, _ AuditEvent, err error) {
			caught = err
		},
	}
	if err := multi.Emit(context.Background(), AuditEvent{RunID: "r1", ToState: "running"}); err != nil {
		t.Fatalf("Emit returned %v", err)
	}
	if len(a.events) != 1 || len(b.events) != 1 {
		t.Errorf("a=%d b=%d events, want 1 each", len(a.events), len(b.events))
	}
	if caught == nil {
		t.Error("OnError was not called for failing sink")
	}
}

// TestAuditEventsResumeDoesNotDuplicate verifies that resuming a run
// skips emitting transitions for already-completed nodes — the audit log
// of the original run already captured those transitions.
func TestAuditEventsResumeDoesNotDuplicate(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "runs.db")

	ss1 := mustOpen(t, dbPath)
	r := NewRunner(&fakeAI{})
	r.UseSQLiteState(ss1)
	w := &Workflow{
		Name: "resume-no-dup", Version: "1",
		Nodes: []Node{
			{ID: "a", Kind: KindBash, Bash: &BashNode{Cmd: "echo a"}},
			{ID: "b", Kind: KindBash, Depends: []string{"a"}, Bash: &BashNode{Cmd: "echo b"}},
		},
	}
	if _, err := r.Run(context.Background(), w, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	runID := ss1.RunID()
	_ = ss1.Close()

	// Resume from the snapshot. All nodes are completed, so nothing
	// should re-execute and no new node-level audit rows should appear.
	ss2, err := ResumeSQLiteState(dbPath, runID)
	if err != nil {
		t.Fatalf("ResumeSQLiteState: %v", err)
	}
	defer ss2.Close()

	var beforeNodeRows int
	if err := ss2.DB().QueryRow(
		`SELECT COUNT(*) FROM audit_events WHERE run_id = ? AND node_id IS NOT NULL`,
		runID,
	).Scan(&beforeNodeRows); err != nil {
		t.Fatalf("count node audit rows: %v", err)
	}

	r2 := NewRunner(&fakeAI{})
	r2.UseSQLiteState(ss2)
	if _, err := r2.Resume(context.Background(), w, ss2); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	var afterNodeRows int
	if err := ss2.DB().QueryRow(
		`SELECT COUNT(*) FROM audit_events WHERE run_id = ? AND node_id IS NOT NULL`,
		runID,
	).Scan(&afterNodeRows); err != nil {
		t.Fatalf("count node audit rows after resume: %v", err)
	}
	if afterNodeRows != beforeNodeRows {
		t.Errorf("resume added %d node-level audit rows, want 0",
			afterNodeRows-beforeNodeRows)
	}
}

// ----- helpers -----

// flakyAI returns an error on the first call for a given prompt and
// succeeds on subsequent calls. Lets tests exercise retry semantics
// without sleeping.
type flakyAI struct {
	failOnce   map[string]bool
	alwaysFail map[string]bool
	calls      map[string]int
}

func (f *flakyAI) Generate(_ context.Context, n *AINode, _ map[string]string, _ map[string]string) (string, error) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[n.Prompt]++
	if f.alwaysFail[n.Prompt] {
		return "", errors.New("flakyAI: always-fail")
	}
	if f.failOnce[n.Prompt] && f.calls[n.Prompt] == 1 {
		return "", errors.New("flakyAI: first-call failure")
	}
	return "ok", nil
}

// captureSink records every event it receives. err lets tests force it to
// fail so the multi-sink fan-out path can be exercised.
type captureSink struct {
	events []AuditEvent
	err    error
}

func (c *captureSink) Emit(_ context.Context, ev AuditEvent) error {
	c.events = append(c.events, ev)
	return c.err
}
