package telemetry

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// instrumentationScope is the meter/tracer name for agent telemetry. It shows
// up as otel.scope.name on every metric and span emitted here.
const instrumentationScope = "github.com/vbonnet/dear-agent/agent"

// AgentMetrics holds the OTel instruments for agent/session activity. Created
// from the global MeterProvider, so it is a no-op until InitMeter installs a
// real provider — instrumented call sites never need to check whether metrics
// are enabled.
type AgentMetrics struct {
	tasksActive    metric.Int64UpDownCounter // agent.tasks.active
	tasksCompleted metric.Int64Counter       // agent.tasks.completed{status}
	tokensUsed     metric.Int64Counter       // agent.tokens.used{provider,model}
	stallDuration  metric.Float64Histogram   // agent.stall.duration_ms

	// Eval / DEAR instruments. These back the eval-as-span-attribute loop
	// (see RecordEvalScore / TraceToEvalCase) and DEAR cycle timing.
	evalScore          metric.Float64Histogram // agent.eval.score{gen_ai.eval.name}
	evalCasesGenerated metric.Int64Counter     // agent.eval.cases_generated
	dearCycleDuration  metric.Float64Histogram // dear.cycle.duration_ms{dear.phase}

	// human_typing guard instruments. The guard is advisory/non-blocking and is
	// known to over-capture (stale scrollback, old prompts, ghost-text remnants);
	// these counters measure how often it fires and — via the before/after stash
	// compare — how often that detection was a false positive, so the heuristic
	// can be improved later. All tagged {harness, stash_key_verified}.
	humanTypingDetected      metric.Int64Counter // human_typing.detected
	humanTypingStashSent     metric.Int64Counter // human_typing.stash_sent
	humanTypingStashed       metric.Int64Counter // human_typing.stashed
	humanTypingFalsePositive metric.Int64Counter // human_typing.false_positive
}

var (
	agentOnce    sync.Once
	agentMetrics *AgentMetrics
)

// Agent returns the process-wide AgentMetrics, building the instruments on
// first use. Call after InitMeter so the instruments bind to the real provider;
// if called earlier they bind to the no-op provider and stay no-ops.
func Agent() *AgentMetrics {
	agentOnce.Do(func() {
		agentMetrics = newAgentMetrics(otel.GetMeterProvider())
	})
	return agentMetrics
}

func newAgentMetrics(mp metric.MeterProvider) *AgentMetrics {
	m := mp.Meter(instrumentationScope)
	// Errors here only occur for invalid instrument names (compile-time
	// constants below), so the no-op instrument that is returned on error is an
	// acceptable fallback; we keep them rather than failing the host binary.
	tasksActive, _ := m.Int64UpDownCounter(
		"agent.tasks.active",
		metric.WithDescription("Number of agent tasks currently in flight"),
		metric.WithUnit("{task}"),
	)
	tasksCompleted, _ := m.Int64Counter(
		"agent.tasks.completed",
		metric.WithDescription("Total agent tasks completed, by terminal status"),
		metric.WithUnit("{task}"),
	)
	tokensUsed, _ := m.Int64Counter(
		"agent.tokens.used",
		metric.WithDescription("Total model tokens consumed, by provider and model"),
		metric.WithUnit("{token}"),
	)
	stallDuration, _ := m.Float64Histogram(
		"agent.stall.duration_ms",
		metric.WithDescription("Duration of agent stalls (no progress) in milliseconds"),
		metric.WithUnit("ms"),
	)
	evalScore, _ := m.Float64Histogram(
		"agent.eval.score",
		metric.WithDescription("Eval scores attached to agent traces, by eval name"),
		metric.WithUnit("1"),
	)
	evalCasesGenerated, _ := m.Int64Counter(
		"agent.eval.cases_generated",
		metric.WithDescription("Total production traces converted into eval cases"),
		metric.WithUnit("{case}"),
	)
	dearCycleDuration, _ := m.Float64Histogram(
		"dear.cycle.duration_ms",
		metric.WithDescription("Duration of a DEAR (Define-Execute-Audit-Retro) cycle in milliseconds"),
		metric.WithUnit("ms"),
	)
	humanTypingDetected, _ := m.Int64Counter(
		"human_typing.detected",
		metric.WithDescription("Times the advisory human_typing guard fired (non-blocking), by harness"),
		metric.WithUnit("{detection}"),
	)
	humanTypingStashSent, _ := m.Int64Counter(
		"human_typing.stash_sent",
		metric.WithDescription("Times a stash key was sent before delivery to preserve detected input, by harness"),
		metric.WithUnit("{stash}"),
	)
	humanTypingStashed, _ := m.Int64Counter(
		"human_typing.stashed",
		metric.WithDescription("Times a stash actually set input aside (input line cleared afterward), by harness"),
		metric.WithUnit("{stash}"),
	)
	humanTypingFalsePositive, _ := m.Int64Counter(
		"human_typing.false_positive",
		metric.WithDescription("Times a stash was sent but nothing was stashed — human_typing over-captured, by harness"),
		metric.WithUnit("{detection}"),
	)
	return &AgentMetrics{
		tasksActive:              tasksActive,
		tasksCompleted:           tasksCompleted,
		tokensUsed:               tokensUsed,
		stallDuration:            stallDuration,
		evalScore:                evalScore,
		evalCasesGenerated:       evalCasesGenerated,
		dearCycleDuration:        dearCycleDuration,
		humanTypingDetected:      humanTypingDetected,
		humanTypingStashSent:     humanTypingStashSent,
		humanTypingStashed:       humanTypingStashed,
		humanTypingFalsePositive: humanTypingFalsePositive,
	}
}

