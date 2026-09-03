package mergeloop

import "github.com/vbonnet/dear-agent/internal/agenticreview"

// agenticReviewGate maps the per-family review verdict onto a merge-loop state.
//
// The gate is also a required status check, so in the normal case the loop
// would see it through requiredVerdict like any other check. Evaluating it here
// as well is deliberate: the loop is what actually calls safe-merge, and a
// required check that has not been added to the ruleset yet, or a projection
// that came back empty, must not be the only thing standing between an
// unreviewed head and main. The two paths read the same labels through the same
// policy file, so they cannot disagree about what the reviewers said.
//
// The mapping distinguishes who can fix each outcome:
//
//   - Still resolving: wait. Waiting is not stalling, and this is the window
//     the gate exists to close.
//   - A family requested changes: a repair agent can address the finding, so it
//     is routed like a failing check, repair budget included.
//   - Quorum not reached because reviewers are down: no code change fixes a
//     reviewer that cannot run, so it escalates to a human.
func (p Policy) agenticReviewGate(pr PR, attempts int) (Classification, bool) {
	cfg := p.AgenticReview
	if cfg == nil {
		return Classification{}, false
	}

	// Without a clock the gate cannot age a silent reviewer out, and guessing
	// would mean either merging on evidence that was never checked or
	// declaring a live reviewer dead. Waiting is the only honest answer.
	if pr.ObservedAt.IsZero() {
		return Classification{StateCIPending, "agentic review gate: no observation time recorded for this pass"}, true
	}

	verdict, err := cfg.Evaluate(agenticreview.Input{
		Labels:    pr.Labels,
		AppliedAt: pr.LabelAppliedAt,
		ReadyAt:   pr.ReadyAt,
		Now:       pr.ObservedAt,
	})
	if err != nil {
		return Classification{StateBlockedPolicy, "agentic review gate misconfigured: " + err.Error()}, true
	}

	switch verdict.Decision {
	case agenticreview.DecisionPass:
		return Classification{}, false
	case agenticreview.DecisionPending:
		return Classification{StateCIPending, "agentic review gate: " + verdict.Reason}, true
	case agenticreview.DecisionBlock:
		return p.blockedReview(verdict, attempts), true
	default:
		// An unrecognized decision is not a pass. The gate is the last thing
		// between an unreviewed head and main, so anything it cannot interpret
		// escalates rather than falling through to a merge.
		return Classification{StateBlockedPolicy,
			"agentic review gate returned an unrecognized decision " + string(verdict.Decision)}, true
	}
}

// blockedReview splits a blocking verdict into the repairable case and the
// human case, and stops respawning a repair agent once the same finding has
// exhausted the repair budget.
func (p Policy) blockedReview(verdict agenticreview.Verdict, attempts int) Classification {
	for _, fv := range verdict.Families {
		if fv.State != agenticreview.StateBlocking {
			continue
		}
		if attempts >= p.maxAttempts() {
			return Classification{StateAbandoned, "agentic review gate: " + verdict.Reason +
				" after " + itoa(attempts) + " agent fix attempt(s); needs human"}
		}
		return Classification{StateCIFailing, "agentic review gate: " + verdict.Reason}
	}
	return Classification{StateBlockedPolicy, "agentic review gate: " + verdict.Reason +
		" (reviewer availability needs a human, not a code fix)"}
}
