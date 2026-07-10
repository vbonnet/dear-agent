package escalation

import (
	"context"
	"strings"
)

// Outcome is the quality verdict on an answered escalation, backfilled onto the
// EscalationEvent.Outcome column by the adjudicator pass. It is the column the
// first analysis groups on: incorrect/misaligned answers are
// `WHERE outcome IN ('incorrect', 'misaligned')`.
type Outcome string

const (
	// OutcomeCorrect means the answer is accurate and addresses the question.
	OutcomeCorrect Outcome = "correct"
	// OutcomeIncorrect means the answer is factually wrong or actively misleading.
	OutcomeIncorrect Outcome = "incorrect"
	// OutcomeMisaligned means the answer is not necessarily wrong but pulls the
	// agent away from intent — the case the analysis exists to surface (an answer
	// that technically resolves the ask while steering work off-course).
	OutcomeMisaligned Outcome = "misaligned"
	// OutcomeUnclear means the adjudicator looked but could not decide (ambiguous
	// question, answer it lacks the context to verify). Recorded so the event is
	// not re-adjudicated every pass, but excluded from the incorrect/misaligned
	// analysis.
	OutcomeUnclear Outcome = "unclear"
)

// Valid reports whether o is a recognised outcome.
func (o Outcome) Valid() bool {
	switch o {
	case OutcomeCorrect, OutcomeIncorrect, OutcomeMisaligned, OutcomeUnclear:
		return true
	default:
		return false
	}
}

// AdjudicationRequest is the input an Adjudicator scores: everything needed to
// judge whether an answer was good, with no live engine dependency so it can be
// reconstructed from a logged EscalationEvent.
type AdjudicationRequest struct {
	Kind           Kind
	Question       string
	Context        string
	Answer         string
	AnsweredByRole string
	Topic          string
}

// Adjudication is an Adjudicator's verdict.
type Adjudication struct {
	// Outcome is the verdict. An empty Outcome is the explicit "could not
	// assess" signal: the backfill leaves the event untouched (so a later pass
	// with a model wired in can still adjudicate it). A non-empty Outcome — even
	// OutcomeUnclear — is a recorded decision.
	Outcome Outcome
	// Misalignment is a short note on *how* an answer steered the agent wrong,
	// populated for OutcomeMisaligned (and optionally OutcomeIncorrect). It lands
	// in the EscalationEvent.Misalignment column.
	Misalignment string
	// Reason is a human-readable justification, for the audit trail.
	Reason string
}

// Adjudicator renders an after-the-fact quality verdict on an answered
// escalation. The default ([DefaultAdjudicator]) is deterministic and offline;
// [ModelAdjudicator] layers a model classifier on top. The interface is the
// same seam shape as internal/override.Judge — call sites and tests never change
// when the model layer is swapped in.
type Adjudicator interface {
	// Name identifies the adjudicator in the audit/log (e.g. "default", "openai").
	Name() string
	// Adjudicate returns a verdict. A non-nil error means the adjudicator could
	// not be consulted at all; the backfill skips the event and leaves it for a
	// later pass (it never invents an outcome on error).
	Adjudicate(ctx context.Context, req AdjudicationRequest) (Adjudication, error)
}

// DefaultAdjudicator is the deterministic, offline floor. Semantic correctness
// of a free-text answer genuinely cannot be judged without a model, so the
// default declines to render a verdict (empty Outcome) for any substantive
// answer — the honest "I can't assess this offline" result, which leaves the
// event for a later model pass rather than mislabelling it.
//
// It does catch the one case that *is* decidable without a model: an answer that
// is empty or a bare non-answer ("idk", "n/a") is marked incorrect — a
// supervisor that "answered" without answering.
type DefaultAdjudicator struct{}

// Name implements Adjudicator.
func (DefaultAdjudicator) Name() string { return "default" }

// nonAnswers are replies that close an escalation without actually answering it.
// Matched case-insensitively against the whole trimmed answer.
var nonAnswers = map[string]bool{
	"": true, "idk": true, "i don't know": true, "i dont know": true,
	"n/a": true, "na": true, "none": true, "no idea": true, "dunno": true,
	"not sure": true, "unsure": true, "?": true, "-": true,
}

// Adjudicate implements Adjudicator with deterministic heuristics only.
func (DefaultAdjudicator) Adjudicate(_ context.Context, req AdjudicationRequest) (Adjudication, error) {
	a := strings.ToLower(strings.TrimSpace(req.Answer))
	if nonAnswers[a] {
		return Adjudication{
			Outcome:      OutcomeIncorrect,
			Misalignment: "escalation was closed without a substantive answer",
			Reason:       "answer is empty or a bare non-answer",
		}, nil
	}
	// Anything substantive: the offline floor cannot judge correctness. Decline
	// (empty Outcome) so a later model pass can; do not guess.
	return Adjudication{
		Reason: "deterministic adjudicator cannot assess semantic correctness; " +
			"configure a model adjudicator to enable semantic scoring",
	}, nil
}

// Compile-time check.
var _ Adjudicator = DefaultAdjudicator{}
