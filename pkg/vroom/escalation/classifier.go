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
//
// Its high-stakes boundary is DefaultApprovalPolicy — the declarative taxonomy
// (see policy.go). To override the boundary from a .safe-merge.yml approval_policy
// block, use PolicyClassifier instead; DefaultClassifier is the built-in baseline.
type DefaultClassifier struct{}

// Name identifies the classifier in audit records.
func (DefaultClassifier) Name() string { return "default" }

// PolicyClassifier is a DefaultClassifier whose must-reach-human boundary is
// supplied by a loaded ApprovalPolicy (typically from .safe-merge.yml) rather
// than the built-in default. The routing logic is identical; only the taxonomy
// of human-required categories differs. This is the seam the bead asks for:
// "express the taxonomy in .safe-merge.yml policy + the VROOM gate".
type PolicyClassifier struct {
	policy *CompiledApprovalPolicy
}

// NewPolicyClassifier compiles p and returns a classifier that gates on it.
func NewPolicyClassifier(p ApprovalPolicy) (*PolicyClassifier, error) {
	cp, err := p.Compile()
	if err != nil {
		return nil, err
	}
	return &PolicyClassifier{policy: cp}, nil
}

// Name identifies the classifier in audit records.
func (*PolicyClassifier) Name() string { return "policy" }

// Classify implements Classifier using the loaded policy's taxonomy.
func (c *PolicyClassifier) Classify(_ context.Context, e *Escalation) (Verdict, error) {
	return classifyWith(c.policy, e), nil
}

var _ Classifier = (*PolicyClassifier)(nil)

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

// Classify implements Classifier, gating on the built-in DefaultApprovalPolicy.
func (DefaultClassifier) Classify(_ context.Context, e *Escalation) (Verdict, error) {
	return classifyWith(defaultCompiledPolicy, e), nil
}

// classifyWith is the shared routing logic for DefaultClassifier and
// PolicyClassifier. The only thing that varies between them is policy — the
// taxonomy of human-required categories. Everything else (proceed auto-resolve,
// decision routing, supervisor fallthrough) is identical, so a custom policy
// changes the boundary without forking the routing logic.
func classifyWith(policy *CompiledApprovalPolicy, e *Escalation) Verdict {
	q := e.Question

	// High-stakes always wins, regardless of phrasing: a "should I proceed"
	// dressed over a product decision still must reach the human.
	if topic, reason, required := policy.RequiresHuman(q); required {
		return Verdict{
			Disposition:    DispRouteToHuman,
			MustReachHuman: true,
			Topic:          topic,
			Reason:         reason,
		}
	}

	// Decisions never auto-resolve — a judgment call is, by definition, not
	// obvious — but absent a high-stakes marker they route to the supervisor.
	if e.Kind == KindDecision {
		return Verdict{
			Disposition: DispRouteToSupervisor,
			Topic:       "decision",
			Reason:      "judgment call with no high-stakes marker; supervisor decides",
		}
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
		}
	}

	return Verdict{
		Disposition: DispRouteToSupervisor,
		Topic:       topicHint(q),
		Reason:      "no auto-answer and no high-stakes marker; supervisor decides",
	}
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
