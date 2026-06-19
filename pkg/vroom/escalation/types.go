package escalation

import "time"

// Kind classifies what is being escalated. It drives both routing and
// after-the-fact analysis (group escalations by kind to see what agents most
// often get stuck on).
type Kind string

const (
	// KindQuestion is a factual or how-to question with a knowable answer
	// ("which API version should I target?").
	KindQuestion Kind = "question"
	// KindDecision is a judgment call ("should we make this product decision?").
	// Decisions are the kind most likely to be flagged must-reach-human.
	KindDecision Kind = "decision"
	// KindBlockedAction is the ADR-031 case: an action with no approved path
	// ("I need to force-push but the gate forbids it").
	KindBlockedAction Kind = "blocked-action"
)

// Valid reports whether k is a recognised kind.
func (k Kind) Valid() bool {
	switch k {
	case KindQuestion, KindDecision, KindBlockedAction:
		return true
	default:
		return false
	}
}

// Mode selects whether the *asking worker* waits for the answer. Supervisors are
// never blocked regardless of mode; see the package doc.
type Mode string

const (
	// ModeBlocking means the asking worker calls Await after Raise and waits
	// (up to a timeout) for the escalation to resolve.
	ModeBlocking Mode = "blocking"
	// ModeAsync means the worker continues immediately; the resolution is
	// delivered back to its session when it lands.
	ModeAsync Mode = "async"
)

// Valid reports whether m is a recognised mode.
func (m Mode) Valid() bool { return m == ModeBlocking || m == ModeAsync }

// Phase is the lifecycle state of an escalation. Transitions are append-only in
// the event log; the Store holds the current Phase.
type Phase string

const (
	// PhaseRaised: the worker created the escalation.
	PhaseRaised Phase = "raised"
	// PhaseClassified: the classifier rendered a disposition.
	PhaseClassified Phase = "classified"
	// PhaseRouted: delivered to the next node up the chain.
	PhaseRouted Phase = "routed"
	// PhaseConferring: a VROOM supervisor is conferring with its peers.
	PhaseConferring Phase = "conferring"
	// PhaseDispatchedToHuman: surfaced to the human via Dispatch.
	PhaseDispatchedToHuman Phase = "dispatched-to-human"
	// PhaseAnswered: a node (or the human) gave a terminal answer.
	PhaseAnswered Phase = "answered"
	// PhaseAutoResolved: the classifier answered it without routing.
	PhaseAutoResolved Phase = "auto-resolved"
	// PhaseTimedOut: a blocking Await gave up; the escalation remains in-flight
	// but the worker stopped waiting.
	PhaseTimedOut Phase = "timed-out"
	// PhaseDismissed: closed without an answer (e.g. obsolete).
	PhaseDismissed Phase = "dismissed"
)

// Terminal reports whether the phase is an end state for the escalation itself
// (PhaseTimedOut is not terminal — only the worker's wait ended).
func (p Phase) Terminal() bool {
	switch p {
	case PhaseAnswered, PhaseAutoResolved, PhaseDismissed:
		return true
	default:
		return false
	}
}

// NodeKind describes a node's position in the spawn hierarchy, which decides how
// the engine routes from it.
type NodeKind string

const (
	// NodeWorker is an ordinary spawned agent.
	NodeWorker NodeKind = "worker"
	// NodeSupervisor is a non-VROOM agent that spawned children (an
	// intermediate manager).
	NodeSupervisor NodeKind = "supervisor"
	// NodeVROOMMetaOrchestrator / NodeVROOMOrchestrator / NodeVROOMOverseer are
	// the three apex supervisors. Reaching any of them arms the confer→human
	// terminus.
	NodeVROOMMetaOrchestrator NodeKind = "vroom-meta-orchestrator"
	NodeVROOMOrchestrator     NodeKind = "vroom-orchestrator"
	NodeVROOMOverseer         NodeKind = "vroom-overseer"
	// NodeNone means "no parent" (a root session).
	NodeNone NodeKind = "none"
	// NodeHuman is the terminal authority.
	NodeHuman NodeKind = "human"
)

// IsVROOM reports whether the node is one of the three apex supervisors.
func (n NodeKind) IsVROOM() bool {
	switch n {
	case NodeVROOMMetaOrchestrator, NodeVROOMOrchestrator, NodeVROOMOverseer:
		return true
	default:
		return false
	}
}

// ParentRef identifies the session a given session should escalate to.
type ParentRef struct {
	SessionID string
	Role      string   // free-form role label (e.g. "coder", "orchestrator")
	Kind      NodeKind // position in the hierarchy
}

// Escalation is the mutable record of one in-flight (or resolved) escalation.
// It is what the Store persists; the event log derives from its transitions.
type Escalation struct {
	ID   string `json:"id"`
	Kind Kind   `json:"kind"`
	Mode Mode   `json:"mode"`

	// Question is the verbatim text the worker raised. For KindDecision it is
	// the decision being asked; for KindBlockedAction it is "<action>: <reason>".
	Question string `json:"question"`
	// Context is optional supporting detail the worker supplied.
	Context string `json:"context,omitempty"`

	// Origin is the worker that raised the escalation.
	OriginSessionID string `json:"origin_session_id"`
	OriginRole      string `json:"origin_role,omitempty"`

	// Current holds where the escalation sits right now (the node that must act).
	CurrentSessionID string   `json:"current_session_id"`
	CurrentRole      string   `json:"current_role,omitempty"`
	CurrentKind      NodeKind `json:"current_kind,omitempty"`

	// Chain is the ordered list of session IDs the escalation has visited,
	// starting with the origin. Used for loop detection and audit.
	Chain []string `json:"chain"`

	Phase Phase `json:"phase"`

	// Disposition / MustReachHuman are the classifier's output, retained so a
	// supervisor (and the audit log) can see why routing went the way it did.
	Disposition      Disposition `json:"disposition"`
	MustReachHuman   bool        `json:"must_reach_human"`
	ClassifierReason string      `json:"classifier_reason,omitempty"`
	Topic            string      `json:"topic,omitempty"`

	// Answer / answerer are set on resolution.
	Answer         string `json:"answer,omitempty"`
	AnsweredBy     string `json:"answered_by,omitempty"`
	AnsweredByRole string `json:"answered_by_role,omitempty"`

	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	ResolvedAt time.Time `json:"resolved_at,omitempty"`
}

// resolved reports whether the escalation has reached a terminal phase.
func (e *Escalation) resolved() bool { return e.Phase.Terminal() }
