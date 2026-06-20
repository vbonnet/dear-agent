package escalation

import (
	"context"
	"regexp"
	"strings"
)

// Disposition is the classifier's verdict on how an escalation should be routed
// before any chain walking happens.
type Disposition string

const (
	// DispAutoResolve means the answer is obvious; the engine answers immediately
	// and the escalation never leaves the worker. The canonical case is "should I
	// do the task you assigned me?" → yes.
	DispAutoResolve Disposition = "auto-resolve"
	// DispRouteToSupervisor needs a human-or-supervisor judgment; route up the
	// spawn chain one hop at a time.
	DispRouteToSupervisor Disposition = "route-to-supervisor"
	// DispRouteToHuman is high-stakes; route up the chain but it must terminate
	// at the human — no node below the human may answer it.
	DispRouteToHuman Disposition = "route-to-human"
)

// Verdict is the full classifier output.
type Verdict struct {
	Disposition    Disposition
	MustReachHuman bool   // true ⇒ no non-human node may give a terminal answer
	Answer         string // populated only for DispAutoResolve
	Topic          string // coarse category for frequency analysis (may be "")
	Reason         string // human-readable explanation, recorded for audit
}

// Classifier decides how an escalation is routed. The default is deterministic;
// the interface is the seam for a future model-backed classifier (mirroring
// internal/override.Judge), so call sites and tests never change.
type Classifier interface {
	Classify(ctx context.Context, e *Escalation) (Verdict, error)
}

// DefaultClassifier is a deterministic, offline classifier. It is intentionally
// conservative: it auto-resolves only the narrow, unambiguous "proceed with the
// assigned task" pattern, and it flags must-reach-human on any high-stakes
// marker. Everything else routes to the supervisor, where a live agent applies
// judgment. Being deterministic, it is safe in CI and fully testable.
type DefaultClassifier struct{}

// Name identifies the classifier in audit records.
func (DefaultClassifier) Name() string { return "default" }

// proceedRe matches "may I proceed with the work I was told to do" questions:
// an optional polite lead-in, then a proceed verb, with no high-stakes marker
// (checked separately). Anchored loosely because workers phrase this many ways.
var proceedRe = regexp.MustCompile(
	`(?i)\b(should|shall|can|may|ok(ay)? (for me|to)|is it (ok|fine|alright)).{0,40}?\b(proceed|continue|go ahead|start|begin|execute|carry on|run|do)\b`,
)

// assignedRe is a second, stricter signal: the worker explicitly references the
// task it was *given* ("the task you asked me to do", "my assigned task").
var assignedRe = regexp.MustCompile(
	`(?i)\b(the |my )?(task|work|job|thing)\b.{0,30}?\b(you (asked|told|assigned|gave)|assigned|i was (asked|told|assigned|given))\b`,
)

// highStakesMarkers force must-reach-human. These are the categories a worker —
// or even an intermediate supervisor — must not decide unilaterally. Matched as
// whole-word/substring, case-insensitive. Kept explicit (not an LLM) so the
// boundary is auditable and stable.
var highStakesMarkers = []struct {
	pat   *regexp.Regexp
	topic string
}{
	{regexp.MustCompile(`(?i)\bproduct (decision|direction|strategy)\b`), "product"},
	{regexp.MustCompile(`(?i)\b(pricing|price|paywall|monetiz)`), "pricing"},
	{regexp.MustCompile(`(?i)\b(roadmap|prioriti[sz]e|deprioriti[sz]e|strategy|strategic)\b`), "strategy"},
	{regexp.MustCompile(`(?i)\b(publish|release publicly|go public|announce|tweet|post to)\b`), "publishing"},
	{regexp.MustCompile(`(?i)\b(delete|drop|wipe|destroy|purge)\b.{0,20}\b(prod|production|database|customer|user data)\b`), "destructive"},
	{regexp.MustCompile(`(?i)\b(irreversible|cannot be undone|permanent(ly)? (delete|remove))\b`), "irreversible"},
	{regexp.MustCompile(`(?i)\b(legal|compliance|gdpr|license|licensing|contract|terms of service)\b`), "legal"},
	{regexp.MustCompile(`(?i)\b(security policy|disable (the )?(guard|gate|auth|2fa|mfa)|rotate.{0,15}key|exfiltrat)`), "security"},
	{regexp.MustCompile(`(?i)\b(spend|budget|purchase|pay|invoice|\$\d)`), "spend"},
	{regexp.MustCompile(`(?i)\b(hire|fire|headcount|terminate (an?|the) (employee|contractor))\b`), "people"},
	{regexp.MustCompile(`(?i)\b(email|message|contact|reach out to) (the )?(customer|client|user|press|investor)`), "external-comms"},
}

// Classify implements Classifier.
func (DefaultClassifier) Classify(_ context.Context, e *Escalation) (Verdict, error) {
	q := e.Question

	// High-stakes always wins, regardless of phrasing: a "should I proceed"
	// dressed over a product decision still must reach the human.
	if topic, ok := matchHighStakes(q); ok {
		return Verdict{
			Disposition:    DispRouteToHuman,
			MustReachHuman: true,
			Topic:          topic,
			Reason:         "matched high-stakes marker (" + topic + "); only the human may decide this",
		}, nil
	}

	// Decisions never auto-resolve — a judgment call is, by definition, not
	// obvious — but absent a high-stakes marker they route to the supervisor.
	if e.Kind == KindDecision {
		return Verdict{
			Disposition: DispRouteToSupervisor,
			Topic:       "decision",
			Reason:      "judgment call with no high-stakes marker; supervisor decides",
		}, nil
	}

	// Auto-resolve the narrow "proceed with the assigned task" pattern. Requires
	// either an explicit proceed-verb question or an explicit reference to the
	// assigned task. This is the case the spec calls out as having an obvious
	// answer that must never reach a human.
	if e.Kind == KindQuestion && (proceedRe.MatchString(q) || assignedRe.MatchString(q)) {
		return Verdict{
			Disposition: DispAutoResolve,
			Answer:      "Yes — proceed. This is the task you were assigned; you do not need approval to do the work you were dispatched to do.",
			Topic:       "proceed-confirmation",
			Reason:      "self-evident confirmation of the assigned task",
		}, nil
	}

	return Verdict{
		Disposition: DispRouteToSupervisor,
		Topic:       topicHint(q),
		Reason:      "no auto-answer and no high-stakes marker; supervisor decides",
	}, nil
}

func matchHighStakes(q string) (string, bool) {
	for _, m := range highStakesMarkers {
		if m.pat.MatchString(q) {
			return m.topic, true
		}
	}
	return "", false
}

// topicHint produces a coarse, cheap category for frequency analysis when no
// stronger signal exists. It is best-effort; the authoritative grouping key for
// analysis is question_hash (see EscalationEvent).
func topicHint(q string) string {
	lq := strings.ToLower(q)
	switch {
	case strings.Contains(lq, "test"):
		return "testing"
	case strings.Contains(lq, "merge") || strings.Contains(lq, "pr") || strings.Contains(lq, "pull request"):
		return "pr-merge"
	case strings.Contains(lq, "api"):
		return "api"
	case strings.Contains(lq, "permission") || strings.Contains(lq, "access") || strings.Contains(lq, "allow"):
		return "permissions"
	case strings.Contains(lq, "which") || strings.Contains(lq, "what") || strings.Contains(lq, "how"):
		return "how-to"
	default:
		return "general"
	}
}

var _ Classifier = DefaultClassifier{}
