package supervisor

import (
	"context"
	"errors"
	"fmt"

	"github.com/vbonnet/dear-agent/pkg/vroom/escalation"
)

// EscalationDrainer is the Loop's per-tick escalation hook. The Loop calls it
// once every iteration so a supervisor answers or forwards the escalations it
// currently holds as part of its own job — rather than relying on a live agent
// remembering to run `agm escalate list --mine --pending` by hand each tick
// (ADR-032 follow-up, ce-xshb).
//
// It is an optional Loop dependency: a nil drainer keeps the legacy behaviour
// (a live agent drains escalations manually). Draining is best-effort — the
// Loop records the result to the decision trail and never lets a drain error
// abort the iteration, exactly as it treats a failed Tick.
type EscalationDrainer interface {
	// Drain processes the escalations currently held by this supervisor and
	// reports a per-pass summary. It returns the first per-escalation error
	// encountered (if any) but always continues to the next escalation: one
	// wedged escalation must not stall the rest of the inbox or the loop.
	Drain(ctx context.Context) (DrainResult, error)
}

// EscalationInbox is the slice of the escalation engine an InboxDrainer needs:
// list the escalations a supervisor currently holds, and resolve each by
// answering or forwarding it. *escalation.Engine satisfies it; defining it as a
// narrow interface keeps the drainer testable with a fake.
type EscalationInbox interface {
	List(ctx context.Context, f escalation.Filter) ([]*escalation.Escalation, error)
	Answer(ctx context.Context, id, bySessionID, byRole, answer string) (*escalation.Escalation, error)
	Forward(ctx context.Context, id, fromSessionID, fromRole, note string) (*escalation.Escalation, error)
}

// DrainAction is what a DrainPolicy decides to do with one held escalation.
type DrainAction int

const (
	// DrainForward routes the escalation one hop further up the chain. It is the
	// zero value and the safe default: a programmatic supervisor that cannot
	// author a confident answer forwards instead, so the escalation still
	// reaches a node (the VROOM trio, ultimately the human) that can resolve it.
	DrainForward DrainAction = iota
	// DrainAnswer resolves the escalation with a terminal answer. The engine
	// refuses this for must-reach-human escalations, so a policy that returns it
	// for one of those will see the answer counted as a failure, not applied.
	DrainAnswer
	// DrainSkip leaves the escalation in place for a later tick (e.g. it needs a
	// judgment this drainer cannot supply yet). It is not an error.
	DrainSkip
)

// DrainDecision pairs an action with the text it carries: the answer body for
// DrainAnswer, or the forwarding note for DrainForward. DrainSkip ignores Text.
type DrainDecision struct {
	Action DrainAction
	Text   string
}

// DrainPolicy decides what to do with one escalation a supervisor holds. It is
// the seam where answer-vs-forward judgment lives: the default
// (ForwardingDrainPolicy) forwards everything, while a smarter (e.g.
// live-agent-backed) policy can be injected to answer in-process.
type DrainPolicy func(ctx context.Context, e *escalation.Escalation) DrainDecision

// ForwardingDrainPolicy forwards every escalation one hop up with a uniform
// note. It is the conservative default for a programmatic supervisor that has
// no way to author a confident answer itself: nothing is lost (the chain still
// reaches a node that can answer) and must-reach-human routing is preserved
// (the policy never answers, so the engine's human-only guard is never tested).
func ForwardingDrainPolicy(note string) DrainPolicy {
	if note == "" {
		note = "auto-forwarded by supervisor loop: no in-process answer available"
	}
	return func(_ context.Context, _ *escalation.Escalation) DrainDecision {
		return DrainDecision{Action: DrainForward, Text: note}
	}
}

// DrainResult summarises one drain pass for the decision trail.
type DrainResult struct {
	Listed    int // escalations held by this supervisor at the start of the pass
	Answered  int // resolved with a terminal answer
	Forwarded int // routed one hop further up the chain
	Skipped   int // deliberately left in place by the policy
	Failed    int // List succeeded but the per-escalation action errored
}

// InboxDrainer drains a single supervisor's pending escalation inbox each tick,
// applying a DrainPolicy to every escalation it holds. It is the production
// EscalationDrainer (ce-xshb): the supervisor no longer depends on a live agent
// reading the protocol skill and running the CLI by hand.
type InboxDrainer struct {
	inbox     EscalationInbox
	sessionID string // this supervisor's own session — the "--mine" key
	role      string
	policy    DrainPolicy
}

// NewInboxDrainer validates its inputs and returns an InboxDrainer. sessionID is
// the supervisor's own session id (used both to select its inbox and as the
// answerer/forwarder identity); policy defaults to ForwardingDrainPolicy when
// nil.
func NewInboxDrainer(inbox EscalationInbox, sessionID, role string, policy DrainPolicy) (*InboxDrainer, error) {
	if inbox == nil {
		return nil, errors.New("supervisor: InboxDrainer inbox is nil")
	}
	if sessionID == "" {
		return nil, errors.New("supervisor: InboxDrainer sessionID is empty")
	}
	if policy == nil {
		policy = ForwardingDrainPolicy("")
	}
	return &InboxDrainer{inbox: inbox, sessionID: sessionID, role: role, policy: policy}, nil
}

// Drain lists the pending escalations this supervisor holds and applies the
// policy to each. Per-escalation errors are counted and the first is returned,
// but draining always proceeds to the next escalation.
func (d *InboxDrainer) Drain(ctx context.Context) (DrainResult, error) {
	var res DrainResult
	pending, err := d.inbox.List(ctx, escalation.Filter{Pending: true, CurrentSessionID: d.sessionID})
	if err != nil {
		return res, fmt.Errorf("supervisor: list escalations for %q: %w", d.sessionID, err)
	}
	res.Listed = len(pending)

	var firstErr error
	for _, esc := range pending {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		decision := d.policy(ctx, esc)
		var actErr error
		switch decision.Action {
		case DrainAnswer:
			if _, actErr = d.inbox.Answer(ctx, esc.ID, d.sessionID, d.role, decision.Text); actErr == nil || transitionCommitted(actErr) {
				res.Answered++
			}
		case DrainForward:
			if _, actErr = d.inbox.Forward(ctx, esc.ID, d.sessionID, d.role, decision.Text); actErr == nil || transitionCommitted(actErr) {
				res.Forwarded++
			}
		case DrainSkip:
			res.Skipped++
		default:
			// An unrecognised action (a policy bug) is treated as skip rather
			// than silently forwarding or answering: leave the escalation in
			// place so a later, correct pass can act on it.
			res.Skipped++
		}
		if actErr != nil {
			res.Failed++
			if firstErr == nil {
				firstErr = fmt.Errorf("supervisor: drain escalation %s: %w", esc.ID, actErr)
			}
		}
	}
	return res, firstErr
}

func transitionCommitted(err error) bool {
	var committed *escalation.CommittedEffectError
	return errors.As(err, &committed)
}

var _ EscalationDrainer = (*InboxDrainer)(nil)

// Engine satisfies EscalationInbox so an *escalation.Engine can back an
// InboxDrainer directly.
var _ EscalationInbox = (*escalation.Engine)(nil)
