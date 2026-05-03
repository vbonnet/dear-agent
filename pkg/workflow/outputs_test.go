package workflow

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

type fakeOutputRecorder struct {
	records []OutputRecord
}

func (f *fakeOutputRecorder) RecordOutput(_ context.Context, rec OutputRecord) error {
	f.records = append(f.records, rec)
	return nil
}

type fakeGitCommitter struct {
	calls [][]string
	msg   string
	err   error
}

func (f *fakeGitCommitter) AddAndCommit(_ context.Context, paths []string, message string) error {
	f.calls = append(f.calls, paths)
	f.msg = message
	return f.err
}

func TestMaterialiseOutputsLocalDiskOK(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := &fakeOutputRecorder{}
	w := &OutputWriter{WorkflowDir: dir, Recorder: rec}
	specs := map[string]OutputSpec{
		"report": {Path: "report.md", ContentType: "text/markdown", Durability: DurabilityLocalDisk},
	}
	err := w.MaterialiseOutputs(context.Background(), "run-1", "n", specs, &nodeContext{outputs: map[string]string{}})
	if err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
	if len(rec.records) != 1 {
		t.Fatalf("records = %d", len(rec.records))
	}
	if rec.records[0].Hash == "" || rec.records[0].SizeBytes != 5 {
		t.Errorf("recorded = %+v", rec.records[0])
	}
}

func TestMaterialiseOutputsRunIDTemplate(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "notes", "abc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes/abc/report.md"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := &fakeOutputRecorder{}
	w := &OutputWriter{WorkflowDir: dir, Recorder: rec}
	specs := map[string]OutputSpec{
		"report": {Path: "notes/{{ .RunID }}/report.md", Durability: DurabilityLocalDisk},
	}
	err := w.MaterialiseOutputs(context.Background(), "abc", "n", specs, &nodeContext{outputs: map[string]string{}})
	if err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
	if rec.records[0].Path == "" || filepath.Base(rec.records[0].Path) != "report.md" {
		t.Errorf("path = %q", rec.records[0].Path)
	}
}

func TestMaterialiseOutputsMissingFails(t *testing.T) {
	dir := t.TempDir()
	w := &OutputWriter{WorkflowDir: dir}
	specs := map[string]OutputSpec{
		"report": {Path: "missing.md", Durability: DurabilityLocalDisk},
	}
	err := w.MaterialiseOutputs(context.Background(), "run", "n", specs, &nodeContext{outputs: map[string]string{}})
	if !errors.Is(err, ErrOutputMissing) {
		t.Errorf("expected ErrOutputMissing, got %v", err)
	}
}

func TestMaterialiseOutputsEphemeralAllowsMissing(t *testing.T) {
	dir := t.TempDir()
	rec := &fakeOutputRecorder{}
	w := &OutputWriter{WorkflowDir: dir, Recorder: rec}
	specs := map[string]OutputSpec{
		"r": {Path: "missing.md", Durability: DurabilityEphemeral},
	}
	err := w.MaterialiseOutputs(context.Background(), "run", "n", specs, &nodeContext{outputs: map[string]string{}})
	if err != nil {
		t.Errorf("ephemeral should not require existence: %v", err)
	}
	if len(rec.records) != 1 {
		t.Errorf("record count = %d", len(rec.records))
	}
}

func TestMaterialiseOutputsGitCommitterCalled(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := &fakeGitCommitter{}
	w := &OutputWriter{WorkflowDir: dir, Git: g}
	specs := map[string]OutputSpec{
		"r": {Path: "x.md", Durability: DurabilityGitCommitted},
	}
	if err := w.MaterialiseOutputs(context.Background(), "run", "n", specs, &nodeContext{outputs: map[string]string{}}); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
	if len(g.calls) != 1 {
		t.Errorf("git committer called %d times, want 1", len(g.calls))
	}
}

func TestMaterialiseOutputsAlphabeticalOrder(t *testing.T) {
	// The recorder receives outputs in alphabetical key order so audit
	// logs are diff-friendly.
	dir := t.TempDir()
	for _, name := range []string{"a.md", "b.md", "c.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rec := &fakeOutputRecorder{}
	w := &OutputWriter{WorkflowDir: dir, Recorder: rec}
	specs := map[string]OutputSpec{
		"c": {Path: "c.md", Durability: DurabilityLocalDisk},
		"a": {Path: "a.md", Durability: DurabilityLocalDisk},
		"b": {Path: "b.md", Durability: DurabilityLocalDisk},
	}
	if err := w.MaterialiseOutputs(context.Background(), "run", "n", specs, &nodeContext{outputs: map[string]string{}}); err != nil {
		t.Fatalf("got %v", err)
	}
	want := []string{"a", "b", "c"}
	for i, key := range want {
		if rec.records[i].OutputKey != key {
			t.Errorf("record[%d] = %q, want %q", i, rec.records[i].OutputKey, key)
		}
	}
}

func TestSQLiteStateRecordOutputRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ss, err := OpenSQLiteState(filepath.Join(dir, "runs.db"))
	if err != nil {
		t.Fatalf("OpenSQLiteState: %v", err)
	}
	defer ss.Close()
	if err := ss.BeginRun(context.Background(), RunRecord{
		RunID: "r1", WorkflowName: "wf", State: RunStateRunning, InputsJSON: "{}",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ss.UpsertNode(context.Background(), NodeRecord{RunID: "r1", NodeID: "n1", State: NodeStateRunning}); err != nil {
		t.Fatal(err)
	}
	rec := OutputRecord{
		RunID: "r1", NodeID: "n1", OutputKey: "report",
		Path: "/tmp/x.md", ContentType: "text/markdown",
		Durability: DurabilityLocalDisk, SizeBytes: 42, Hash: "abc",
	}
	if err := ss.RecordOutput(context.Background(), rec); err != nil {
		t.Fatalf("RecordOutput: %v", err)
	}
	got, err := ss.queryOutput(context.Background(), "r1", "n1", "report")
	if err != nil {
		t.Fatalf("queryOutput: %v", err)
	}
	if got.Path != rec.Path || got.SizeBytes != 42 || got.Hash != "abc" || got.Durability != DurabilityLocalDisk {
		t.Errorf("got %+v", got)
	}
}

// TestShellGitCommitterAgainstRealRepo only runs when git is on PATH.
// Provides the round-trip smoke test for the default committer.
func TestShellGitCommitterAgainstRealRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	for _, c := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
	} {
		cmd := exec.Command("git", c...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", c, err, out)
		}
	}
	target := filepath.Join(dir, "x.md")
	if err := os.WriteFile(target, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := &ShellGitCommitter{Dir: dir}
	if err := g.AddAndCommit(context.Background(), []string{target}, "init"); err != nil {
		t.Fatalf("AddAndCommit: %v", err)
	}
	// Idempotent second call — same content, "nothing to commit".
	if err := g.AddAndCommit(context.Background(), []string{target}, "init"); err != nil {
		t.Errorf("second commit: %v", err)
	}
}
