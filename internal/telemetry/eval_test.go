package telemetry

import (
	"context"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkmetricdata "go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace"
)

// TestEvalAndDEARMetricsRecord drives the eval/DEAR instruments through an
// in-memory reader and asserts the three new metrics are emitted with their
// expected names.
func TestEvalAndDEARMetricsRecord(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	a := newAgentMetrics(mp)
	ctx := context.Background()
	a.recordEvalScore(ctx, "helpfulness", 0.92)
	a.recordEvalCaseGenerated(ctx)
	a.recordDEARCycle(ctx, "execute", 1234.5)

	var rm sdkmetricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}

	got := map[string]bool{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			got[m.Name] = true
		}
	}
	for _, name := range []string{
		"agent.eval.score",
		"agent.eval.cases_generated",
		"dear.cycle.duration_ms",
	} {
		if !got[name] {
			t.Errorf("expected metric %q to be recorded; got %v", name, got)
		}
	}
}

// TestRecordEvalScorePanicSafe verifies the package-level eval helper is safe
// with the default (no-op) global providers and with both a zero and a populated
// SpanContext.
func TestRecordEvalScorePanicSafe(t *testing.T) {
	ctx := context.Background()
	RecordEvalScore(ctx, trace.SpanContext{}, "groundedness", 0.5)

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{0x01},
		SpanID:  trace.SpanID{0x02},
	})
	RecordEvalScore(ctx, sc, "groundedness", 0.75)
}

// TestTraceToEvalCase covers the validation contract: an empty traceID is an
// error, a non-empty one succeeds and is panic-safe with no-op providers.
func TestTraceToEvalCase(t *testing.T) {
	ctx := context.Background()
	if err := TraceToEvalCase(ctx, ""); err == nil {
		t.Fatal("TraceToEvalCase(\"\") = nil, want error for empty traceID")
	}
	if err := TraceToEvalCase(ctx, "trace-abc123"); err != nil {
		t.Fatalf("TraceToEvalCase(valid) = %v, want nil", err)
	}
}

// TestRecordDEARCycleDuration ensures the convenience helper is panic-safe with
// both a named and an empty phase under the default no-op providers.
func TestRecordDEARCycleDuration(t *testing.T) {
	ctx := context.Background()
	RecordDEARCycleDuration(ctx, "retro", 88.0)
	RecordDEARCycleDuration(ctx, "", 12.0)
}