// taskAttrs builds the low-cardinality attribute set shared by the active and
// completed task metrics. session_id is deliberately excluded: it is a
// high-cardinality value (one per session) that belongs on spans, not metrics,
// and tagging the up/down active counter with it would also unbalance the
// increment/decrement (they would land on distinct timeseries and leak).
func taskAttrs(provider, model string) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 3)
	if provider != "" {
		attrs = append(attrs, attribute.String("provider", provider))
	}
	if model != "" {
		attrs = append(attrs, attribute.String("model", model))
	}
	return attrs
}

// TaskStarted records that a task became active (active counter +1), tagged
// with the low-cardinality provider/model pair.
func (a *AgentMetrics) TaskStarted(ctx context.Context, provider, model string) {
	if a == nil || a.tasksActive == nil {
		return
	}
	a.tasksActive.Add(ctx, 1, metric.WithAttributes(taskAttrs(provider, model)...))
}

// TaskCompleted records a terminal task outcome: active counter -1 (with the
// same provider/model attributes TaskStarted used, so the up/down counter
// balances) and the completed counter +1, additionally tagged with status
// (e.g. "archived", "error").
func (a *AgentMetrics) TaskCompleted(ctx context.Context, provider, model, status string) {
	if a == nil {
		return
	}
	if a.tasksActive != nil {
		a.tasksActive.Add(ctx, -1, metric.WithAttributes(taskAttrs(provider, model)...))
	}
	if a.tasksCompleted != nil {
		withStatus := append(taskAttrs(provider, model), attribute.String("status", status))
		a.tasksCompleted.Add(ctx, 1, metric.WithAttributes(withStatus...))
	}
}

// TokensUsed records model token consumption for a provider/model pair.
func (a *AgentMetrics) TokensUsed(ctx context.Context, provider, model string, tokens int64, attrs ...attribute.KeyValue) {
	if a == nil || a.tokensUsed == nil {
		return
	}
	all := append([]attribute.KeyValue{
		attribute.String("provider", provider),
		attribute.String("model", model),
	}, attrs...)
	a.tokensUsed.Add(ctx, tokens, metric.WithAttributes(all...))
}

// StallDuration records the length of an agent stall in milliseconds.
func (a *AgentMetrics) StallDuration(ctx context.Context, ms float64, attrs ...attribute.KeyValue) {
	if a == nil || a.stallDuration == nil {
		return
	}
	a.stallDuration.Record(ctx, ms, metric.WithAttributes(attrs...))
}

