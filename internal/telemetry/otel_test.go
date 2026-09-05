package telemetry

import (
	"context"
	"net"
	"testing"
	"time"

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
	a.TaskStarted(ctx, "anthropic", "claude-opus-4-8")
	a.TaskCompleted(ctx, "anthropic", "claude-opus-4-8", "ok")
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
	a.TaskStarted(ctx, "anthropic", "claude-opus-4-8")
	a.TaskCompleted(ctx, "anthropic", "claude-opus-4-8", "archived")
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

	// The active up/down counter must balance back to zero: TaskStarted(+1)
	// and TaskCompleted(-1) carry identical provider/model attributes, so they
	// accumulate on a single timeseries. Regression guard for the prior bug
	// where the increment and decrement used mismatched attribute sets and the
	// gauge leaked.
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "agent.tasks.active" {
				continue
			}
			sum, ok := m.Data.(sdkmetricdata.Sum[int64])
			if !ok {
				t.Fatalf("agent.tasks.active: unexpected data type %T", m.Data)
			}
			var total int64
			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
			if total != 0 {
				t.Errorf("agent.tasks.active must balance to 0 after start+complete, got %d", total)
			}
		}
	}
}

// TestInitMeterURLEndpoint verifies that an http:// or https:// prefixed endpoint
// is accepted without error (the scheme is stripped before the gRPC dial).
func TestInitMeterURLEndpoint(t *testing.T) {
	for _, ep := range []string{"http://localhost:4317", "https://localhost:4317"} {
		t.Run(ep, func(t *testing.T) {
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", ep)
			shutdown, err := InitMeter("test-service")
			if err != nil {
				t.Fatalf("InitMeter(%q) returned error: %v", ep, err)
			}
			if shutdown == nil {
				t.Fatal("InitMeter returned nil shutdown func")
			}
			_ = shutdown(context.Background())
			_ = Shutdown(context.Background())
		})
	}
}

// TestSessionLifecycleHelpers exercises the span+metric convenience helpers to
// ensure they are panic-safe with the default (no-op) global providers.
func TestSessionLifecycleHelpers(t *testing.T) {
	ctx := context.Background()
	SessionStarted(ctx, "sess-1", "claude-opus-4-8", "anthropic", "WORKING", "worker")
	_, span := SessionExecute(ctx, "sess-1")
	span.End()
	SessionCompleted(ctx, "sess-1", "claude-opus-4-8", "anthropic", "archived")
}

// TestSessionStarted_EmptyRole verifies that an empty role string does not
// panic and is accepted without error (role is optional).
func TestSessionStarted_EmptyRole(t *testing.T) {
	ctx := context.Background()
	// Must not panic. The no-op global provider is used (no OTLP endpoint set).
	SessionStarted(ctx, "sess-2", "claude-sonnet-4-6", "anthropic", "WORKING", "")
}

// TestInitMeter_ShutdownBoundedWhenCollectorBlackHoles is the metrics twin of
// otelsetup's tracer test: a collector that accepts the TCP connection but
// never answers must not stall MeterProvider.Shutdown. cc-usage-monitor
// flushes metrics on exit, so an unbounded export shows up as a
// hung CLI. The bound comes from otlpmetricgrpc.WithTimeout: disabling retry
// does not help, because it is the initial attempt that blocks.
func TestInitMeter_ShutdownBoundedWhenCollectorBlackHoles(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	accepted := make(chan struct{})
	go func() {
		defer close(accepted)
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			t.Cleanup(func() { _ = conn.Close() })
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		<-accepted
	})

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", ln.Addr().String())

	shutdown, err := InitMeter("test-service")
	if err != nil {
		t.Fatalf("InitMeter returned error: %v", err)
	}

	// Record something so the final collect has data to push.
	ctx := context.Background()
	newAgentMetrics(otel.GetMeterProvider()).TaskStarted(ctx, "anthropic", "claude-opus-4-8")

	start := time.Now()
	_ = shutdown(ctx) // export failure against a dead collector is expected
	elapsed := time.Since(start)

	if limit := otlpExportTimeout + 2*time.Second; elapsed > limit {
		t.Fatalf("shutdown took %v against a black-holed collector, want <= %v", elapsed, limit)
	}
}
