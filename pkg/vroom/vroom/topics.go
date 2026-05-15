package vroom

// VROOM decision event topics.
//
// Each topic corresponds to a consequential decision made by a VROOM role,
// per ADR-020's decision trail specification.
const (
	// TopicDecisionDispatched is emitted when the Orchestrator dispatches a task to a worker.
	TopicDecisionDispatched = "vroom.decision.dispatched"

	// TopicDecisionEscalated is emitted when the Overseer escalates an anomaly.
	TopicDecisionEscalated = "vroom.decision.escalated"

	// TopicDecisionEvaluated is emitted when the Verifier evaluates an output against values/invariants.
	TopicDecisionEvaluated = "vroom.decision.evaluated"

	// TopicDecisionGated is emitted when the Meta-Orchestrator gates a state transition.
	TopicDecisionGated = "vroom.decision.gated"

	// TopicDecisionHandedOff is emitted when an agent serializes its
	// context for a successor (the /handoff pattern). It carries the
	// sender's confidence in that context so a low-confidence handoff
	// is a first-class, auditable decision rather than a silent one.
	TopicDecisionHandedOff = "vroom.decision.handed_off"
)
