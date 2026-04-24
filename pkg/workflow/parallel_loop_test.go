package workflow

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestParallelLoopRunsConcurrently verifies that iterations run concurrently
// by tracking peak concurrency with a shared atomic counter.
func TestParallelLoopRunsConcurrently(t *testing.T) {
	var (
		active atomic.Int32
		peakMu sync.Mutex
		peak   int32
	)

	// Each iteration's bash node records active count.
	// Since bash runs in-process here we need an AI executor that
	// increments active for the duration of the call.
	pai := &slowCountingAI{active: &active, peak: &peak, mu: &peakMu}

	r := NewRunner(pai)
	w := &Workflow{
		Name: "parallel-loop", Version: "1",
		Nodes: []Node{
			{ID: "lp", Kind: KindLoop, Loop: &LoopNode{
				Parallel:    true,
				MaxIters:    8,
				Concurrency: 4,
				Nodes: []Node{
					{ID: "work", Kind: KindAI, AI: &AINode{Prompt: "work"}},
				},
			}},
		},
	}
	rep, err := r.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	iters, _ := rep.Results[0].Meta["iterations"].(int)
	if iters != 8 {
		t.Errorf("iterations = %d, want 8", iters)
	}
	peakMu.Lock()
	p := peak
	peakMu.Unlock()
	if p < 2 {
		t.Errorf("peak concurrency = %d, want >= 2 (iterations ran concurrently)", p)
	}
}

// slowCountingAI increments an active counter during Generate, tracks peak.
type slowCountingAI struct {
	active *atomic.Int32
	peak   *int32
	mu     *sync.Mutex
}

func (s *slowCountingAI) Generate(_ context.Context, node *AINode, _ map[string]string, _ map[string]string) (string, error) {
	cur := s.active.Add(1)
	s.mu.Lock()
	if cur > *s.peak {
		*s.peak = cur
	}
	s.mu.Unlock()
	// Sleep briefly so goroutines overlap in wall-clock time.
	time.Sleep(20 * time.Millisecond)
	defer s.active.Add(-1)
	return node.Prompt, nil
}

// TestParallelLoopIterIndexAvailable verifies that {{ .Iter }} is accessible
// in parallel loop iterations.
func TestParallelLoopIterIndexAvailable(t *testing.T) {
	// Use a collecting AI that records the Iter values it sees in prompts.
	rai := &recordingAI{}
	r := NewRunner(rai)
	w := &Workflow{
		Name: "iter-index", Version: "1",
		Nodes: []Node{
			{ID: "lp", Kind: KindLoop, Loop: &LoopNode{
				Parallel: true,
				MaxIters: 3,
				Nodes: []Node{
					{ID: "work", Kind: KindAI, AI: &AINode{Prompt: "iter={{.Iter}}"}},
				},
			}},
		},
	}
	_, err := r.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	rai.mu.Lock()
	defer rai.mu.Unlock()
	if len(rai.calls) != 3 {
		t.Errorf("expected 3 calls, got %d", len(rai.calls))
	}
	seen := make(map[string]bool)
	for _, c := range rai.calls {
		seen[c] = true
	}
	for _, want := range []string{"iter=0", "iter=1", "iter=2"} {
		if !seen[want] {
			t.Errorf("missing iter value %q; calls = %v", want, rai.calls)
		}
	}
}

// recordingAI records all rendered prompts.
type recordingAI struct {
	mu    sync.Mutex
	calls []string
}

func (r *recordingAI) Generate(_ context.Context, node *AINode, _ map[string]string, _ map[string]string) (string, error) {
	r.mu.Lock()
	r.calls = append(r.calls, node.Prompt)
	r.mu.Unlock()
	return node.Prompt, nil
}

// TestParallelLoopZeroMaxItersIsNoop verifies that MaxIters=0 in parallel
// mode runs zero iterations and returns cleanly.
func TestParallelLoopZeroMaxItersIsNoop(t *testing.T) {
	r := NewRunner(&fakeAI{})
	w := &Workflow{
		Name: "parallel-zero", Version: "1",
		Nodes: []Node{
			{ID: "lp", Kind: KindLoop, Loop: &LoopNode{
				Parallel: true,
				MaxIters: 0,
				Nodes: []Node{
					{ID: "work", Kind: KindAI, AI: &AINode{Prompt: "x"}},
				},
			}},
		},
	}
	rep, err := r.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	iters, _ := rep.Results[0].Meta["iterations"].(int)
	if iters != 0 {
		t.Errorf("iterations = %d, want 0", iters)
	}
}

// TestParallelLoopRejectsUntil verifies that Validate rejects Parallel+Until.
func TestParallelLoopRejectsUntil(t *testing.T) {
	y := `
name: parallel-until
version: "1"
nodes:
  - id: lp
    kind: loop
    loop:
      parallel: true
      until: "Outputs.x == done"
      max_iters: 5
      nodes:
        - id: x
          kind: ai
          ai: {prompt: go}
`
	_, err := LoadBytes([]byte(y))
	if err == nil {
		t.Fatal("expected validation error for parallel+until")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error = %v, want mutually exclusive", err)
	}
}

// TestParallelLoopContextCancellation verifies that if the context is
// cancelled, the parallel loop returns promptly.
func TestParallelLoopContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately.
	cancel()

	r := NewRunner(&fakeAI{})
	w := &Workflow{
		Name: "parallel-cancel", Version: "1",
		Nodes: []Node{
			{ID: "lp", Kind: KindLoop, Loop: &LoopNode{
				Parallel: true,
				MaxIters: 100,
				Nodes: []Node{
					{ID: "work", Kind: KindBash, Bash: &BashNode{Cmd: "sleep 10"}},
				},
			}},
		},
	}
	_, err := r.Run(ctx, w, nil)
	if err == nil {
		t.Error("expected context cancellation error")
	}
}
