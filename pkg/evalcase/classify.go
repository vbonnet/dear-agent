package evalcase

import (
	"fmt"
	"sort"

	"github.com/vbonnet/dear-agent/pkg/agenttrace"
)

// FailureClass labels why a trace is problematic. A trace can match several; the
// Verdict's Primary is the most specific one (see classPriority).
type FailureClass string

const (
	// ClassNone means the trace is not problematic.
	ClassNone FailureClass = ""
	// ClassToolError — a tool-call span errored (pillar 1).
	ClassToolError FailureClass = "tool_error"
	// ClassReasoningError — a reasoning span errored (pillar 2).
	ClassReasoningError FailureClass = "reasoning_error"
	// ClassStateLoss — a state-transition span errored (pillar 3).
	ClassStateLoss FailureClass = "state_loss"
	// ClassMemoryError — a memory span errored or returned a stale/low-relevance
	// read (pillar 4).
	ClassMemoryError FailureClass = "memory_error"
	// ClassStall — the agent got stuck: a stalled outcome or a tool retried past
	// the configured ceiling.
	ClassStall FailureClass = "stall"
	// ClassLowEvalScore — an attached eval score fell below threshold.
	ClassLowEvalScore FailureClass = "low_eval_score"
	// ClassErrorOutcome — the run ended in an error outcome with no more specific
	// span-level cause found.
	ClassErrorOutcome FailureClass = "error_outcome"
)

// classPriority orders failure classes from most to least specific. Extract uses
// it to pick the Primary class so a span-level cause (a concrete errored tool)
// wins over a generic whole-run signal (an error outcome).
var classPriority = []FailureClass{
	ClassToolError,
	ClassReasoningError,
	ClassStateLoss,
	ClassMemoryError,
	ClassStall,
	ClassLowEvalScore,
	ClassErrorOutcome,
}

// ClassifierConfig holds the thresholds that decide whether a trace is
// problematic. The zero value is not meaningful — use DefaultClassifierConfig
// and override fields as needed.
type ClassifierConfig struct {
	// MinEvalScore: an attached eval score strictly below this is a failure.
	MinEvalScore float64
	// MaxToolRetries: a tool-call span whose retry_count is >= this counts as a
	// stall (a silent retry loop). Zero disables the retry-based stall check.
	MaxToolRetries int
	// MinMemoryRelevance: a memory read/query whose relevance_score is strictly
	// below this is a memory error (wrong-entity / stale retrieval). Zero
	// disables the relevance check.
	MinMemoryRelevance float64
}

// DefaultClassifierConfig returns sensible thresholds for AGM session traces.
func DefaultClassifierConfig() ClassifierConfig {
	return ClassifierConfig{
		MinEvalScore:       0.5,
		MaxToolRetries:     3,
		MinMemoryRelevance: 0.2,
	}
}

// Verdict is the classifier's decision about a trace.
type Verdict struct {
	// Problematic is true when at least one failure class matched.
	Problematic bool `json:"problematic"`
	// Primary is the most specific matched class (ClassNone if not problematic).
	Primary FailureClass `json:"primary"`
	// Classes are all matched classes, ordered by specificity.
	Classes []FailureClass `json:"classes,omitempty"`
	// Reasons are human-readable explanations, one or more per matched class.
	Reasons []string `json:"reasons,omitempty"`
}

// Classify inspects a completed trace and returns a Verdict. A trace is
// problematic if any pillar span errored, an eval score is below threshold, a
// memory read is low-relevance, or the outcome is error/stalled. A clean,
// successful trace yields a non-problematic Verdict (Primary == ClassNone).
func (c ClassifierConfig) Classify(t Trace) Verdict {
	matched := map[FailureClass]bool{}
	var reasons []string

	add := func(class FailureClass, reason string) {
		matched[class] = true
		reasons = append(reasons, reason)
	}

	// Pillar span errors + per-pillar heuristics.
	for _, s := range t.Spans {
		switch s.Pillar {
		case PillarToolCall:
			if s.failed() {
				add(ClassToolError, fmt.Sprintf("tool call %q errored: %s", s.str(agenttrace.AttrToolName), s.errDesc()))
			}
			if c.MaxToolRetries > 0 {
				if n, ok := s.num(agenttrace.AttrToolRetryCount); ok && int(n) >= c.MaxToolRetries {
					add(ClassStall, fmt.Sprintf("tool call %q retried %d times (>= %d)", s.str(agenttrace.AttrToolName), int(n), c.MaxToolRetries))
				}
			}
		case PillarReasoning:
			if s.failed() {
				add(ClassReasoningError, fmt.Sprintf("reasoning step %q errored: %s", s.str(agenttrace.AttrReasoningStep), s.errDesc()))
			}
		case PillarStateTransition:
			if s.failed() {
				add(ClassStateLoss, fmt.Sprintf("state transition %s→%s errored: %s", s.str(agenttrace.AttrStateFrom), s.str(agenttrace.AttrStateTo), s.errDesc()))
			}
		case PillarMemory:
			if s.failed() {
				add(ClassMemoryError, fmt.Sprintf("memory %s on %q errored: %s", s.str(agenttrace.AttrMemoryOperation), s.str(agenttrace.AttrMemoryStore), s.errDesc()))
			}
			if c.MinMemoryRelevance > 0 && isMemoryRead(s) {
				if rel, ok := s.num(agenttrace.AttrMemoryRelevance); ok && rel < c.MinMemoryRelevance {
					add(ClassMemoryError, fmt.Sprintf("memory %s on %q returned low relevance %.3f (< %.3f)", s.str(agenttrace.AttrMemoryOperation), s.str(agenttrace.AttrMemoryStore), rel, c.MinMemoryRelevance))
				}
			}
		}
	}

	// Eval scores below threshold.
	for name, score := range t.EvalScores {
		if score < c.MinEvalScore {
			add(ClassLowEvalScore, fmt.Sprintf("eval %q scored %.3f (< %.3f)", name, score, c.MinEvalScore))
		}
	}

	// Terminal outcome.
	switch t.Outcome {
	case OutcomeStalled:
		add(ClassStall, "run outcome was stalled")
	case OutcomeError:
		// Only surface the generic error-outcome class if nothing more specific
		// already explained the failure.
		if !matched[ClassToolError] && !matched[ClassReasoningError] &&
			!matched[ClassStateLoss] && !matched[ClassMemoryError] {
			add(ClassErrorOutcome, "run outcome was error")
		}
	}

	if len(matched) == 0 {
		return Verdict{Problematic: false, Primary: ClassNone}
	}

	classes := make([]FailureClass, 0, len(matched))
	for _, class := range classPriority {
		if matched[class] {
			classes = append(classes, class)
		}
	}
	// Stable, readable reason ordering.
	sort.Strings(reasons)
	return Verdict{
		Problematic: true,
		Primary:     classes[0],
		Classes:     classes,
		Reasons:     reasons,
	}
}

// errDesc renders a span's error for a reason string.
func (s Span) errDesc() string {
	if s.ErrorType != "" {
		return s.ErrorType
	}
	return "error status"
}

// isMemoryRead reports whether a memory span is a read or query (operations
// where relevance is meaningful), as opposed to a write.
func isMemoryRead(s Span) bool {
	switch s.str(agenttrace.AttrMemoryOperation) {
	case string(agenttrace.MemoryRead), string(agenttrace.MemoryQuery):
		return true
	default:
		return false
	}
}
