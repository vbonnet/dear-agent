// Command workflow-migrate ports a legacy FileState JSON snapshot
// into the SQLite runs.db that Phase 0 introduced. Existing v0.1
// workflows that persist with FileState can be brought under the
// substrate without rewriting the runner — `workflow migrate
// snap.json` writes the equivalent rows to runs / nodes /
// node_attempts and leaves the JSON file untouched.
//
// Usage:
//
//	workflow-migrate -db ./runs.db --workflow my-wf snap.json
//	workflow-migrate -db ./runs.db --workflow my-wf --dry-run snap.json
//
// Exit codes: 0 = ok, 1 = IO/SQL error, 2 = bad usage.
//
// The migration is idempotent at the run-id level: re-running the
// command with the same snapshot is a no-op (the run row is upserted
// in place rather than duplicated). If the JSON snapshot lacks a
// run_id (legacy snapshots written before Phase 0), the migrate tool
// derives a deterministic id from the workflow name + Started time so
// two invocations of the same snapshot still collapse to one row.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("workflow-migrate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		dbPath      = fs.String("db", "runs.db", "destination SQLite file (created if missing)")
		workflowArg = fs.String("workflow", "", "workflow name (required when snapshot.workflow is empty)")
		dryRun      = fs.Bool("dry-run", false, "report what would be written without committing")
	)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: %s [flags] <snapshot.json>\n\nFlags:\n", "workflow-migrate")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	snapPath := fs.Arg(0)

	snap, err := readSnapshot(snapPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	wfName := snap.Workflow
	if wfName == "" {
		wfName = *workflowArg
	}
	if wfName == "" {
		fmt.Fprintln(stderr, "snapshot has no workflow name; pass --workflow")
		return 2
	}
	runID := snap.RunID
	if runID == "" {
		runID = derivedRunID(wfName, snap)
	}

	plan := migrationPlan{
		RunID:        runID,
		WorkflowName: wfName,
		StartedAt:    coalesceTime(snap.Started),
		UpdatedAt:    coalesceTime(snap.UpdatedAt),
		Inputs:       snap.Inputs,
		Outputs:      snap.Outputs,
		Completed:    snap.Completed,
	}
	if *dryRun {
		fmt.Fprintln(stdout, plan.summary())
		return 0
	}

	ss, err := workflow.OpenSQLiteState(*dbPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer ss.Close()

	if err := apply(context.Background(), ss, plan); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, plan.summary())
	fmt.Fprintf(stdout, "wrote run %s to %s\n", plan.RunID, *dbPath)
	return 0
}

// migrationPlan is the in-memory representation of one snapshot
// before it lands in SQLite. Kept separate so dry-run can print it
// without touching the database.
type migrationPlan struct {
	RunID        string
	WorkflowName string
	StartedAt    time.Time
	UpdatedAt    time.Time
	Inputs       map[string]string
	Outputs      map[string]string
	Completed    map[string]bool
}

func (p migrationPlan) summary() string {
	completedCount := 0
	for _, ok := range p.Completed {
		if ok {
			completedCount++
		}
	}
	return fmt.Sprintf("run=%s workflow=%s nodes=%d completed=%d started=%s",
		p.RunID, p.WorkflowName, len(uniqNodes(p.Outputs, p.Completed)),
		completedCount, p.StartedAt.UTC().Format(time.RFC3339))
}

// readSnapshot loads a v0.1 FileState JSON file. Snapshot is the
// canonical struct; we re-decode here so this command does not depend
// on FileState internals (those may change in Phase 4 cleanup).
func readSnapshot(path string) (workflow.Snapshot, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path is a positional CLI arg
	if err != nil {
		return workflow.Snapshot{}, fmt.Errorf("read %s: %w", path, err)
	}
	var snap workflow.Snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return workflow.Snapshot{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return snap, nil
}

// derivedRunID gives a stable id for legacy snapshots that pre-date
// the Phase 0 RunID field. sha256(workflow + started_at) keeps re-runs
// of `workflow migrate` idempotent without inventing a new id each
// time.
func derivedRunID(workflowName string, snap workflow.Snapshot) string {
	h := sha256.Sum256([]byte(workflowName + "|" + coalesceTime(snap.Started).UTC().Format(time.RFC3339Nano)))
	return "migrated-" + hex.EncodeToString(h[:8])
}

// apply writes the run, nodes, and a single attempt-row per completed
// node. A migrated run is recorded as either succeeded (every node
// completed) or running (some nodes still pending) — the engine has
// no signal that the snapshot represents a final state, only that it
// was the last checkpoint.
//
// The function is idempotent at the run-id level: if the run already
// exists in the DB, BeginRun will fail with a UNIQUE-constraint
// violation; we ignore that and continue with the upserts so re-runs
// of `workflow migrate` against the same snapshot are safe.
func apply(ctx context.Context, ss *workflow.SQLiteState, p migrationPlan) error {
	nodeIDs := uniqNodes(p.Outputs, p.Completed)
	runState := workflow.RunStateRunning
	if allCompleted(p.Completed, nodeIDs) {
		runState = workflow.RunStateSucceeded
	}
	startedAt := p.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	if err := ss.BeginRun(ctx, workflow.RunRecord{
		RunID:        p.RunID,
		WorkflowName: p.WorkflowName,
		State:        runState,
		InputsJSON:   mustJSON(p.Inputs),
		StartedAt:    startedAt,
		Trigger:      "migrate",
	}); err != nil && !isAlreadyExists(err) {
		return fmt.Errorf("begin run: %w", err)
	}
	for _, nodeID := range nodeIDs {
		state := workflow.NodeStatePending
		if p.Completed[nodeID] {
			state = workflow.NodeStateSucceeded
		}
		if err := ss.UpsertNode(ctx, workflow.NodeRecord{
			RunID:      p.RunID,
			NodeID:     nodeID,
			State:      state,
			Output:     p.Outputs[nodeID],
			StartedAt:  startedAt,
			FinishedAt: coalesceTime(p.UpdatedAt),
		}); err != nil {
			return fmt.Errorf("upsert node %s: %w", nodeID, err)
		}
	}
	if runState == workflow.RunStateSucceeded {
		if err := ss.FinishRun(ctx, p.RunID, workflow.RunStateSucceeded, coalesceTime(p.UpdatedAt), ""); err != nil {
			return fmt.Errorf("finish run: %w", err)
		}
	}
	return nil
}

// isAlreadyExists reports whether err looks like "the run row was
// already inserted" — SQLite surfaces a UNIQUE-constraint failure for
// PRIMARY KEY collisions. We check for both the substring (driver-
// agnostic) and the typed wrapper if it shows up.
func isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "UNIQUE constraint failed") || contains(msg, "already exists")
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func uniqNodes(outputs map[string]string, completed map[string]bool) []string {
	seen := make(map[string]struct{}, len(outputs)+len(completed))
	for k := range outputs {
		seen[k] = struct{}{}
	}
	for k := range completed {
		seen[k] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	// Stable order so the dry-run summary is diff-friendly.
	sortStrings(out)
	return out
}

func allCompleted(c map[string]bool, ids []string) bool {
	if len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		if !c[id] {
			return false
		}
	}
	return true
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func coalesceTime(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