// Session lifecycle instrumentation helpers.
//
// These emit the agm.session.* spans and the matching metrics from primitive
// args so call sites in the agm command tree stay decoupled from this package's
// instrument plumbing (and this package stays decoupled from agm/manifest).

func sessionTracer() trace.Tracer { return otel.Tracer(instrumentationScope) }

func sessionAttrs(sessionID, model, provider, status, role string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{attribute.String("session_id", sessionID)}
	if model != "" {
		attrs = append(attrs, attribute.String("model", model))
	}
	if provider != "" {
		attrs = append(attrs, attribute.String("provider", provider))
	}
	if status != "" {
		attrs = append(attrs, attribute.String("status", status))
	}
	if role != "" {
		attrs = append(attrs, attribute.String("vroom.role", role))
	}
	return attrs
}

// SessionStarted emits an agm.session.start span and increments the active-task
// counter. Call once a session has been created/registered. role is the VROOM
// role assigned to this session (empty string if unset).
func SessionStarted(ctx context.Context, sessionID, model, provider, status, role string) {
	attrs := sessionAttrs(sessionID, model, provider, status, role)
	_, span := sessionTracer().Start(ctx, "agm.session.start", trace.WithAttributes(attrs...))
	span.End()
	Agent().TaskStarted(ctx, provider, model)
}

// SessionExecute starts an agm.session.execute span for a unit of work (e.g.
// delivering a message/task to a session). The caller must call End on the
// returned span; pass the returned context to propagate the span.
func SessionExecute(ctx context.Context, sessionID string) (context.Context, trace.Span) {
	return sessionTracer().Start(ctx, "agm.session.execute",
		trace.WithAttributes(attribute.String("session_id", sessionID)))
}

// SessionCompleted emits an agm.session.complete span and records the terminal
// metric (active -1, completed +1{status}). The model/provider must match those
// passed to SessionStarted so the active up/down counter balances. Call once a
// session finishes (archived/closed).
func SessionCompleted(ctx context.Context, sessionID, model, provider, status string) {
	attrs := sessionAttrs(sessionID, model, provider, status, "")
	_, span := sessionTracer().Start(ctx, "agm.session.complete", trace.WithAttributes(attrs...))
	span.End()
	Agent().TaskCompleted(ctx, provider, model, status)
}

// Eval-as-span-attribute instrumentation.
//
// These support the eval feedback loop popularised by Arize/Braintrust: an
// evaluator (online or offline) scores a production trace, the score is
// attached back to that trace for correlation in the trace UI, and high-signal
// traces are promoted into a regression eval dataset. Scores also flow to the
// agent.eval.score histogram for aggregate dashboards/alerting.

// RecordEvalScore attaches an eval result to telemetry two ways: as attributes
// on the active span in ctx (so the score is visible alongside the trace it
// grades) and as an agent.eval.score histogram sample tagged with evalName.
//
// spanCtx identifies the trace/span the eval pertains to; its IDs are recorded
// as span attributes for correlation but deliberately kept off the metric to
// avoid high-cardinality timeseries (see taskAttrs). A zero spanCtx is fine —
// the correlation attributes are simply omitted. Safe to call with the default
// no-op providers.
func RecordEvalScore(ctx context.Context, spanCtx trace.SpanContext, evalName string, score float64) {
	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		attrs := []attribute.KeyValue{
			attribute.String("gen_ai.eval.name", evalName),
			attribute.Float64("gen_ai.eval.score", score),
		}
		if spanCtx.IsValid() {
			attrs = append(attrs,
				attribute.String("gen_ai.eval.trace_id", spanCtx.TraceID().String()),
				attribute.String("gen_ai.eval.span_id", spanCtx.SpanID().String()),
			)
		}
		span.SetAttributes(attrs...)
	}
	Agent().recordEvalScore(ctx, evalName, score)
}

func (a *AgentMetrics) recordEvalScore(ctx context.Context, evalName string, score float64) {
	if a == nil || a.evalScore == nil {
		return
	}
	a.evalScore.Record(ctx, score, metric.WithAttributes(attribute.String("gen_ai.eval.name", evalName)))
}

