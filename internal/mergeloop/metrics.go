package mergeloop

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationScope = "github.com/vbonnet/dear-agent/mergeloop"

// Metrics holds the OTel instruments and tracer for the merge loop. Build it
// with [NewMetrics] after telemetry.InitMeter/otelsetup.InitTracer. A nil
// *Metrics is valid: every method is a no-op, so the driver runs unmetered in
// tests without any conditional plumbing.
type Metrics struct {
	tracer trace.Tracer

	timeToMerge   metric.Float64Histogram // mergeloop.time_to_merge (ms)
	stalls        metric.Int64Counter     // mergeloop.pr.stall
	escalations   metric.Int64Counter     // mergeloop.human_escalation{reason}
	agentsSpawned metric.Int64Counter     // mergeloop.agent_spawned{kind}
	merges        metric.Int64Counter     // mergeloop.merged
	openGauge     metric.Int64Counter     // mergeloop.tick.open_prs (per-tick sample)
}

// NewMetrics builds the instruments from the global providers. Instrument-name
// errors fall back to no-op instruments rather than failing the binary.
func NewMetrics() *Metrics {
	m := otel.GetMeterProvider().Meter(instrumentationScope)
	timeToMerge, _ := m.Float64Histogram("mergeloop.time_to_merge",
		metric.WithDescription("Time from first-seen to merged"), metric.WithUnit("ms"))
	stalls, _ := m.Int64Counter("mergeloop.pr.stall",
		metric.WithDescription("PRs actionable but untouched beyond the stall threshold"), metric.WithUnit("{pr}"))
	escalations, _ := m.Int64Counter("mergeloop.human_escalation",
		metric.WithDescription("Escalations to a human, by reason"), metric.WithUnit("{escalation}"))
	agentsSpawned, _ := m.Int64Counter("mergeloop.agent_spawned",
		metric.WithDescription("Agent sessions spawned to fix code, by kind"), metric.WithUnit("{session}"))
	merges, _ := m.Int64Counter("mergeloop.merged",
		metric.WithDescription("PRs merged by the loop"), metric.WithUnit("{pr}"))
	openGauge, _ := m.Int64Counter("mergeloop.tick.open_prs",
		metric.WithDescription("Open PR count sampled per tick"), metric.WithUnit("{pr}"))
	return &Metrics{
		tracer:        otel.Tracer(instrumentationScope),
		timeToMerge:   timeToMerge,
		stalls:        stalls,
		escalations:   escalations,
		agentsSpawned: agentsSpawned,
		merges:        merges,
		openGauge:     openGauge,
	}
}

// StartTickSpan opens the per-tick span. Returns the child ctx and an end func;
// both are safe to use on a nil *Metrics.
func (m *Metrics) StartTickSpan(ctx context.Context, repo string) (context.Context, func()) {
	if m == nil || m.tracer == nil {
		return ctx, func() {}
	}
	ctx, span := m.tracer.Start(ctx, "mergeloop.tick",
		trace.WithAttributes(attribute.String("repo", repo)))
	return ctx, func() { span.End() }
}

func (m *Metrics) recordTick(ctx context.Context, res TickResult) {
	if m == nil {
		return
	}
	m.openGauge.Add(ctx, int64(res.OpenPRs))
}

func (m *Metrics) recordStall(ctx context.Context, _ int, st State) {
	if m == nil {
		return
	}
	m.stalls.Add(ctx, 1, metric.WithAttributes(attribute.String("state", string(st))))
}

func (m *Metrics) recordMerge(ctx context.Context, _ int, age time.Duration) {
	if m == nil {
		return
	}
	m.merges.Add(ctx, 1)
	if age > 0 {
		m.timeToMerge.Record(ctx, float64(age.Milliseconds()))
	}
}

func (m *Metrics) recordAgentSpawn(ctx context.Context, _ int, kind string) {
	if m == nil {
		return
	}
	m.agentsSpawned.Add(ctx, 1, metric.WithAttributes(attribute.String("kind", kind)))
}

func (m *Metrics) recordEscalation(ctx context.Context, _ int, reason string) {
	if m == nil {
		return
	}
	m.escalations.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", reason)))
}
