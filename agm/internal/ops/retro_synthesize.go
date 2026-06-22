package ops

import (
	"fmt"
	"strings"
)

// RetroSynthesis is the supervisor's cross-lens verdict.
//
// Each of the four lenses (root-cause, recurrence, remediation, systemic) answers
// one question in isolation. On their own they are four disconnected facts; a
// human still has to read all four and decide how much the retro matters and what
// to do next. Synthesis is the supervisor step that does that reasoning: it folds
// the four findings into a single prioritized, actionable decision. This is the
// layer that makes the multi-lens split worthwhile rather than just verbose.
//
// Synthesis is only produced when all four lenses ran (the supervisor needs every
// lens to reason across them); single-lens analyses leave it nil.
type RetroSynthesis struct {
	// Priority is the supervisor's triage call: P1 (act now), P2 (schedule),
	// P3 (note and move on). Derived from severity Score.
	Priority string `json:"priority"`
	// Score is the combined severity, 0–4: recurrence score (0–2) plus a
	// systemic weight (systemic=2, indeterminate=1, one-off=0).
	Score int `json:"score"`
	// Verdict is a one-line headline combining the recurrence/classification
	// framing with the root-cause summary.
	Verdict string `json:"verdict"`
	// Confidence reflects how strong the underlying signals are: high|medium|low.
	Confidence string `json:"confidence"`
	// KeyActions are the most actionable remediation items, tracked ones
	// (bead/PR-tagged) first, capped to keep the supervisor output scannable.
	KeyActions []string `json:"key_actions,omitempty"`
	// Gaps are issues the supervisor flags by reading lenses against each other
	// (e.g. a recurring systemic issue with no tracked follow-up).
	Gaps []string `json:"gaps,omitempty"`
	// Rationale explains how Priority was derived.
	Rationale string `json:"rationale"`
}

// keyActionLimit caps how many remediation actions the synthesis surfaces so the
// supervisor verdict stays a short decision, not a full dump of the lens.
const keyActionLimit = 3

// synthesizeLenses folds the four lens findings into one prioritized verdict.
// Returns nil unless all four lenses are present — the supervisor reasons across
// every lens, so a partial set has nothing to synthesize.
func synthesizeLenses(l *RetroLenses) *RetroSynthesis {
	if l.RootCause == nil || l.Recurrence == nil || l.Remediation == nil || l.Systemic == nil {
		return nil
	}

	s := &RetroSynthesis{}

	// Severity: recurrence score (how often it happens) plus a systemic weight
	// (how structural it is). Both axes push priority up.
	sysWeight := systemicWeight(l.Systemic.Classification)
	s.Score = l.Recurrence.Score + sysWeight
	s.Priority = priorityForScore(s.Score)
	s.Confidence = synthConfidence(l)
	s.KeyActions = topActions(l.Remediation.Actions, keyActionLimit)
	s.Verdict = synthVerdict(l)
	s.Gaps = synthGaps(l, s.Priority)
	s.Rationale = fmt.Sprintf(
		"severity score %d/4 (recurrence %d + systemic weight %d); recurrence=%s, classification=%s",
		s.Score, l.Recurrence.Score, sysWeight, l.Recurrence.Label, l.Systemic.Classification)

	return s
}

// systemicWeight maps a systemic classification to its contribution to severity.
func systemicWeight(classification string) int {
	switch classification {
	case "systemic":
		return 2
	case "indeterminate":
		return 1
	default: // one-off
		return 0
	}
}

// priorityForScore turns the 0–4 severity score into a triage priority.
//
//	4         → P1  (recurring AND systemic — the worst quadrant)
//	2–3       → P2  (structural OR repeating, but not both at full strength)
//	0–1       → P3  (novel and/or one-off)
func priorityForScore(score int) string {
	switch {
	case score >= 4:
		return "P1"
	case score >= 2:
		return "P2"
	default:
		return "P3"
	}
}

// synthConfidence judges how much to trust the verdict from the strength of the
// recurrence and systemic signals.
func synthConfidence(l *RetroLenses) string {
	switch {
	case l.Systemic.Classification == "indeterminate":
		return "low"
	case len(l.Recurrence.Signals) >= 2 || len(l.Recurrence.PriorInstances) >= 2:
		return "high"
	case len(l.Recurrence.Signals) >= 1 || len(l.Recurrence.PriorInstances) >= 1:
		return "medium"
	default:
		return "low"
	}
}

// topActions returns the most useful remediation actions, tracked ones (carrying
// a bead or PR) first since those are already actionable, capped at n.
func topActions(actions []RemediationAction, n int) []string {
	var tracked, untracked []string
	for _, a := range actions {
		label := a.Description
		var tags []string
		if a.Bead != "" {
			tags = append(tags, a.Bead)
		}
		if a.PR != "" {
			tags = append(tags, a.PR)
		}
		if len(tags) > 0 {
			tracked = append(tracked, "["+strings.Join(tags, " ")+"] "+label)
		} else {
			untracked = append(untracked, label)
		}
	}
	out := make([]string, 0, len(tracked)+len(untracked))
	out = append(out, tracked...)
	out = append(out, untracked...)
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// hasTrackedAction reports whether any remediation action carries a bead or PR.
func hasTrackedAction(actions []RemediationAction) bool {
	for _, a := range actions {
		if a.Bead != "" || a.PR != "" {
			return true
		}
	}
	return false
}

// synthVerdict builds the one-line headline: a recurrence/classification framing
// plus the first sentence of the root-cause summary.
func synthVerdict(l *RetroLenses) string {
	adj := map[string]string{"novel": "novel", "seen-once": "previously-seen", "recurring": "recurring"}[l.Recurrence.Label]
	if adj == "" {
		adj = l.Recurrence.Label
	}
	noun := "issue"
	switch l.Systemic.Classification {
	case "systemic":
		noun = "systemic issue"
	case "one-off":
		noun = "one-off issue"
	}

	headline := strings.TrimSpace(adj + " " + noun)
	headline = strings.ToUpper(headline[:1]) + headline[1:]

	if summary := firstSentence(l.RootCause.Summary); summary != "" {
		headline += ": " + summary
	}
	return headline
}

// synthGaps flags cross-lens inconsistencies the supervisor should surface — the
// kind of thing only visible once the lenses are read against one another.
func synthGaps(l *RetroLenses, priority string) []string {
	var gaps []string
	if priority != "P3" && len(l.Remediation.Actions) == 0 {
		gaps = append(gaps, "high-priority issue with no recorded follow-up actions")
	}
	if l.Systemic.Classification == "systemic" && len(l.RootCause.Chain) == 0 {
		gaps = append(gaps, "classified systemic but no root-cause chain was extracted")
	}
	if l.Recurrence.Score == 2 && len(l.Remediation.Actions) > 0 && !hasTrackedAction(l.Remediation.Actions) {
		gaps = append(gaps, "recurring pattern with no bead/PR-tracked remediation")
	}
	return gaps
}

// firstSentence returns the first sentence of s, trimmed to a readable length so
// the verdict stays a one-liner.
func firstSentence(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return ""
	}
	if i := strings.Index(s, ". "); i >= 0 {
		s = s[:i+1]
	}
	const maxLen = 160
	if len(s) > maxLen {
		s = strings.TrimSpace(s[:maxLen]) + "…"
	}
	return s
}
