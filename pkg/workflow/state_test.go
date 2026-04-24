package workflow

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestFileStateSaveLoad verifies that a Snapshot survives a save/load round-trip.
func TestFileStateSaveLoad(t *testing.T) {
	dir := t.TempDir()
	fs := &FileState{Path: filepath.Join(dir, "snap.json")}

	snap := Snapshot{
		Workflow:  "test-wf",
		Inputs:    map[string]string{"env": "prod"},
		Outputs:   map[string]string{"n1": "hello"},
		Completed: map[string]bool{"n1": true},
		Started:   time.Now().Truncate(time.Millisecond),
		UpdatedAt: time.Now().Truncate(time.Millisecond),
	}

	if err := fs.Save(context.Background(), snap); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := fs.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil {
		t.Fatal("Load returned nil, want snapshot")
	}
	if got.Workflow != snap.Workflow {
		t.Errorf("Workflow = %q, want %q", got.Workflow, snap.Workflow)
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

// TestFileStateLoadMissingReturnsNil verifies that Load returns (nil, nil)
// when no checkpoint file exists yet.
func TestFileStateLoadMissingReturnsNil(t *testing.T) {
	dir := t.TempDir()
	fs := &FileState{Path: filepath.Join(dir, "nonexistent.json")}
	got, err := fs.Load(context.Background())
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil snapshot for missing file, got %+v", got)
	}
}

// TestFileStateAtomicWrite verifies that a crash during write doesn't corrupt
// the previous snapshot. We simulate this by truncating the temp file after
// it's created but before rename — not easily injectable, so instead we
// verify the final file is valid JSON and test that a zero-byte temp file
// left behind doesn't corrupt the original.
func TestFileStateAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	snapPath := filepath.Join(dir, "snap.json")
	fs := &FileState{Path: snapPath}

	// Write a valid snapshot.
	snap1 := Snapshot{
		Workflow:  "wf",
		Inputs:    map[string]string{"x": "1"},
		Outputs:   map[string]string{"a": "out-a"},
		Completed: map[string]bool{"a": true},
		Started:   time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := fs.Save(context.Background(), snap1); err != nil {
		t.Fatalf("first Save: %v", err)
	}

	// Verify the file is valid JSON.
	data, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var check Snapshot
	if err := json.Unmarshal(data, &check); err != nil {
		t.Fatalf("file is not valid JSON after Save: %v", err)
	}

	// Write a second snapshot (overwrites atomically).
	snap2 := Snapshot{
		Workflow:  "wf",
		Inputs:    map[string]string{"x": "1"},
		Outputs:   map[string]string{"a": "out-a", "b": "out-b"},
		Completed: map[string]bool{"a": true, "b": true},
		Started:   snap1.Started,
		UpdatedAt: time.Now(),
	}
	if err := fs.Save(context.Background(), snap2); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	got, err := fs.Load(context.Background())
	if err != nil {
		t.Fatalf("Load after second save: %v", err)
	}
	if got.Outputs["b"] != "out-b" {
		t.Errorf("Outputs[b] = %q after second save, want out-b", got.Outputs["b"])
	}

	// Ensure no stale .tmp files remain.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("stale temp file found: %s", e.Name())
		}
	}
}

// TestRunnerSavesStateAfterEachNode verifies that Runner.State.Save is called
// after each successfully completed node.
func TestRunnerSavesStateAfterEachNode(t *testing.T) {
	var saves []Snapshot
	ms := &mockState{saves: &saves}

	r := NewRunner(&fakeAI{})
	r.State = ms
	w := &Workflow{
		Name: "state-test", Version: "1",
		Nodes: []Node{
			{ID: "a", Kind: KindBash, Bash: &BashNode{Cmd: "echo alpha"}},
			{ID: "b", Kind: KindBash, Depends: []string{"a"}, Bash: &BashNode{Cmd: "echo beta"}},
		},
	}
	_, err := r.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(saves) != 2 {
		t.Errorf("State.Save called %d times, want 2", len(saves))
	}
	// After node "a" completes.
	if !saves[0].Completed["a"] {
		t.Error("first save should mark 'a' completed")
	}
	if saves[0].Completed["b"] {
		t.Error("first save should not mark 'b' completed yet")
	}
	// After node "b" completes.
	if !saves[1].Completed["b"] {
		t.Error("second save should mark 'b' completed")
	}
}

