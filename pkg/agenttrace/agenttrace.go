// Package agenttrace provides the four-pillar trace schema for dear-agent's
// agent instrumentation, built on top of the OpenTelemetry bootstrap in
// pkg/otelsetup.
//
// The deep-research telemetry report (Braintrust 2026 / Arize OpenInference)
// identifies four span types that every agent trace needs in order to make
// agent behaviour debuggable:
//
//  1. Tool-call spans     — what tool ran, with what arguments, what it
//     returned, how long it took, how many retries, and whether it errored.
//     Hallucinated arguments and silent retry loops blend into normal traffic
//     without these.
//  2. Reasoning spans     — the model's plan, the action it picked, the
//     observation it made, and the next decision. Surfaces plan drift and
//     wrong-branch selection a single LLM span can't show.
//  3. State-transition spans — the state before and after a step, context
//     edits applied, and handoff payloads. Catches context loss and
//     summarisation drift in longer runs.
//  4. Memory-operation spans — reads/writes to long-term stores, the query
//     issued, relevance scores, and freshness. Exposes stale reads,
//     wrong-entity retrieval, and memory leakage between sessions.
//
// All attribute names follow the OpenTelemetry GenAI semantic conventions
// (the gen_ai.* namespace). Where the conventions do not (yet) define a key
// for one of the four pillars, this package uses a gen_ai.* extension key,
// documented inline next to each constant.
//
// Like pkg/otelsetup, this package is a thin wrapper over the global
// TracerProvider: when no collector is configured InitTracer installs a no-op
// provider, so the helpers here are safe to call unconditionally — they emit
// nothing when tracing is disabled.
package agenttrace

import (
	"strconv"
	"unicode/utf8"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// instrumentationName is the OTel instrumentation scope for every span this
// package emits. It mirrors the "engram/<area>" convention used elsewhere in
// the repo (see engram/internal/tokentracking).
const instrumentationName = "dear-agent/agenttrace"

// Span name constants, one per pillar. These follow the gen_ai.* operation
// naming requested by the four-pillar schema and are stable identifiers that
// downstream eval/aggregation code can match on.
const (
	// SpanToolCall is the span name for a single tool invocation.
	SpanToolCall = "gen_ai.tool.call"
	// SpanReasoning is the span name for a reasoning/decision step.
	SpanReasoning = "gen_ai.reasoning"
	// SpanStateTransition is the span name for a session state change.
	SpanStateTransition = "gen_ai.state_transition"
	// SpanMemory is the span name for a long-term memory operation.
	SpanMemory = "gen_ai.memory"
)

// Attribute keys. Keys that exist in the OTel GenAI semantic conventions are
// used verbatim; the rest are gen_ai.* extensions for the parts of the
// four-pillar schema the conventions do not yet cover.
const (
	// --- Tool-call pillar ---
	attrToolName       = "gen_ai.tool.name"        // convention
	attrToolCallID     = "gen_ai.tool.call.id"     // convention
	attrToolArguments  = "gen_ai.tool.arguments"   // extension: raw/serialised args
	attrToolOutput     = "gen_ai.tool.output"      // extension: raw tool output
	attrToolRetryCount = "gen_ai.tool.retry_count" // extension
	attrToolDurationMS = "gen_ai.tool.duration_ms" // extension (also in span timing)

	// --- Reasoning pillar (all extensions) ---
	attrReasoningPlan         = "gen_ai.reasoning.plan"
	attrReasoningAction       = "gen_ai.reasoning.action"
	attrReasoningObservation  = "gen_ai.reasoning.observation"
	attrReasoningNextDecision = "gen_ai.reasoning.next_decision"
	attrReasoningStep         = "gen_ai.reasoning.step"

	// --- State-transition pillar (all extensions) ---
	attrStateFrom         = "gen_ai.state.from"
	attrStateTo           = "gen_ai.state.to"
	attrStateContextEdits = "gen_ai.state.context_edits"
	attrStateHandoff      = "gen_ai.state.handoff_payload"

	// --- Memory pillar (all extensions) ---
	attrMemoryOperation        = "gen_ai.memory.operation"
	attrMemoryStore            = "gen_ai.memory.store"
	attrMemoryQuery            = "gen_ai.memory.query"
	attrMemoryRelevance        = "gen_ai.memory.relevance_score"
	attrMemoryFreshnessSeconds = "gen_ai.memory.freshness_seconds"
	attrMemoryResultCount      = "gen_ai.memory.result_count"

	// --- shared ---
	attrErrorType = "error.type" // convention
)

// maxAttrLen bounds the length of free-text attributes (arguments, outputs,
// plans, payloads). Traces are not a place to ship megabytes of tool output;
// anything longer is truncated with an explicit marker so a reader knows the
// value was clipped rather than empty.
const maxAttrLen = 8 * 1024

// truncate clips s to at most maxAttrLen bytes, appending a marker noting how
// many bytes were dropped. The cut is backed up to a UTF-8 rune boundary so a
// multi-byte rune is never split — emitting invalid UTF-8 can make collectors
// reject the span. The marker keeps truncation visible in the trace UI.
func truncate(s string) string {
	if len(s) <= maxAttrLen {
		return s
	}
	end := maxAttrLen
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	dropped := len(s) - end
	return s[:end] + "…[truncated " + strconv.Itoa(dropped) + " bytes]"
}

// tracer returns the shared tracer for this package. It resolves the global
// TracerProvider on every call, which is cheap and ensures a no-op provider
// (tracing disabled) is honoured even if InitTracer ran after a span helper
// was first referenced.
func tracer() trace.Tracer {
	return otel.Tracer(instrumentationName)
}

// strAttr builds a truncated string attribute, skipping empty values so the
// span carries only the fields the caller actually populated.
func strAttr(key, val string) (attribute.KeyValue, bool) {
	if val == "" {
		return attribute.KeyValue{}, false
	}
	return attribute.String(key, truncate(val)), true
}
