package main

import (
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// minimalBashWorkflow returns a one-node bash workflow YAML body that
// echoes a marker so we know the run executed. Used for CLI exit-path
// and persistence tests in this package.
const minimalBashWorkflow = `
name: cli-bash-roundtrip
version: "1"
inputs:
  - name: marker
    required: true
nodes:
  - id: greet
    kind: bash
    bash:
      cmd: echo "hello $INPUT_marker"
`

func TestRun_PersistsRunToSQLite(t *testing.T) {
	dir := t.TempDir()
	wfPath := filepath.Join(dir, "wf.yaml")
	if err := os.WriteFile(wfPath, []byte(minimalBashWorkflow), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	dbPath := filepath.Join(dir, "runs.db")

	stderr := tmpFile(t)
	args := []string{
		"-file", wfPath,
		"-db", dbPath,
		"-trigger", "cron",
		"-input", "marker=world",
	}
	if code := run(args, stderr); code != 0 {
		t.Fatalf("run exit %d, stderr=%s", code, readFile(stderr))
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("runs.db not created: %v", err)
	}

	db := openTestDB(t, dbPath)
	defer db.Close()

	var (
		runs    int
		trigger string
		state   string
	)
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*), COALESCE(MAX(trigger),''), COALESCE(MAX(state),'') FROM runs`,
	).Scan(&runs, &trigger, &state); err != nil {
		t.Fatalf("query runs: %v", err)
	}
	if runs != 1 {
		t.Errorf("runs row count = %d, want 1", runs)
	}
	if trigger != "cron" {
		t.Errorf("runs.trigger = %q, want %q (Bug 3 — flag must thread into Runner.Trigger)", trigger, "cron")
	}
	if state != "succeeded" {
		t.Errorf("runs.state = %q, want succeeded; stderr=%s", state, readFile(stderr))
	}
}

func TestRun_DefaultTriggerIsCLI(t *testing.T) {
	dir := t.TempDir()
	wfPath := filepath.Join(dir, "wf.yaml")
	if err := os.WriteFile(wfPath, []byte(minimalBashWorkflow), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	dbPath := filepath.Join(dir, "runs.db")

	stderr := tmpFile(t)
	args := []string{"-file", wfPath, "-db", dbPath, "-input", "marker=x"}
	if code := run(args, stderr); code != 0 {
		t.Fatalf("run exit %d, stderr=%s", code, readFile(stderr))
	}

	db := openTestDB(t, dbPath)
	defer db.Close()
	var trigger string
	if err := db.QueryRow(`SELECT trigger FROM runs LIMIT 1`).Scan(&trigger); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if trigger != "cli" {
		t.Errorf("default -trigger = %q, want cli", trigger)
	}
}

func TestRun_EmptyDBPathSkipsPersistence(t *testing.T) {
	dir := t.TempDir()
	wfPath := filepath.Join(dir, "wf.yaml")
	if err := os.WriteFile(wfPath, []byte(minimalBashWorkflow), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	// Run with -db="" should succeed without creating any DB file.
	stderr := tmpFile(t)
	args := []string{"-file", wfPath, "-db", "", "-input", "marker=x"}
	if code := run(args, stderr); code != 0 {
		t.Fatalf("run exit %d, stderr=%s", code, readFile(stderr))
	}
	// The current working directory must not contain runs.db either.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, "runs.db")); err == nil {
		t.Error("-db=\"\" still created a runs.db")
	}
}

func TestRun_DryRunDoesNotCreateDB(t *testing.T) {
	dir := t.TempDir()
	wfPath := filepath.Join(dir, "wf.yaml")
	if err := os.WriteFile(wfPath, []byte(minimalBashWorkflow), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	dbPath := filepath.Join(dir, "runs.db")
	stderr := tmpFile(t)
	args := []string{"-file", wfPath, "-db", dbPath, "-dry-run"}
	if code := run(args, stderr); code != 0 {
		t.Fatalf("run exit %d, stderr=%s", code, readFile(stderr))
	}
	if _, err := os.Stat(dbPath); err == nil {
		t.Error("dry-run created a runs.db")
	}
}

func TestRun_MissingFileFlag(t *testing.T) {
	stderr := tmpFile(t)
	if code := run([]string{}, stderr); code != 2 {
		t.Errorf("exit = %d, want 2 (missing -file); stderr=%s", code, readFile(stderr))
	}
	if !strings.Contains(readFile(stderr), "-file is required") {
		t.Errorf("missing usage hint in stderr=%s", readFile(stderr))
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

func tmpFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "log-*")
	if err != nil {
		t.Fatalf("create tmp: %v", err)
	}
	return f
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
