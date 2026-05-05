package workflow

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestSQLiteStateSaveLoad mirrors TestFileStateSaveLoad — a save/load
// round-trip preserves every field of the snapshot.
func TestSQLiteStateSaveLoad(t *testing.T) {
	ss := openTestState(t)

	snap := Snapshot{
		Workflow:  "test-wf",
		Inputs:    map[string]string{"env": "prod"},
		Outputs:   map[string]string{"n1": "hello"},
		Completed: map[string]bool{"n1": true},
		Started:   time.Now().Truncate(time.Millisecond),
		UpdatedAt: time.Now().Truncate(time.Millisecond),
	}
	if err := ss.Save(context.Background(), snap); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if ss.RunID() == "" {
		t.Fatal("RunID empty after Save")
	}

	got, err := ss.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil {
		t.Fatal("Load returned nil after Save")
	}
	if got.Workflow != "test-wf" {
		t.Errorf("Workflow = %q, want test-wf", got.Workflow)
	}
	if got.Inputs["env"] != "prod" {
		t.Errorf("Inputs[env] = %q, want prod", got.Inputs["env"])
	}
	if got.Outputs["n1"] != "hello" {
		t.Errorf("Outputs[n1] = %q, want hello", got.Outputs["n1"])
	}
	if !got.Completed["n1"] {
		t.Error("Completed[n1] should be true")
	}
}

// TestSQLiteStateLoadEmptyReturnsNil mirrors TestFileStateLoadMissingReturnsNil.
// A freshly opened state with no Save has no run; Load must return (nil, nil).
func TestSQLiteStateLoadEmptyReturnsNil(t *testing.T) {
	ss := openTestState(t)
	got, err := ss.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil snapshot, got %+v", got)
	}
}

// TestSQLiteStateMultipleSaves verifies that subsequent saves overwrite the
// same run rather than creating new ones — RunID is stable across Save.
func TestSQLiteStateMultipleSaves(t *testing.T) {
	ss := openTestState(t)

	snap1 := Snapshot{
		Workflow:  "wf",
		Inputs:    map[string]string{"x": "1"},
		Outputs:   map[string]string{"a": "out-a"},
		Completed: map[string]bool{"a": true},
		Started:   time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := ss.Save(context.Background(), snap1); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	firstID := ss.RunID()
	if firstID == "" {
		t.Fatal("RunID empty after first save")
	}

	snap2 := snap1
	snap2.Outputs = map[string]string{"a": "out-a", "b": "out-b"}
	snap2.Completed = map[string]bool{"a": true, "b": true}
	snap2.UpdatedAt = time.Now().Add(time.Second)
	if err := ss.Save(context.Background(), snap2); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	if ss.RunID() != firstID {
		t.Errorf("RunID changed across saves: %q → %q", firstID, ss.RunID())
	}

	got, err := ss.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Outputs["b"] != "out-b" {
		t.Errorf("Outputs[b] = %q after second save, want out-b", got.Outputs["b"])
	}
	if !got.Completed["b"] {
		t.Error("Completed[b] should be true after second save")
	}

	// Verify only one run exists in the underlying DB.
	var count int
	if err := ss.DB().QueryRow(`SELECT COUNT(*) FROM runs`).Scan(&count); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if count != 1 {
		t.Errorf("runs row count = %d, want 1", count)
	}
}

// TestSQLiteStateConcurrentSaves verifies WAL mode lets independent runs
// (separate SQLiteState instances on the same DB file) write concurrently
// without lock contention.
func TestSQLiteStateConcurrentSaves(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "runs.db")

	const writers = 10
	var wg sync.WaitGroup
	errCh := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ss, err := OpenSQLiteState(dbPath)
			if err != nil {
				errCh <- err
				return
			}
			defer ss.Close()
			snap := Snapshot{
				Workflow:  "concurrent-wf",
				Inputs:    map[string]string{"writer": "x"},
				Outputs:   map[string]string{"n": "out"},
				Completed: map[string]bool{"n": true},
				Started:   time.Now(),
				UpdatedAt: time.Now(),
			}
			if err := ss.Save(context.Background(), snap); err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent Save: %v", err)
	}

	// Verify all runs landed.
	ss, err := OpenSQLiteState(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer ss.Close()
	var count int
	if err := ss.DB().QueryRow(`SELECT COUNT(*) FROM runs`).Scan(&count); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if count < writers {
		t.Errorf("runs persisted = %d, want at least %d", count, writers)
	}
}

