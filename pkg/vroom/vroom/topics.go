package vroom

// VROOM decision event topics.
//
// Each topic corresponds to a consequential decision in the VROOM mesh. Per
// /docs/adr/ADR-002 (which supersedes the old agm/docs/adr/ADR-020), the
// canonical model has three supervisors — Meta-Orchestrator, Orchestrator,
// Overseer — with per-task Primary/Secondary/Tertiary ownership. "Evaluation"
// is a *Secondary responsibility*, not a standing role; the retired
// "Verifier" role is no longer referenced here.
//
// The richer append-only persistence lives in pkg/vroom/decisiontrail; these
// topics remain for the in-memory event bus surfaced via Emitter so existing
// consumers such as pkg/selfimprove keep working unchanged.
const (
	// TopicDecisionDispatched is emitted when the Orchestrator dispatches a task to a worker.
	TopicDecisionDispatched = "vroom.decision.dispatched"

	// TopicDecisionEscalated is emitted when the Overseer escalates an anomaly.
	TopicDecisionEscalated = "vroom.decision.escalated"

	// TopicDecisionEvaluated is emitted when an agent acting as Secondary
	// evaluates an output against values/invariants. (Previously attributed
	// to a "Verifier" standing role, now retired.)
	TopicDecisionEvaluated = "vroom.decision.evaluated"

	// TopicDecisionGated is emitted when the Meta-Orchestrator gates a state transition.
	TopicDecisionGated = "vroom.decision.gated"

	// TopicDecisionHandedOff is emitted when an agent serializes its
	// context for a successor (the /handoff pattern). It carries the
	// sender's confidence in that context so a low-confidence handoff
	// is a first-class, auditable decision rather than a silent one.
	TopicDecisionHandedOff = "vroom.decision.handed_off"
)
