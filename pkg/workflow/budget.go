package workflow

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// CostEstimate is the per-call cost an AIExecutor reports back to the
// runner so the budget meter can debit it. AIExecutor implementations
// that don't yet report cost may return zero — the meter then only
// enforces wallclock and (for callers that fed it Tokens) max_tokens.
type CostEstimate struct {
	Tokens  int
	Dollars float64
}

// CostReporter is the optional interface an AIExecutor can implement to
// surface the cost of its last Generate call. The runner type-asserts on
// it after each AI call; implementations that don't satisfy it are
// treated as "cost unknown".
//
// Why optional: most existing executors don't track cost yet. Making
// this a separate interface lets Phase 1 ship the budget surface
// without forcing every executor to add a method on day one.
type CostReporter interface {
	LastCost() CostEstimate
}

// ErrBudgetExceeded is returned when a budget cap is hit. Callers can
// errors.Is on it to distinguish budget overruns from generic errors.
var ErrBudgetExceeded = errors.New("workflow: budget exceeded")

// budgetOutcome is the signal Meter.Charge returns. The runner reads
// Action to decide whether to fail the node, escalate it (Phase 2 wires
// HITL; Phase 1 logs and fails), or truncate (return the partial output
// as-is).
type budgetOutcome struct {
	Action string // "ok"|"fail"|"escalate"|"truncate"
	Reason string
}

// Meter tracks token, dollar, and wallclock spend for a single node and
// for the overall run. It enforces both ceilings: the per-node Budget
// declared on the Node, plus an optional run-level cap passed in via
// Runner.MaxRunDollars / MaxRunTokens / MaxRunWallclock.
//
// Meter is safe for concurrent use from multiple loop iterations. The
// run-level totals are protected by mu; per-node Charge calls do not
// share state across nodes (a fresh Meter per node would also work, but
// sharing one Meter keeps the run-level accounting in one place).
type Meter struct {
	// Run-level caps. Zero means "no cap at this dimension".
	MaxRunTokens    int
	MaxRunDollars   float64
	MaxRunWallclock time.Duration

	// Now is overridable in tests. Defaults to time.Now.
	Now func() time.Time

	mu        sync.Mutex
	tokens    int
	dollars   float64
	startedAt time.Time
}

// NewMeter constructs a Meter with the given run-level caps. The clock
// is captured at construction so wallclock budgets count from "the
// run started", not "the meter was first charged".
func NewMeter(maxTokens int, maxDollars float64, maxWallclock time.Duration) *Meter {
	return &Meter{
		MaxRunTokens:    maxTokens,
		MaxRunDollars:   maxDollars,
		MaxRunWallclock: maxWallclock,
		Now:             time.Now,
		startedAt:       time.Now(),
	}
}

// Totals snapshots the running totals. Used by the runner to write
// per-run aggregate columns and by tests to assert spend.
func (m *Meter) Totals() (tokens int, dollars float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tokens, m.dollars
}

// Charge debits the meter and decides what the runner should do. The
// per-node Budget cap is checked against the per-call cost (so a single
// $100 call against a $5/node budget is rejected). The run-level caps
// are checked against the cumulative totals after the debit.
//
// Returns budgetOutcome{Action="ok"} when the call is within budget.
// Returns Action="fail"|"escalate"|"truncate" when a cap is hit; the
// returned Reason is suitable for the audit log's reason column.
func (m *Meter) Charge(node *Node, cost CostEstimate, perNodeElapsed time.Duration) budgetOutcome {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens += cost.Tokens
	m.dollars += cost.Dollars

	// Per-node ceiling.
	if b := node.Budget; b != nil {
		if b.MaxTokens > 0 && cost.Tokens > b.MaxTokens {
			return outcomeFromBudget(b, fmt.Sprintf("node tokens %d > max_tokens %d", cost.Tokens, b.MaxTokens))
		}
		if b.MaxDollars > 0 && cost.Dollars > b.MaxDollars {
			return outcomeFromBudget(b, fmt.Sprintf("node dollars $%.4f > max_dollars $%.4f", cost.Dollars, b.MaxDollars))
		}
		if b.MaxWallclock > 0 && perNodeElapsed > b.MaxWallclock {
			return outcomeFromBudget(b, fmt.Sprintf("node wallclock %s > max_wallclock %s", perNodeElapsed, b.MaxWallclock))
		}
	}

	// Run-level ceilings — cumulative.
	if m.MaxRunTokens > 0 && m.tokens > m.MaxRunTokens {
		return budgetOutcome{Action: "fail", Reason: fmt.Sprintf("run tokens %d > MaxRunTokens %d", m.tokens, m.MaxRunTokens)}
	}
	if m.MaxRunDollars > 0 && m.dollars > m.MaxRunDollars {
		return budgetOutcome{Action: "fail", Reason: fmt.Sprintf("run dollars $%.4f > MaxRunDollars $%.4f", m.dollars, m.MaxRunDollars)}
	}
	if m.MaxRunWallclock > 0 && m.Now().Sub(m.startedAt) > m.MaxRunWallclock {
		return budgetOutcome{Action: "fail", Reason: fmt.Sprintf("run wallclock %s > MaxRunWallclock %s", m.Now().Sub(m.startedAt), m.MaxRunWallclock)}
	}

	return budgetOutcome{Action: "ok"}
}