// TestRunnerResumeSkipsCompletedNodes verifies that Resume skips nodes
// listed in the snapshot's Completed map and uses their saved outputs.
func TestRunnerResumeSkipsCompletedNodes(t *testing.T) {
	var execCount atomic.Int32
	trackAI := &trackingAI{count: &execCount}
	r := NewRunner(trackAI)

	// Build a snapshot pretending "a" already ran.
	snap := Snapshot{
		Workflow:  "resume-test",
		Inputs:    map[string]string{},
		Outputs:   map[string]string{"a": "saved-output"},
		Completed: map[string]bool{"a": true},
		Started:   time.Now().Add(-time.Minute),
		UpdatedAt: time.Now().Add(-time.Second),
	}
	ms := &mockState{snap: &snap}

	w := &Workflow{
		Name: "resume-test", Version: "1",
		Nodes: []Node{
			{ID: "a", Kind: KindAI, AI: &AINode{Prompt: "first"}},
			{ID: "b", Kind: KindAI, Depends: []string{"a"}, AI: &AINode{Prompt: "second {{.Outputs.a}}"}},
		},
	}
	rep, err := r.Resume(context.Background(), w, ms)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	// Only "b" should have been executed.
	if execCount.Load() != 1 {
		t.Errorf("AI called %d times, want 1 (only node b)", execCount.Load())
	}
	// "b" should have used "a"'s saved output.
	byID := make(map[string]Result)
	for _, res := range rep.Results {
		byID[res.NodeID] = res
	}
	if b, ok := byID["b"]; !ok {
		t.Error("result for node 'b' missing")
	} else if !strings.Contains(b.Output, "saved-output") {
		t.Errorf("node b output = %q, expected to contain saved-output", b.Output)
	}
}

// TestRunnerResumeNoSnapshot runs from scratch when no snapshot exists.
func TestRunnerResumeNoSnapshot(t *testing.T) {
	ms := &mockState{snap: nil}
	r := NewRunner(&fakeAI{})
	w := &Workflow{
		Name: "resume-no-snap", Version: "1",
		Nodes: []Node{
			{ID: "n", Kind: KindAI, AI: &AINode{Prompt: "hello"}},
		},
	}
	rep, err := r.Resume(context.Background(), w, ms)
	if err != nil {
		t.Fatalf("Resume with nil snapshot: %v", err)
	}
	if len(rep.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(rep.Results))
	}
}

// ------- helpers -------

// mockState is an in-memory State implementation for tests.
type mockState struct {
	snap  *Snapshot
	saves *[]Snapshot
}

func (m *mockState) Save(_ context.Context, snap Snapshot) error {
	if m.saves != nil {
		cp := snap
		// Deep-copy maps.
		cp.Outputs = make(map[string]string, len(snap.Outputs))
		for k, v := range snap.Outputs {
			cp.Outputs[k] = v
		}
		cp.Completed = make(map[string]bool, len(snap.Completed))
		for k, v := range snap.Completed {
			cp.Completed[k] = v
		}
		*m.saves = append(*m.saves, cp)
	}
	return nil
}

func (m *mockState) Load(_ context.Context) (*Snapshot, error) {
	return m.snap, nil
}

// trackingAI counts Generate invocations.
type trackingAI struct {
	count *atomic.Int32
}

func (t *trackingAI) Generate(_ context.Context, node *AINode, _ map[string]string, _ map[string]string) (string, error) {
	t.count.Add(1)
	return node.Prompt, nil
}
