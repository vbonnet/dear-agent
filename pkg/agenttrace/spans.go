package agenttrace

import (
	"context"
	"reflect"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// recordError marks span as failed and tags it with the error type, following
// the OTel convention of error.type + span status. A nil err is a no-op so
// callers can defer span.End(err) unconditionally.
func recordError(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	span.SetAttributes(attribute.String(attrErrorType, reflect.TypeOf(err).String()))
}

// ---------------------------------------------------------------------------
// Pillar 1 — Tool-call spans
// ---------------------------------------------------------------------------

// ToolCallSpan instruments a single tool invocation (pillar 1). Instrument
// every MCP tool call in an AGM session with one of these so hallucinated
// arguments and silent retry loops become visible.
type ToolCallSpan struct {
	span    trace.Span
	start   time.Time
	retries int
}

// StartToolCall opens a gen_ai.tool.call span for the named tool. The returned
// context carries the span so nested work is parented correctly; always pair
// it with a deferred End.
func StartToolCall(ctx context.Context, toolName string) (context.Context, *ToolCallSpan) {
	ctx, span := tracer().Start(ctx, SpanToolCall, trace.WithSpanKind(trace.SpanKindInternal))
	span.SetAttributes(attribute.String(attrToolName, toolName))
	return ctx, &ToolCallSpan{span: span, start: time.Now()}
}

// SetCallID records the provider-assigned tool-call id (gen_ai.tool.call.id),
// letting a span be correlated back to the model turn that requested it.
func (s *ToolCallSpan) SetCallID(id string) {
	if kv, ok := strAttr(attrToolCallID, id); ok {
		s.span.SetAttributes(kv)
	}
}

// SetArguments records the (serialised) arguments the tool was called with.
func (s *ToolCallSpan) SetArguments(args string) {
	if kv, ok := strAttr(attrToolArguments, args); ok {
		s.span.SetAttributes(kv)
	}
}

// SetOutput records the raw tool output.
func (s *ToolCallSpan) SetOutput(output string) {
	if kv, ok := strAttr(attrToolOutput, output); ok {
		s.span.SetAttributes(kv)
	}
}

// IncRetry bumps the retry counter by one. Call it each time the tool is
// re-attempted after a failure; the total is written to the span at End.
func (s *ToolCallSpan) IncRetry() { s.retries++ }

// SetRetryCount sets the retry counter to an absolute value, for callers that
// track retries themselves rather than via IncRetry.
func (s *ToolCallSpan) SetRetryCount(n int) { s.retries = n }

// End closes the span, recording retry count, wall-clock duration, and — if
// err is non-nil — the error state. err may be nil for a clean call.
func (s *ToolCallSpan) End(err error) {
	s.span.SetAttributes(
		attribute.Int(attrToolRetryCount, s.retries),
		attribute.Int64(attrToolDurationMS, time.Since(s.start).Milliseconds()),
	)
	recordError(s.span, err)
	s.span.End()
}

// ---------------------------------------------------------------------------
// Pillar 2 — Reasoning spans
// ---------------------------------------------------------------------------

// ReasoningSpan instruments a reasoning/decision step (pillar 2): a Wayfinder
// planning phase or a VROOM role decision. It captures the plan-act-observe-
// decide loop so plan drift and wrong-branch selection are auditable.
type ReasoningSpan struct {
	span trace.Span
}

// StartReasoning opens a gen_ai.reasoning span. step is a short label for the
// reasoning step (e.g. the Wayfinder phase or VROOM role name).
func StartReasoning(ctx context.Context, step string) (context.Context, *ReasoningSpan) {
	ctx, span := tracer().Start(ctx, SpanReasoning, trace.WithSpanKind(trace.SpanKindInternal))
	if kv, ok := strAttr(attrReasoningStep, step); ok {
		span.SetAttributes(kv)
	}
	return ctx, &ReasoningSpan{span: span}
}

// SetPlan records the plan the model is operating under.
func (s *ReasoningSpan) SetPlan(plan string) { s.set(attrReasoningPlan, plan) }

// SetAction records the action the model picked.
func (s *ReasoningSpan) SetAction(action string) { s.set(attrReasoningAction, action) }

// SetObservation records what the model observed after acting.
func (s *ReasoningSpan) SetObservation(obs string) { s.set(attrReasoningObservation, obs) }

// SetNextDecision records the decision the model reached for the next step.
func (s *ReasoningSpan) SetNextDecision(dec string) { s.set(attrReasoningNextDecision, dec) }

func (s *ReasoningSpan) set(key, val string) {
	if kv, ok := strAttr(key, val); ok {
		s.span.SetAttributes(kv)
	}
}

// End closes the span, recording err as the error state when non-nil.
func (s *ReasoningSpan) End(err error) {
	recordError(s.span, err)
	s.span.End()
}

// ---------------------------------------------------------------------------
// Pillar 3 — State-transition spans
// ---------------------------------------------------------------------------

// StateTransitionSpan instruments a session state change (pillar 3): a session
// lifecycle transition, a context-compaction event, or an agent handoff. It
// captures before/after state plus the context edits and handoff payload so
// context loss and summarisation drift are catchable.
type StateTransitionSpan struct {
	span trace.Span
}

// StartStateTransition opens a gen_ai.state_transition span moving from one
// state to another (e.g. "active" → "compacted").
func StartStateTransition(ctx context.Context, from, to string) (context.Context, *StateTransitionSpan) {
	ctx, span := tracer().Start(ctx, SpanStateTransition, trace.WithSpanKind(trace.SpanKindInternal))
	attrs := make([]attribute.KeyValue, 0, 2)
	if kv, ok := strAttr(attrStateFrom, from); ok {
		attrs = append(attrs, kv)
	}
	if kv, ok := strAttr(attrStateTo, to); ok {
		attrs = append(attrs, kv)
	}
	span.SetAttributes(attrs...)
	return ctx, &StateTransitionSpan{span: span}
}

// SetContextEdits records a summary of edits applied to the context window
// during the transition (e.g. what compaction dropped or rewrote).
func (s *StateTransitionSpan) SetContextEdits(edits string) {
	if kv, ok := strAttr(attrStateContextEdits, edits); ok {
		s.span.SetAttributes(kv)
	}
}

// SetHandoffPayload records the payload handed off to the next agent/session.
func (s *StateTransitionSpan) SetHandoffPayload(payload string) {
	if kv, ok := strAttr(attrStateHandoff, payload); ok {
		s.span.SetAttributes(kv)
	}
}

// End closes the span, recording err as the error state when non-nil.
func (s *StateTransitionSpan) End(err error) {
	recordError(s.span, err)
	s.span.End()
}

// ---------------------------------------------------------------------------
// Pillar 4 — Memory-operation spans
// ---------------------------------------------------------------------------

// MemoryOp is the kind of long-term memory operation being instrumented.
type MemoryOp string

const (
	// MemoryRead is a point read from a long-term store.
	MemoryRead MemoryOp = "read"
	// MemoryWrite is a write to a long-term store.
	MemoryWrite MemoryOp = "write"
	// MemoryQuery is a search/retrieval query against a long-term store.
	MemoryQuery MemoryOp = "query"
)

// MemoryOpSpan instruments a long-term memory operation (pillar 4): an engram
// KB read/write or an auto-memory operation. It captures the store, query,
// relevance scores, and freshness so stale reads and wrong-entity retrieval
// are exposed.
type MemoryOpSpan struct {
	span trace.Span
}

// StartMemoryOp opens a gen_ai.memory span for an operation against the named
// store (e.g. "engram-kb", "auto-memory").
func StartMemoryOp(ctx context.Context, op MemoryOp, store string) (context.Context, *MemoryOpSpan) {
	ctx, span := tracer().Start(ctx, SpanMemory, trace.WithSpanKind(trace.SpanKindInternal))
	span.SetAttributes(attribute.String(attrMemoryOperation, string(op)))
	if kv, ok := strAttr(attrMemoryStore, store); ok {
		span.SetAttributes(kv)
	}
	return ctx, &MemoryOpSpan{span: span}
}

// SetQuery records the query string issued against the store.
func (s *MemoryOpSpan) SetQuery(query string) {
	if kv, ok := strAttr(attrMemoryQuery, query); ok {
		s.span.SetAttributes(kv)
	}
}

// SetRelevanceScore records the relevance score of the top result (or the
// aggregate score for the operation), typically in [0,1].
func (s *MemoryOpSpan) SetRelevanceScore(score float64) {
	s.span.SetAttributes(attribute.Float64(attrMemoryRelevance, score))
}

// SetFreshness records how stale the retrieved data is. It is stored as whole
// seconds under gen_ai.memory.freshness_seconds.
func (s *MemoryOpSpan) SetFreshness(age time.Duration) {
	s.span.SetAttributes(attribute.Int64(attrMemoryFreshnessSeconds, int64(age.Seconds())))
}

// SetResultCount records how many results the operation read or returned.
func (s *MemoryOpSpan) SetResultCount(n int) {
	s.span.SetAttributes(attribute.Int(attrMemoryResultCount, n))
}

// End closes the span, recording err as the error state when non-nil.
func (s *MemoryOpSpan) End(err error) {
	recordError(s.span, err)
	s.span.End()
}