// outcomeFromBudget translates a per-node Budget.OnOverrun into a
// budgetOutcome. Defaults to "fail" so callers who don't think about
// the policy still get a safe behaviour.
func outcomeFromBudget(b *Budget, reason string) budgetOutcome {
	switch b.OnOverrun {
	case "escalate":
		return budgetOutcome{Action: "escalate", Reason: reason}
	case "truncate":
		return budgetOutcome{Action: "truncate", Reason: reason}
	default:
		return budgetOutcome{Action: "fail", Reason: reason}
	}
}

// MeteredAIExecutor wraps an AIExecutor and a Meter; after each
// Generate call it reads the underlying executor's cost (via
// CostReporter, if implemented) and consults the meter.
//
// When the meter rejects the call, the wrapper:
//   - Action=fail: returns ErrBudgetExceeded wrapped with the reason.
//   - Action=escalate: same as fail in Phase 1; logs a hint that
//     Phase 2 will route this through HITL.
//   - Action=truncate: returns the response text the inner executor
//     produced, but signals success — the runner treats the node as
//     succeeded with whatever it has.
//
// The wrapper does NOT enforce per-node wallclock here — that is the
// runner's existing Timeout field (per Node.Timeout). Budget wallclock
// is enforced at the run level via MaxRunWallclock so a single slow
// node doesn't bring down the whole run.
type MeteredAIExecutor struct {
	Inner AIExecutor
	Meter *Meter

	// Logger is the slog hook the wrapper uses for the "$ printout"
	// per node. Optional; nil means no per-call logging.
	Logger ChargeLogger

	// CurrentNode is set by the runner immediately before Generate so
	// the wrapper can read the per-node Budget. The runner sets it
	// back to nil afterward; concurrent callers must wrap their own
	// Meter, not share one.
	CurrentNode *Node
	// CurrentNodeStarted is set by the runner alongside CurrentNode
	// so the wrapper can compute per-node elapsed wallclock for the
	// budget check. Reset to zero between nodes.
	CurrentNodeStarted time.Time
}

// ChargeLogger is the minimal interface the meter calls into for
// "$0.0123 / 1500 tokens — research" line printing. Decoupled from
// slog so tests can capture lines without a logger setup.
type ChargeLogger interface {
	Charge(nodeID, model string, cost CostEstimate)
}

// Generate forwards to Inner, then debits the meter. Budget violations
// return ErrBudgetExceeded; the runner translates that into the right
// node-state transition.
func (m *MeteredAIExecutor) Generate(ctx context.Context, node *AINode, inputs, outputs map[string]string) (string, error) {
	out, err := m.Inner.Generate(ctx, node, inputs, outputs)

	// Even on error the inner executor may have spent tokens (LLM
	// returned a partial response, then a stream error fired); we
	// still want the cost on the books.
	cost := m.lastCost()

	elapsed := time.Duration(0)
	if !m.CurrentNodeStarted.IsZero() {
		elapsed = time.Since(m.CurrentNodeStarted)
	}
	outcome := budgetOutcome{Action: "ok"}
	if m.CurrentNode != nil && m.Meter != nil {
		outcome = m.Meter.Charge(m.CurrentNode, cost, elapsed)
	}
	if m.Logger != nil && m.CurrentNode != nil {
		m.Logger.Charge(m.CurrentNode.ID, node.Model, cost)
	}

	switch outcome.Action {
	case "fail", "escalate":
		// Phase 1 treats escalate as fail; Phase 2 will route to HITL.
		// The error wraps ErrBudgetExceeded so callers can detect it.
		return out, fmt.Errorf("%w: %s", ErrBudgetExceeded, outcome.Reason)
	case "truncate":
		// Return the output as-is and swallow the inner error: the
		// node is treated as succeeded with partial output.
		return out, nil
	}
	return out, err
}

// lastCost reads CostEstimate from the inner executor when it satisfies
// CostReporter; otherwise returns zero.
func (m *MeteredAIExecutor) lastCost() CostEstimate {
	if cr, ok := m.Inner.(CostReporter); ok {
		return cr.LastCost()
	}
	return CostEstimate{}
}

// SlogChargeLogger is the default ChargeLogger — emits one structured
// line per AI call so operators can tail the dollar spend live. The
// runner installs one of these by default; tests substitute their own.
type SlogChargeLogger struct {
	// Writer is the optional sink. nil prints to stderr via slog.
	Writer ChargeLineWriter
}

// ChargeLineWriter is the surface the slog logger writes through.
// Decoupled so a CLI can install a styled progress sink.
type ChargeLineWriter interface {
	WriteCharge(line string)
}

// Charge formats the line and forwards it.
func (s *SlogChargeLogger) Charge(nodeID, model string, cost CostEstimate) {
	if s == nil || s.Writer == nil {
		return
	}
	s.Writer.WriteCharge(fmt.Sprintf("$%.4f  %d tok  node=%s model=%s", cost.Dollars, cost.Tokens, nodeID, model))
}
