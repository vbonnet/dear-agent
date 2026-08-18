package otelsetup

import (
	"context"
	"net"
	"testing"
	"time"

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

// blackHoleCollector starts a TCP listener that accepts connections and then
// never speaks. This is the shape of a wedged OTLP collector: the dial and the
// TCP handshake both succeed, so nothing fails fast, but the gRPC HTTP/2
// handshake never completes and the export RPC blocks until its deadline. A
// closed port is *not* an adequate stand-in — that fails immediately and hides
// the hang this test guards.
func blackHoleCollector(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// Hold the connection open without reading or writing.
			t.Cleanup(func() { _ = conn.Close() })
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		<-done
	})
	return ln.Addr().String()
}

// TestInitTracer_ShutdownBoundedWhenCollectorBlackHoles pins the bug this
// package's OTLP export budget exists for: a collector that accepts but never
// answers must not stall process exit. Callers such as engram, safe-pr and
// mergeloop hand the returned shutdown func a context.Background(), so the
// bound has to come from the exporter's own timeout.
//
// Without otlptracegrpc.WithTimeout the exporter uses its 10s default and this
// takes ~10s regardless of the retry setting, because it is the *initial*
// attempt that blocks.
func TestInitTracer_ShutdownBoundedWhenCollectorBlackHoles(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", blackHoleCollector(t))

	shutdown := InitTracer("test-service")

	// Put a span in the batch so shutdown has something it must try to flush.
	_, span := otel.Tracer("test").Start(context.Background(), "black-hole-span")
	span.End()

	start := time.Now()
	_ = shutdown(context.Background()) // export failure against a dead collector is expected
	elapsed := time.Since(start)

	// otlpExportTimeout plus scheduling slack, and comfortably under the 10s
	// exporter default that this option replaces.
	if limit := otlpExportTimeout + 2*time.Second; elapsed > limit {
		t.Fatalf("shutdown took %v against a black-holed collector, want <= %v", elapsed, limit)
	}
}

// TestOTLPRetryBudget guards the two halves of the fix against each other:
// retries must stay enabled (a long-lived agm/mergeloop process should ride out
// a brief collector blip rather than drop the batch), and the retry schedule
// must fit inside the export budget. Defaults of 5s initial / 1m elapsed would
// burn the entire budget waiting to retry even once.
func TestOTLPRetryBudget(t *testing.T) {
	if otlpRetryInitialInterval >= otlpExportTimeout {
		t.Fatalf("initial retry interval %v leaves no room inside export budget %v",
			otlpRetryInitialInterval, otlpExportTimeout)
	}
	if otlpRetryMaxInterval > otlpExportTimeout {
		t.Fatalf("max retry interval %v exceeds export budget %v",
			otlpRetryMaxInterval, otlpExportTimeout)
	}
	if otlpRetryInitialInterval > otlpRetryMaxInterval {
		t.Fatalf("initial retry interval %v exceeds max %v",
			otlpRetryInitialInterval, otlpRetryMaxInterval)
	}
}