// TraceToEvalCase marks a production trace for conversion into an eval case: it
// adds a gen_ai.eval.case_generated event to the active span (carrying the source
// trace ID) and increments the agent.eval.cases_generated counter. This is the
// "trace → eval dataset" half of the loop, turning high-signal production
// traces into regression cases.
//
// It returns an error for an empty traceID and is otherwise safe to call with
// the default no-op providers.
func TraceToEvalCase(ctx context.Context, traceID string) error {
	if traceID == "" {
		return fmt.Errorf("telemetry: empty trace ID")
	}
	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		span.AddEvent("gen_ai.eval.case_generated",
			trace.WithAttributes(attribute.String("gen_ai.eval.source_trace_id", traceID)))
	}
	Agent().recordEvalCaseGenerated(ctx)
	return nil
}

func (a *AgentMetrics) recordEvalCaseGenerated(ctx context.Context) {
	if a == nil || a.evalCasesGenerated == nil {
		return
	}
	a.evalCasesGenerated.Add(ctx, 1)
}

// RecordDEARCycleDuration records the wall-clock duration of one DEAR
// (Define-Execute-Audit-Retro) cycle in milliseconds, tagged with the phase
// that just completed (low cardinality, e.g. "define", "execute", "audit",
// "retro"). An empty phase records the sample untagged.
func RecordDEARCycleDuration(ctx context.Context, phase string, ms float64) {
	Agent().recordDEARCycle(ctx, phase, ms)
}

func (a *AgentMetrics) recordDEARCycle(ctx context.Context, phase string, ms float64) {
	if a == nil || a.dearCycleDuration == nil {
		return
	}
	var attrs []attribute.KeyValue
	if phase != "" {
		attrs = append(attrs, attribute.String("dear.phase", phase))
	}
	a.dearCycleDuration.Record(ctx, ms, metric.WithAttributes(attrs...))
}

// human_typing guard instrumentation.
//
// The human_typing guard is advisory and non-blocking: it never aborts a send.
// It is known to over-capture — stale scrollback, old prompts, and ghost-text
// remnants all read as "a human is typing". Instead of blocking, callers stash
// the composer (Claude Code's C-s auto-unstashes after the next submit, so
// genuine human input survives) and deliver anyway. These counters let us
// quantify the over-capture from production: a stash that changed nothing means
// the detection was a false positive. Data feeds a future heuristic revamp.

func humanTypingAttrs(harness string, verified bool) []attribute.KeyValue {
	if harness == "" {
		harness = "claude-code"
	}
	return []attribute.KeyValue{
		attribute.String("harness", harness),
		attribute.Bool("stash_key_verified", verified),
	}
}

// RecordHumanTypingDetected records one advisory human_typing detection (the
// guard fired but did not block), tagged by harness.
func RecordHumanTypingDetected(ctx context.Context, harness string) {
	a := Agent()
	if a == nil || a.humanTypingDetected == nil {
		return
	}
	a.humanTypingDetected.Add(ctx, 1, metric.WithAttributes(humanTypingAttrs(harness, false)...))
}

// RecordHumanTypingStash records the outcome of a pre-delivery stash attempt:
// sent (the stash key went to the pane), stashed (the input line was cleared,
// so real input was set aside), and falsePositive (the stash changed nothing,
// so the human_typing detection over-captured). verified reports whether the
// harness's stash key is a confirmed mapping (only claude-code today). Counters
// are tagged {harness, stash_key_verified}.
func RecordHumanTypingStash(ctx context.Context, harness string, verified, sent, stashed, falsePositive bool) {
	a := Agent()
	if a == nil {
		return
	}
	attrs := metric.WithAttributes(humanTypingAttrs(harness, verified)...)
	if sent && a.humanTypingStashSent != nil {
		a.humanTypingStashSent.Add(ctx, 1, attrs)
	}
	if stashed && a.humanTypingStashed != nil {
		a.humanTypingStashed.Add(ctx, 1, attrs)
	}
	if falsePositive && a.humanTypingFalsePositive != nil {
		a.humanTypingFalsePositive.Add(ctx, 1, attrs)
	}
}
