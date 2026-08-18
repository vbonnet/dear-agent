// Package otelsetup provides OpenTelemetry bootstrap for engram services.
//
// It configures a TracerProvider with an OTLP gRPC exporter driven by
// OTEL_EXPORTER_OTLP_ENDPOINT (e.g. http://localhost:4317 for a local Jaeger
// started with `otel-local up`, or a Grafana Tempo endpoint). When the env var
// is not set, a no-op TracerProvider is returned so instrumented code works
// without a collector running.
//
// The endpoint may be given with or without a scheme: "localhost:4317",
// "http://localhost:4317", and "https://tempo:4317" all work. A scheme-less or
// http:// endpoint is treated as plaintext gRPC; https:// uses TLS. This is a
// deliberate guard against a sharp footgun — otlptracegrpc's own env parser
// maps a scheme-less "localhost:4317" to an empty target and then silently
// exports nothing.
package otelsetup

import (
	"context"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace/noop"
)

// OTLP export budget.
//
// otlpExportTimeout bounds one whole export — the initial gRPC attempt *and*
// every retry — because otlptracegrpc applies it around the entire retry loop
// (see client.exportContext). Without it the exporter falls back to its 10s
// default, so a black-holed collector (one that accepts the TCP connection but
// never answers) stalls Shutdown/ForceFlush for 10s on every process exit.
// Disabling retry alone does not fix that: the *initial* attempt is what
// blocks. Several callers pass context.Background() to the returned shutdown
// func (engram, safe-pr, mergeloop, safe-merge, vroom-dispatch, wayfinder), so
// the bound has to live here rather than at each call site.
//
// Retry stays enabled — a long-lived process (agm, mergeloop) should still ride
// out a brief collector blip instead of dropping the batch — but the intervals
// are scaled to fit inside the budget. The SDK defaults (5s initial, 30s max,
// 1m elapsed) would spend the whole budget waiting to retry even once.
const (
	otlpExportTimeout        = 3 * time.Second
	otlpRetryInitialInterval = 250 * time.Millisecond
	otlpRetryMaxInterval     = 1 * time.Second
)

// parseOTLPEndpoint splits an OTLP endpoint into a gRPC dial target (host:port,
// no scheme) and whether the connection should be plaintext (insecure).
//
//   - "localhost:4317"        → ("localhost:4317", true)   // scheme-less ⇒ plaintext
//   - "http://localhost:4317" → ("localhost:4317", true)
//   - "https://tempo:4317"    → ("tempo:4317", false)
//
// Passing the resulting target via otlptracegrpc.WithEndpoint avoids the
// scheme-less-target-is-empty footgun in the exporter's env parser.
func parseOTLPEndpoint(raw string) (target string, insecure bool) {
	e := strings.TrimSpace(raw)
	e = strings.TrimSuffix(e, "/")
	switch {
	case strings.HasPrefix(e, "https://"):
		return strings.TrimPrefix(e, "https://"), false
	case strings.HasPrefix(e, "http://"):
		return strings.TrimPrefix(e, "http://"), true
	default:
		return e, true
	}
}

// InitTracer initialises the global OpenTelemetry TracerProvider.
//
// If OTEL_EXPORTER_OTLP_ENDPOINT is unset the provider is a no-op and
// the returned shutdown function is a harmless no-op too.
//
// serviceName overrides OTEL_SERVICE_NAME when the env var is empty.
func InitTracer(serviceName string) (shutdown func(context.Context) error) {
	noopShutdown := func(context.Context) error { return nil }

	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		// No collector configured — install a no-op provider so spans
		// are silently discarded.
		otel.SetTracerProvider(noop.NewTracerProvider())
		return noopShutdown
	}

	// Resolve service name: env var > explicit arg > binary name.
	if envName := os.Getenv("OTEL_SERVICE_NAME"); envName != "" {
		serviceName = envName
	} else if serviceName == "" {
		serviceName = filepath.Base(os.Args[0])
	}

	// Derive version from build info when available.
	serviceVersion := "dev"
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && s.Value != "" {
				serviceVersion = shortRevision(s.Value)
				break
			}
		}
	}

	ctx := context.Background()

	// Build explicit exporter options from the endpoint so a scheme-less
	// "localhost:4317" exports correctly instead of silently dropping spans.
	target, insecure := parseOTLPEndpoint(endpoint)
	exporterOpts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(target),
		otlptracegrpc.WithTimeout(otlpExportTimeout),
		otlptracegrpc.WithRetry(otlptracegrpc.RetryConfig{
			Enabled:         true,
			InitialInterval: otlpRetryInitialInterval,
			MaxInterval:     otlpRetryMaxInterval,
			MaxElapsedTime:  otlpExportTimeout,
		}),
	}
	if insecure {
		exporterOpts = append(exporterOpts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(ctx, exporterOpts...)
	if err != nil {
		// Fall back to no-op rather than crashing the host binary.
		otel.SetTracerProvider(noop.NewTracerProvider())
		return noopShutdown
	}

	svcRes := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(serviceVersion),
	)
	res, err := resource.Merge(resource.Default(), svcRes)
	if err != nil {
		// Schema URL conflict between SDK default and semconv — keep service name.
		res = svcRes
	}

	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	}

	// Register JSONL exporter alongside OTLP when ENGRAM_SESSION_ID is set.
	if sid := os.Getenv("ENGRAM_SESSION_ID"); sid != "" {
		if je, err := NewJSONLExporter(sid); err == nil {
			opts = append(opts, sdktrace.WithBatcher(je))
		}
	}

	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown
}

func shortRevision(revision string) string {
	if len(revision) <= 7 {
		return revision
	}
	return revision[:7]
}
