package telemetry

import (
	"context"
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
	return &AgentMetrics{
		tasksActive:    tasksActive,
		tasksCompleted: tasksCompleted,
		tokensUsed:     tokensUsed,
		stallDuration:  stallDuration,
	}
}

// TaskStarted records that a task became active (active counter +1).
func (a *AgentMetrics) TaskStarted(ctx context.Context, attrs ...attribute.KeyValue) {
	if a == nil || a.tasksActive == nil {
		return
	}
	a.tasksActive.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// TaskCompleted records a terminal task outcome: active counter -1 and the
// completed counter +1, tagged with status (e.g. "archived", "error").
func (a *AgentMetrics) TaskCompleted(ctx context.Context, status string, attrs ...attribute.KeyValue) {
	if a == nil {
		return
	}
	withStatus := append([]attribute.KeyValue{attribute.String("status", status)}, attrs...)
	if a.tasksActive != nil {
		a.tasksActive.Add(ctx, -1, metric.WithAttributes(attrs...))
	}
	if a.tasksCompleted != nil {
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

func sessionAttrs(sessionID, model, provider, status string) []attribute.KeyValue {
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
	return attrs
}

// SessionStarted emits an agm.session.start span and increments the active-task
// counter. Call once a session has been created/registered.
func SessionStarted(ctx context.Context, sessionID, model, provider, status string) {
	attrs := sessionAttrs(sessionID, model, provider, status)
	_, span := sessionTracer().Start(ctx, "agm.session.start", trace.WithAttributes(attrs...))
	span.End()
	Agent().TaskStarted(ctx, attrs...)
}

// SessionExecute starts an agm.session.execute span for a unit of work (e.g.
// delivering a message/task to a session). The caller must call End on the
// returned span; pass the returned context to propagate the span.
func SessionExecute(ctx context.Context, sessionID string) (context.Context, trace.Span) {
	return sessionTracer().Start(ctx, "agm.session.execute",
		trace.WithAttributes(attribute.String("session_id", sessionID)))
}

// SessionCompleted emits an agm.session.complete span and records the terminal
// metric (active -1, completed +1{status}). Call once a session finishes
// (archived/closed).
func SessionCompleted(ctx context.Context, sessionID, status string) {
	attrs := sessionAttrs(sessionID, "", "", status)
	_, span := sessionTracer().Start(ctx, "agm.session.complete", trace.WithAttributes(attrs...))
	span.End()
	Agent().TaskCompleted(ctx, status, attribute.String("session_id", sessionID))
}
