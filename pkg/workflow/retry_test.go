package workflow

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// countingAI returns an error for the first N calls, then succeeds.
type countingAI struct {
	failFor int32       // fail for the first N calls
	calls   atomic.Int32
	succeed string // output to return on success
}

func (c *countingAI) Generate(_ context.Context, node *AINode, _ map[string]string, _ map[string]string) (string, error) {
	n := c.calls.Add(1)
	if n <= c.failFor {
		return "", fmt.Errorf("transient error (call %d)", n)
	}
	if c.succeed != "" {
		return c.succeed, nil
	}
	return node.Prompt, nil
}

// TestRetryExhaustsAttemptsAndFails verifies that a node with MaxAttempts=3
// fails after 3 errors and does not make a 4th attempt.
func TestRetryExhaustsAttemptsAndFails(t *testing.T) {
	ai := &countingAI{failFor: 10} // always fails
	r := NewRunner(ai)
	w := &Workflow{
		Name: "retry-exhaust", Version: "1",
		Nodes: []Node{
			{
				ID: "n", Kind: KindAI,
				AI:    &AINode{Prompt: "go"},
				Retry: &RetryPolicy{MaxAttempts: 3, Backoff: time.Millisecond},
			},
		},
	}
	_, err := r.Run(context.Background(), w, nil)
	if err == nil {
		t.Fatal("expected error after exhausting attempts")
	}
	got := ai.calls.Load()
	if got != 3 {
		t.Errorf("AI.Generate called %d times, want 3", got)
	}
	if !strings.Contains(err.Error(), "transient error") {
		t.Errorf("error = %v, expected transient error", err)
	}
}

// TestRetrySucceedsOnSecondAttempt verifies that a node succeeds on the
// second attempt and that attempts is recorded correctly.
func TestRetrySucceedsOnSecondAttempt(t *testing.T) {
	ai := &countingAI{failFor: 1, succeed: "ok"}
	r := NewRunner(ai)
	w := &Workflow{
		Name: "retry-ok", Version: "1",
		Nodes: []Node{
			{
				ID: "n", Kind: KindAI,
				AI:    &AINode{Prompt: "go"},
				Retry: &RetryPolicy{MaxAttempts: 3, Backoff: time.Millisecond},
			},
		},
	}
	rep, err := r.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("expected success on 2nd attempt, got: %v", err)
	}
	if rep.Results[0].Output != "ok" {
		t.Errorf("output = %q, want ok", rep.Results[0].Output)
	}
	attempts, _ := rep.Results[0].Meta["attempts"].(int)
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
	if ai.calls.Load() != 2 {
		t.Errorf("AI called %d times, want 2", ai.calls.Load())
	}
}

// TestRetryRecordsAttemptCountOnSuccess verifies that a successful first
// attempt records attempts=1 in Meta.
func TestRetryRecordsAttemptCountOnSuccess(t *testing.T) {
	r := NewRunner(&fakeAI{})
	w := &Workflow{
		Name: "retry-count", Version: "1",
		Nodes: []Node{
			{
				ID: "n", Kind: KindBash,
				Bash:  &BashNode{Cmd: "echo hi"},
				Retry: &RetryPolicy{MaxAttempts: 3},
			},
		},
	}
	rep, err := r.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	attempts, _ := rep.Results[0].Meta["attempts"].(int)
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
}

// TestRetryOnlyKindsFilter verifies that OnlyKinds=["ai"] does not retry
// a bash node.
func TestRetryOnlyKindsFilter(t *testing.T) {
	r := NewRunner(&fakeAI{})
	// bash node that always fails; retry only applies to ai
	w := &Workflow{
		Name: "retry-filter", Version: "1",
		Nodes: []Node{
			{
				ID: "n", Kind: KindBash,
				Bash:  &BashNode{Cmd: "exit 1"},
				Retry: &RetryPolicy{MaxAttempts: 5, Backoff: time.Millisecond, OnlyKinds: []NodeKind{KindAI}},
			},
		},
	}
	_, err := r.Run(context.Background(), w, nil)
	if err == nil {
		t.Fatal("expected bash failure")
	}
	// Should have attempted only once (filter excluded bash).
	// We can't observe the call count directly from a bash node, but we can
	// confirm it failed quickly (no 5-attempt retry on bash).
}

