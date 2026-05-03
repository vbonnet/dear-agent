package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

func TestRun_MigratesSnapshotToSQLite(t *testing.T) {
	dir := t.TempDir()
	snapPath := filepath.Join(dir, "snap.json")
	dbPath := filepath.Join(dir, "runs.db")

	snap := workflow.Snapshot{
		Workflow:  "deep-research",
		RunID:     "run-xyz",
		Inputs:    map[string]string{"q": "routing"},
		Outputs:   map[string]string{"intake": "ok", "research": "ok"},
		Completed: map[string]bool{"intake": true, "research": true},
		Started:   time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 5, 1, 10, 30, 0, 0, time.UTC),
	}
	mustWriteJSON(t, snapPath, snap)

	stdout, stderr := tmpFiles(t)
	code := run([]string{"-db", dbPath, snapPath}, stdout, stderr)
	if code != 0 {
		t.Fatalf("run exit %d, stderr=%s", code, readFile(stderr))
	}
	out := readFile(stdout)
	if !strings.Contains(out, "wrote run run-xyz") {
		t.Errorf("missing confirmation in:\n%s", out)
	}

	db := openTestDB(t, dbPath)
	defer db.Close()
	st, err := workflow.Status(context.Background(), db, "run-xyz")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.State != workflow.RunStateSucceeded {
		t.Errorf("run state = %s, want succeeded", st.State)
	}
	if len(st.Nodes) != 2 {
		t.Errorf("nodes = %d, want 2", len(st.Nodes))
	}
	for _, n := range st.Nodes {
		if n.State != workflow.NodeStateSucceeded {
			t.Errorf("node %s state = %s, want succeeded", n.NodeID, n.State)
		}
	}
}

func TestRun_PartialSnapshotIsRunning(t *testing.T) {
	dir := t.TempDir()
	snapPath := filepath.Join(dir, "snap.json")
	dbPath := filepath.Join(dir, "runs.db")
	snap := workflow.Snapshot{
		Workflow:  "wf",
		RunID:     "run-partial",
		Outputs:   map[string]string{"a": "ok"},
		Completed: map[string]bool{"a": true, "b": false},
		Started:   time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	mustWriteJSON(t, snapPath, snap)
	stdout, stderr := tmpFiles(t)
	if code := run([]string{"-db", dbPath, snapPath}, stdout, stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, readFile(stderr))
	}
	db := openTestDB(t, dbPath)
	defer db.Close()
	st, err := workflow.Status(context.Background(), db, "run-partial")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.State != workflow.RunStateRunning {
		t.Errorf("state = %s, want running (some nodes pending)", st.State)
	}
}

func TestRun_Idempotent(t *testing.T) {
	dir := t.TempDir()
	snapPath := filepath.Join(dir, "snap.json")
	dbPath := filepath.Join(dir, "runs.db")
	snap := workflow.Snapshot{
		Workflow: "wf", RunID: "run-1",
		Outputs:   map[string]string{"a": "x"},
		Completed: map[string]bool{"a": true},
		Started:   time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	mustWriteJSON(t, snapPath, snap)
	for i := 0; i < 2; i++ {
		stdout, stderr := tmpFiles(t)
		if code := run([]string{"-db", dbPath, snapPath}, stdout, stderr); code != 0 {
			t.Fatalf("iteration %d exit %d: %s", i, code, readFile(stderr))
		}
	}
	db := openTestDB(t, dbPath)
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM runs WHERE run_id='run-1'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("rows = %d, want 1 (idempotent)", n)
	}
}

func TestRun_LegacySnapshotWithoutRunID(t *testing.T) {
	dir := t.TempDir()
	snapPath := filepath.Join(dir, "snap.json")
	dbPath := filepath.Join(dir, "runs.db")
	snap := workflow.Snapshot{
		Workflow: "wf",
		Outputs:  map[string]string{"a": "ok"}, Completed: map[string]bool{"a": true},
		Started: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	mustWriteJSON(t, snapPath, snap)
	stdout, stderr := tmpFiles(t)
	if code := run([]string{"-db", dbPath, snapPath}, stdout, stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, readFile(stderr))
	}
	out := readFile(stdout)
	if !strings.Contains(out, "migrated-") {
		t.Errorf("expected derived run-id 'migrated-*', got:\n%s", out)
	}
}

func TestRun_DryRun_DoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	snapPath := filepath.Join(dir, "snap.json")
	dbPath := filepath.Join(dir, "runs.db")
	snap := workflow.Snapshot{Workflow: "wf", RunID: "r", Started: time.Now()}
	mustWriteJSON(t, snapPath, snap)
	stdout, stderr := tmpFiles(t)
	if code := run([]string{"-db", dbPath, "--dry-run", snapPath}, stdout, stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, readFile(stderr))
	}
	if _, err := os.Stat(dbPath); err == nil {
		t.Error("db file created in dry-run mode")
	}
}

func TestRun_RequiresWorkflowName(t *testing.T) {
	dir := t.TempDir()
	snapPath := filepath.Join(dir, "snap.json")
	mustWriteJSON(t, snapPath, workflow.Snapshot{})
	stdout, stderr := tmpFiles(t)
	if code := run([]string{snapPath}, stdout, stderr); code != 2 {
		t.Errorf("exit = %d, want 2 (missing --workflow); stderr=%s", code, readFile(stderr))
	}
}

func mustWriteJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func openTestDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return db
}

func tmpFiles(t *testing.T) (stdout, stderr *os.File) {
	t.Helper()
	a, err := os.CreateTemp(t.TempDir(), "stdout-*")
	if err != nil {
		t.Fatalf("create stdout: %v", err)
	}
	b, err := os.CreateTemp(t.TempDir(), "stderr-*")
	if err != nil {
		t.Fatalf("create stderr: %v", err)
	}
	return a, b
}

func readFile(f *os.File) string {
	if f == nil {
		return ""
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return ""
	}
	b, _ := io.ReadAll(f)
	return string(b)
}
