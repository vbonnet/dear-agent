package otelsetup

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestInitTracer_NoopWhenEndpointUnset(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	shutdown := InitTracer("test-service")
	defer shutdown(context.Background()) //nolint:errcheck

	tp := otel.GetTracerProvider()
	if _, ok := tp.(noop.TracerProvider); !ok {
		t.Fatalf("expected noop.TracerProvider when endpoint unset, got %T", tp)
	}
}

func TestInitTracer_RealProviderWhenEndpointSet(t *testing.T) {
	// Point at a non-existent collector — the provider is still created,
	// spans just won't be exported.
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")

	shutdown := InitTracer("test-service")
	defer shutdown(context.Background()) //nolint:errcheck

	tp := otel.GetTracerProvider()
	if _, ok := tp.(*sdktrace.TracerProvider); !ok {
		t.Fatalf("expected *sdktrace.TracerProvider when endpoint set, got %T", tp)
	}
}

func TestInitTracer_SpanCreation(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")

	shutdown := InitTracer("test-service")
	defer shutdown(context.Background()) //nolint:errcheck

	tracer := otel.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	if !span.SpanContext().IsValid() {
		t.Fatal("expected valid span context")
	}
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
}

func TestInitTracer_ShutdownClean(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")

	shutdown := InitTracer("test-service")

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown returned error: %v", err)
	}
}

func TestInitTracer_NoopShutdownClean(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	shutdown := InitTracer("test-service")

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown returned error: %v", err)
	}
}
