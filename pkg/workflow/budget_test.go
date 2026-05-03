package workflow

import (
	"context"
	"errors"
	"testing"
	"time"
)

// budgetFakeAI is a minimal AIExecutor used by budget tests. It records the
// last cost it was configured to report and exposes it via CostReporter.
type budgetFakeAI struct {
	out     string
	err     error
	cost    CostEstimate
	calls   int
}

func (f *budgetFakeAI) Generate(ctx context.Context, n *AINode, _ map[string]string, _ map[string]string) (string, error) {
	f.calls++
	return f.out, f.err
}

func (f *budgetFakeAI) LastCost() CostEstimate { return f.cost }

func TestMeterChargeOK(t *testing.T) {
	m := NewMeter(0, 0, 0)
	out := m.Charge(&Node{}, CostEstimate{Tokens: 100, Dollars: 0.10}, time.Second)
	if out.Action != "ok" {
		t.Errorf("Action = %q, want ok", out.Action)
	}
	tok, dol := m.Totals()
	if tok != 100 || dol != 0.10 {
		t.Errorf("Totals = %d/%v", tok, dol)
	}
}

func TestMeterPerNodeFail(t *testing.T) {
	node := &Node{Budget: &Budget{MaxTokens: 50}}
	m := NewMeter(0, 0, 0)
	out := m.Charge(node, CostEstimate{Tokens: 100}, 0)
	if out.Action != "fail" {
		t.Errorf("Action = %q, want fail", out.Action)
	}
}

func TestMeterPerNodeEscalate(t *testing.T) {
	node := &Node{Budget: &Budget{MaxDollars: 1.0, OnOverrun: "escalate"}}
	m := NewMeter(0, 0, 0)
	out := m.Charge(node, CostEstimate{Dollars: 5.0}, 0)
	if out.Action != "escalate" {
		t.Errorf("Action = %q, want escalate", out.Action)
	}
}

func TestMeterPerNodeTruncate(t *testing.T) {
	node := &Node{Budget: &Budget{MaxTokens: 50, OnOverrun: "truncate"}}
	m := NewMeter(0, 0, 0)
	out := m.Charge(node, CostEstimate{Tokens: 100}, 0)
	if out.Action != "truncate" {
		t.Errorf("Action = %q, want truncate", out.Action)
	}
}

func TestMeterPerNodeWallclock(t *testing.T) {
	node := &Node{Budget: &Budget{MaxWallclock: 10 * time.Millisecond}}
	m := NewMeter(0, 0, 0)
	out := m.Charge(node, CostEstimate{}, 100*time.Millisecond)
	if out.Action != "fail" {
		t.Errorf("Action = %q, want fail", out.Action)
	}
}

func TestMeterRunLevelFail(t *testing.T) {
	m := NewMeter(150, 0, 0)
	if got := m.Charge(&Node{}, CostEstimate{Tokens: 100}, 0); got.Action != "ok" {
		t.Fatalf("first charge: %v", got)
	}
	if got := m.Charge(&Node{}, CostEstimate{Tokens: 100}, 0); got.Action != "fail" {
		t.Errorf("second charge: %v", got)
	}
}

func TestMeterRunLevelDollars(t *testing.T) {
	m := NewMeter(0, 1.0, 0)
	out := m.Charge(&Node{}, CostEstimate{Dollars: 2.5}, 0)
	if out.Action != "fail" {
		t.Errorf("Action = %q, want fail", out.Action)
	}
}

func TestMeterRunLevelWallclock(t *testing.T) {
	m := NewMeter(0, 0, time.Millisecond)
	now := time.Now()
	m.startedAt = now.Add(-time.Second) // pretend we started 1s ago
	m.Now = func() time.Time { return now }
	out := m.Charge(&Node{}, CostEstimate{}, 0)
	if out.Action != "fail" {
		t.Errorf("Action = %q, want fail", out.Action)
	}
}

func TestMeteredAIExecutorOK(t *testing.T) {
	inner := &budgetFakeAI{out: "hi", cost: CostEstimate{Tokens: 10, Dollars: 0.01}}
	meter := NewMeter(0, 0, 0)
	w := &MeteredAIExecutor{Inner: inner, Meter: meter, CurrentNode: &Node{ID: "n", Budget: &Budget{}}}
	out, err := w.Generate(context.Background(), &AINode{}, nil, nil)
	if err != nil || out != "hi" {
		t.Errorf("got (%q,%v)", out, err)
	}
	tok, dol := meter.Totals()
	if tok != 10 || dol != 0.01 {
		t.Errorf("Totals = %d/%v", tok, dol)
	}
}

func TestMeteredAIExecutorFails(t *testing.T) {
	inner := &budgetFakeAI{out: "partial", cost: CostEstimate{Dollars: 5.0}}
	meter := NewMeter(0, 0, 0)
	w := &MeteredAIExecutor{
		Inner:       inner,
		Meter:       meter,
		CurrentNode: &Node{ID: "n", Budget: &Budget{MaxDollars: 1.0}},
	}
	_, err := w.Generate(context.Background(), &AINode{}, nil, nil)
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Errorf("expected ErrBudgetExceeded, got %v", err)
	}
}

func TestMeteredAIExecutorTruncates(t *testing.T) {
	// Truncate returns inner output as success even on overrun.
	inner := &budgetFakeAI{out: "partial", cost: CostEstimate{Tokens: 100}}
	meter := NewMeter(0, 0, 0)
	w := &MeteredAIExecutor{
		Inner:       inner,
		Meter:       meter,
		CurrentNode: &Node{ID: "n", Budget: &Budget{MaxTokens: 10, OnOverrun: "truncate"}},
	}
	out, err := w.Generate(context.Background(), &AINode{}, nil, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if out != "partial" {
		t.Errorf("out = %q, want partial", out)
	}
}

type captureCharges struct{ lines []string }

func (c *captureCharges) WriteCharge(line string) { c.lines = append(c.lines, line) }

func TestSlogChargeLoggerWritesLine(t *testing.T) {
	c := &captureCharges{}
	logger := &SlogChargeLogger{Writer: c}
	logger.Charge("research", "claude-opus-4-7", CostEstimate{Tokens: 5000, Dollars: 0.075})
	if len(c.lines) != 1 {
		t.Fatalf("lines = %v", c.lines)
	}
	if got := c.lines[0]; got == "" {
		t.Errorf("empty line")
	}
}
