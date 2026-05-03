package workflow

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Performance targets (ADR-010 §6):
//
//	Read run status (with all nodes) for 100-node DAG  : P95 < 5  ms
//	Append audit event                                  : P95 < 1  ms
//	List 50 most recent runs                            : P95 < 10 ms
//
// These tests verify the targets are met on a freshly seeded SQLite DB.
// They are deliberately not benchmarks: a benchmark reports averages but
// the ADR specifies P95, so we sample N times and assert the percentile.
//
// The numbers are conservative — modernc.org/sqlite is pure Go and is
// roughly 2-3× slower than CGO sqlite3 on contended workloads. If a
// future caller wires the engine onto Postgres, these tests document the
// floor every backend must clear.

const (
	perfSampleCount   = 200
	statusReadP95     = 5 * time.Millisecond
	auditAppendP95    = 1 * time.Millisecond
	listRecentP95     = 10 * time.Millisecond
	perfNodeCount     = 100
	perfRunsForListing = 50
)

// TestPerf_StatusReadP95 measures end-to-end run-status reads — runs JOIN
// nodes — for a synthetic 100-node DAG.
func TestPerf_StatusReadP95(t *testing.T) {
	ss := openTestState(t)
	runID := seedRunWithNodes(t, ss, perfNodeCount)

	samples := make([]time.Duration, perfSampleCount)
	for i := 0; i < perfSampleCount; i++ {
		start := time.Now()
		readRunStatus(t, ss, runID)
		samples[i] = time.Since(start)
	}
	assertP95(t, "status_read", samples, statusReadP95)
}

// TestPerf_AuditAppendP95 measures the per-event INSERT cost for the
// audit_events table. The runner emits one of these per state transition,
// so they must stay cheap to keep the engine's overhead invisible.
func TestPerf_AuditAppendP95(t *testing.T) {
	ss := openTestState(t)
	runID := seedRun(t, ss)

	samples := make([]time.Duration, perfSampleCount)
	for i := 0; i < perfSampleCount; i++ {
		start := time.Now()
		if err := ss.Emit(context.Background(), AuditEvent{
			RunID:     runID,
			ToState:   "running",
			Actor:     "system",
		}); err != nil {
			t.Fatalf("Emit: %v", err)
		}
		samples[i] = time.Since(start)
	}
	assertP95(t, "audit_append", samples, auditAppendP95)
}

// TestPerf_ListRecentRunsP95 measures the cost of `workflow list`-style
// queries: select the 50 most recent runs by started_at.
func TestPerf_ListRecentRunsP95(t *testing.T) {
	ss := openTestState(t)
	for i := 0; i < perfRunsForListing*4; i++ {
		seedRunNamed(t, ss, fmt.Sprintf("perf-list-%d", i))
	}

	samples := make([]time.Duration, perfSampleCount)
	for i := 0; i < perfSampleCount; i++ {
		start := time.Now()
		rows, err := ss.DB().Query(`
			SELECT run_id, state, started_at FROM runs
			ORDER BY started_at DESC LIMIT 50
		`)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		var count int
		for rows.Next() {
			var id, state string
			var ts time.Time
			if err := rows.Scan(&id, &state, &ts); err != nil {
				_ = rows.Close()
				t.Fatalf("scan: %v", err)
			}
			count++
		}
		_ = rows.Close()
		samples[i] = time.Since(start)
		if count == 0 {
			t.Fatal("expected at least 1 row, got 0")
		}
	}
	assertP95(t, "list_recent_runs", samples, listRecentP95)
}

// ----- helpers -----

// seedRun inserts a workflow + a single run row and returns the run_id.
func seedRun(t *testing.T, ss *SQLiteState) string {
	t.Helper()
	return seedRunNamed(t, ss, "perf-wf")
}

func seedRunNamed(t *testing.T, ss *SQLiteState, wfName string) string {
	t.Helper()
	runID := uuid.NewString()
	if err := ss.BeginRun(context.Background(), RunRecord{
		RunID:        runID,
		WorkflowName: wfName,
		State:        RunStateRunning,
		InputsJSON:   "{}",
		StartedAt:    time.Now(),
	}); err != nil {
		t.Fatalf("BeginRun: %v", err)
	}
	return runID
}

// seedRunWithNodes inserts a run plus n nodes (all 'succeeded') so reads
// can be measured against a realistic populated table.
func seedRunWithNodes(t *testing.T, ss *SQLiteState, n int) string {
	t.Helper()
	runID := seedRun(t, ss)
	now := time.Now()
	for i := 0; i < n; i++ {
		if err := ss.UpsertNode(context.Background(), NodeRecord{
			RunID:      runID,
			NodeID:     fmt.Sprintf("n%03d", i),
			State:      NodeStateSucceeded,
			Attempts:   1,
			Output:     "out",
			StartedAt:  now,
			FinishedAt: now,
		}); err != nil {
			t.Fatalf("UpsertNode: %v", err)
		}
	}
	return runID
}

// readRunStatus performs the canonical "what's the run state" query — runs
// JOIN nodes — and discards the rows. Modeled after what `workflow status`
// would do.
func readRunStatus(t *testing.T, ss *SQLiteState, runID string) {
	t.Helper()
	var (
		state, inputsJSON string
		startedAt         time.Time
	)
	if err := ss.DB().QueryRow(
		`SELECT state, inputs_json, started_at FROM runs WHERE run_id = ?`,
		runID,
	).Scan(&state, &inputsJSON, &startedAt); err != nil {
		t.Fatalf("scan run: %v", err)
	}
	rows, err := ss.DB().Query(`
		SELECT node_id, state, attempts, output FROM nodes WHERE run_id = ?
	`, runID)
	if err != nil {
		t.Fatalf("query nodes: %v", err)
	}
	for rows.Next() {
		var id, state, output string
		var attempts int
		if err := rows.Scan(&id, &state, &attempts, &output); err != nil {
			_ = rows.Close()
			t.Fatalf("scan node: %v", err)
		}
	}
	_ = rows.Close()
}

// assertP95 sorts samples and fails the test if the 95th percentile exceeds
// limit. Reports the full distribution on failure so flakes are diagnosable.
func assertP95(t *testing.T, label string, samples []time.Duration, limit time.Duration) {
	t.Helper()
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	p95 := samples[len(samples)*95/100]
	median := samples[len(samples)/2]
	if p95 > limit {
		t.Errorf("%s P95 = %s, want < %s (median=%s, max=%s, n=%d)",
			label, p95, limit, median, samples[len(samples)-1], len(samples))
	} else {
		t.Logf("%s P95 = %s (limit %s, median %s, n=%d)",
			label, p95, limit, median, len(samples))
	}
}

