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

func TestParseOTLPEndpoint(t *testing.T) {
	cases := []struct {
		raw          string
		wantTarget   string
		wantInsecure bool
	}{
		// The footgun case: a scheme-less host:port must still produce a valid
		// dial target (the exporter's env parser would map this to "").
		{"localhost:4317", "localhost:4317", true},
		{"http://localhost:4317", "localhost:4317", true},
		{"http://localhost:4317/", "localhost:4317", true},
		{"https://tempo.example.com:4317", "tempo.example.com:4317", false},
		{"  localhost:4317  ", "localhost:4317", true},
		{"127.0.0.1:4317", "127.0.0.1:4317", true},
	}
	for _, c := range cases {
		target, insecure := parseOTLPEndpoint(c.raw)
		if target != c.wantTarget || insecure != c.wantInsecure {
			t.Errorf("parseOTLPEndpoint(%q) = (%q, %v), want (%q, %v)",
				c.raw, target, insecure, c.wantTarget, c.wantInsecure)
		}
	}
}

func TestShortRevision(t *testing.T) {
	tests := map[string]string{
		"abc":        "abc",
		"1234567":    "1234567",
		"1234567890": "1234567",
	}
	for revision, want := range tests {
		if got := shortRevision(revision); got != want {
			t.Errorf("shortRevision(%q) = %q, want %q", revision, got, want)
		}
	}
}