// TestSQLiteStateResume verifies that ResumeSQLiteState binds Load/Save to
// an existing run, so callers can recover progress after a crash.
func TestSQLiteStateResume(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "runs.db")

	ss := mustOpen(t, dbPath)
	snap := Snapshot{
		Workflow:  "resume-wf",
		Inputs:    map[string]string{"k": "v"},
		Outputs:   map[string]string{"n1": "out"},
		Completed: map[string]bool{"n1": true},
		Started:   time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := ss.Save(context.Background(), snap); err != nil {
		t.Fatalf("Save: %v", err)
	}
	runID := ss.RunID()
	_ = ss.Close()

	// Reopen and resume.
	resumed, err := ResumeSQLiteState(dbPath, runID)
	if err != nil {
		t.Fatalf("ResumeSQLiteState: %v", err)
	}
	defer resumed.Close()

	got, err := resumed.Load(context.Background())
	if err != nil {
		t.Fatalf("Load after resume: %v", err)
	}
	if got == nil {
		t.Fatal("Load returned nil after resume")
	}
	if got.Workflow != "resume-wf" {
		t.Errorf("Workflow = %q, want resume-wf", got.Workflow)
	}
	if got.Outputs["n1"] != "out" {
		t.Errorf("Outputs[n1] = %q, want out", got.Outputs["n1"])
	}
}

// TestSQLiteStateResumeUnknownRun verifies that a misspelled run_id surfaces
// as an error rather than a silent empty Load.
func TestSQLiteStateResumeUnknownRun(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "runs.db")

	// Open once so the schema is created.
	ss := mustOpen(t, dbPath)
	_ = ss.Close()

	if _, err := ResumeSQLiteState(dbPath, "deadbeef-not-a-run"); err == nil {
		t.Error("expected error resuming unknown run, got nil")
	}
}

// TestSQLiteStateRunnerIntegration verifies that wiring SQLiteState into a
// Runner via Runner.State produces a Save-then-Load cycle equivalent to
// FileState. A 2-node bash workflow leaves both nodes marked completed.
func TestSQLiteStateRunnerIntegration(t *testing.T) {
	ss := openTestState(t)

	r := NewRunner(&fakeAI{})
	r.State = ss
	w := &Workflow{
		Name: "sqlite-runner-test", Version: "1",
		Nodes: []Node{
			{ID: "a", Kind: KindBash, Bash: &BashNode{Cmd: "echo alpha"}},
			{ID: "b", Kind: KindBash, Depends: []string{"a"}, Bash: &BashNode{Cmd: "echo beta"}},
		},
	}
	if _, err := r.Run(context.Background(), w, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, err := ss.Load(context.Background())
	if err != nil {
		t.Fatalf("Load after run: %v", err)
	}
	if got == nil {
		t.Fatal("Load returned nil after run")
	}
	if !got.Completed["a"] || !got.Completed["b"] {
		t.Errorf("expected both nodes completed, got %+v", got.Completed)
	}
}

// TestModelVariantPersistedToRuns verifies that Runner.ModelVariant is written
// to the runs row via BeginRun so cost-vs-quality queries can filter by variant.
func TestModelVariantPersistedToRuns(t *testing.T) {
	ss := openTestState(t)
	ctx := context.Background()

	r := NewRunner(&fakeAI{})
	r.UseSQLiteState(ss)
	r.ModelVariant = "opus-4-7-test"

	w := &Workflow{
		Name: "variant-test", Version: "1",
		Nodes: []Node{
			{ID: "n1", Kind: KindBash, Bash: &BashNode{Cmd: "echo hi"}},
		},
	}
	if _, err := r.Run(ctx, w, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var variant string
	if err := ss.DB().QueryRowContext(ctx,
		`SELECT COALESCE(model_variant, '') FROM runs LIMIT 1`,
	).Scan(&variant); err != nil {
		t.Fatalf("query model_variant: %v", err)
	}
	if variant != "opus-4-7-test" {
		t.Errorf("model_variant = %q, want %q", variant, "opus-4-7-test")
	}
}

// TestModelVariantInRunReport verifies that Runner.ModelVariant is surfaced in
// the RunReport returned from Run, so callers don't need to query the DB.
func TestModelVariantInRunReport(t *testing.T) {
	ss := openTestState(t)
	ctx := context.Background()

	r := NewRunner(&fakeAI{})
	r.UseSQLiteState(ss)
	r.ModelVariant = "sonnet-4-6-control"

	w := &Workflow{
		Name: "report-variant-test", Version: "1",
		Nodes: []Node{
			{ID: "n1", Kind: KindBash, Bash: &BashNode{Cmd: "echo done"}},
		},
	}
	rep, err := r.Run(ctx, w, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.ModelVariant != "sonnet-4-6-control" {
		t.Errorf("RunReport.ModelVariant = %q, want %q", rep.ModelVariant, "sonnet-4-6-control")
	}
}

// TestMigrateSchema verifies that migrateSchema is idempotent: running it
// twice on a database that already has model_variant does not return an error.
func TestMigrateSchema(t *testing.T) {
	ss := openTestState(t)
	ctx := context.Background()

	// migrateSchema is called inside openSQLiteDB; run it again manually
	// to verify the "duplicate column name" path is swallowed.
	if err := migrateSchema(ctx, ss.db); err != nil {
		t.Errorf("second migrateSchema: %v", err)
	}
}

// ----- helpers -----

func openTestState(t *testing.T) *SQLiteState {
	t.Helper()
	dir := t.TempDir()
	return mustOpen(t, filepath.Join(dir, "runs.db"))
}

func mustOpen(t *testing.T, path string) *SQLiteState {
	t.Helper()
	ss, err := OpenSQLiteState(path)
	if err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	return ss
}