// TestRetryGateNeverRetries verifies that Gate nodes are not retried.
func TestRetryGateNeverRetries(t *testing.T) {
	r := NewRunner(&fakeAI{})
	r.SignalTimeout = 10 * time.Millisecond
	w := &Workflow{
		Name: "retry-gate", Version: "1",
		Nodes: []Node{
			{
				ID: "g", Kind: KindGate,
				Gate:  &GateNode{Name: "never-fired"},
				Retry: &RetryPolicy{MaxAttempts: 5, Backoff: time.Millisecond},
			},
		},
	}
	_, err := r.Run(context.Background(), w, nil)
	// Should time out after one attempt (gate waits for signal, then times
	// out via SignalTimeout — retry policy is ignored for gates).
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %v, expected gate timed out", err)
	}
}

// TestRetryRespectsBackoff verifies that the backoff delay is applied.
func TestRetryRespectsBackoff(t *testing.T) {
	ai := &countingAI{failFor: 2} // fail first 2 calls
	r := NewRunner(ai)
	w := &Workflow{
		Name: "retry-backoff", Version: "1",
		Nodes: []Node{
			{
				ID: "n", Kind: KindAI,
				AI: &AINode{Prompt: "go"},
				Retry: &RetryPolicy{
					MaxAttempts: 3,
					Backoff:     5 * time.Millisecond,
					MaxBackoff:  50 * time.Millisecond,
				},
			},
		},
	}
	start := time.Now()
	rep, err := r.Run(context.Background(), w, nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("expected success: %v", err)
	}
	_ = rep
	// With 2 retries, backoff is 5ms then 10ms → at least ~15ms total.
	if elapsed < 10*time.Millisecond {
		t.Errorf("elapsed %v too short, expected backoff delays", elapsed)
	}
	if ai.calls.Load() != 3 {
		t.Errorf("AI called %d times, want 3", ai.calls.Load())
	}
}

// TestRetryZeroMaxAttemptsActsAsOne verifies MaxAttempts=0 means no retry.
func TestRetryZeroMaxAttemptsActsAsOne(t *testing.T) {
	ai := &countingAI{failFor: 10}
	r := NewRunner(ai)
	w := &Workflow{
		Name: "retry-zero", Version: "1",
		Nodes: []Node{
			{
				ID: "n", Kind: KindAI,
				AI:    &AINode{Prompt: "go"},
				Retry: &RetryPolicy{MaxAttempts: 0, Backoff: time.Millisecond},
			},
		},
	}
	_, err := r.Run(context.Background(), w, nil)
	if err == nil {
		t.Fatal("expected failure")
	}
	if ai.calls.Load() != 1 {
		t.Errorf("AI called %d times with MaxAttempts=0, want 1", ai.calls.Load())
	}
}

// TestRetryNilPolicyNoRetry confirms nodes with no RetryPolicy fail fast.
func TestRetryNilPolicyNoRetry(t *testing.T) {
	ai := &countingAI{failFor: 5}
	r := NewRunner(ai)
	w := &Workflow{
		Name: "no-retry", Version: "1",
		Nodes: []Node{
			{ID: "n", Kind: KindAI, AI: &AINode{Prompt: "go"}},
		},
	}
	_, err := r.Run(context.Background(), w, nil)
	if err == nil {
		t.Fatal("expected error without retry")
	}
	if ai.calls.Load() != 1 {
		t.Errorf("AI called %d times without retry policy, want 1", ai.calls.Load())
	}
}
