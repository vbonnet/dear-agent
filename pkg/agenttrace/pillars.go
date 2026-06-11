package agenttrace

// Pillar identifies which of the four trace pillars a span belongs to. The
// trace→eval-case pipeline (pkg/evalcase) groups captured spans by pillar so it
// can excerpt the right context into a regression eval case.
type Pillar string

const (
	// PillarToolCall is the tool-call pillar (SpanToolCall).
	PillarToolCall Pillar = "tool_call"
	// PillarReasoning is the reasoning pillar (SpanReasoning).
	PillarReasoning Pillar = "reasoning"
	// PillarStateTransition is the state-transition pillar (SpanStateTransition).
	PillarStateTransition Pillar = "state_transition"
	// PillarMemory is the memory-operation pillar (SpanMemory).
	PillarMemory Pillar = "memory"
	// PillarOther is any span this package did not emit.
	PillarOther Pillar = "other"
)

// PillarForSpan maps a span name to the pillar it belongs to. Span names this
// package does not own map to PillarOther, so a caller can fold non-pillar
// spans (e.g. agm.session.*) into a single bucket rather than mishandling them.
func PillarForSpan(spanName string) Pillar {
	switch spanName {
	case SpanToolCall:
		return PillarToolCall
	case SpanReasoning:
		return PillarReasoning
	case SpanStateTransition:
		return PillarStateTransition
	case SpanMemory:
		return PillarMemory
	default:
		return PillarOther
	}
}

// Exported attribute-key aliases for downstream consumers (notably pkg/evalcase)
// that read four-pillar attributes back off captured spans. They alias the
// package-internal keys so there is a single source of truth: the span helpers
// write through the unexported names, and readers match on these exported ones
// without duplicating the string literals.
const (
	// Tool-call pillar.
	AttrToolName       = attrToolName
	AttrToolCallID     = attrToolCallID
	AttrToolArguments  = attrToolArguments
	AttrToolOutput     = attrToolOutput
	AttrToolRetryCount = attrToolRetryCount
	AttrToolDurationMS = attrToolDurationMS

	// Reasoning pillar.
	AttrReasoningPlan         = attrReasoningPlan
	AttrReasoningAction       = attrReasoningAction
	AttrReasoningObservation  = attrReasoningObservation
	AttrReasoningNextDecision = attrReasoningNextDecision
	AttrReasoningStep         = attrReasoningStep

	// State-transition pillar.
	AttrStateFrom         = attrStateFrom
	AttrStateTo           = attrStateTo
	AttrStateContextEdits = attrStateContextEdits
	AttrStateHandoff      = attrStateHandoff

	// Memory pillar.
	AttrMemoryOperation        = attrMemoryOperation
	AttrMemoryStore            = attrMemoryStore
	AttrMemoryQuery            = attrMemoryQuery
	AttrMemoryRelevance        = attrMemoryRelevance
	AttrMemoryFreshnessSeconds = attrMemoryFreshnessSeconds
	AttrMemoryResultCount      = attrMemoryResultCount

	// Shared.
	AttrErrorType = attrErrorType
)
