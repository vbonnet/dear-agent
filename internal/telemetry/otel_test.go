package telemetry

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkmetricdata "go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// TestInitMeterNoEndpoint verifies the no-collector path: with no endpoint
// configured InitMeter installs a no-op provider, returns a no-op shutdown, and
// recording metrics through it does not panic.
func TestInitMeterNoEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	shutdown, err := InitMeter("test-service")
	if err != nil {
		t.Fatalf("InitMeter returned error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("InitMeter returned nil shutdown func")
	}

	// Recording against the no-op provider must be safe.
	ctx := context.Background()
	a := newAgentMetrics(otel.GetMeterProvider())
	a.TaskStarted(ctx)
	a.TaskCompleted(ctx, "ok")
	a.TokensUsed(ctx, "anthropic", "claude-opus-4-8", 123)
	a.StallDuration(ctx, 42.0)

	if err := shutdown(ctx); err != nil {
		t.Fatalf("shutdown returned error: %v", err)
	}
	// Shutdown is idempotent / safe to call again via the package func.
	if err := Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
}

// TestAgentMetricsRecord drives every instrument through an in-memory reader and
// asserts the four metrics are emitted with the expected names and attributes.
func TestAgentMetricsRecord(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	a := newAgentMetrics(mp)
	ctx := context.Background()
	a.TaskStarted(ctx)
	a.TaskCompleted(ctx, "archived")
	a.TokensUsed(ctx, "anthropic", "claude-opus-4-8", 1000)
	a.StallDuration(ctx, 250.5)

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
		"agent.tasks.active",
		"agent.tasks.completed",
		"agent.tokens.used",
		"agent.stall.duration_ms",
	} {
		if !got[name] {
			t.Errorf("expected metric %q to be recorded; got %v", name, got)
		}
	}
}

// TestSessionLifecycleHelpers exercises the span+metric convenience helpers to
// ensure they are panic-safe with the default (no-op) global providers.
func TestSessionLifecycleHelpers(t *testing.T) {
	ctx := context.Background()
	SessionStarted(ctx, "sess-1", "claude-opus-4-8", "anthropic", "WORKING")
	_, span := SessionExecute(ctx, "sess-1")
	span.End()
	SessionCompleted(ctx, "sess-1", "archived")
}
