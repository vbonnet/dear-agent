// Package otelsetup provides OpenTelemetry bootstrap for engram services.
//
// It configures a TracerProvider with an OTLP gRPC exporter pointed at
// Grafana Tempo (default localhost:4317). When OTEL_EXPORTER_OTLP_ENDPOINT
// is not set, a no-op TracerProvider is returned so instrumented code
// works without a collector running.
package otelsetup

import (
	"context"
	"os"
	"path/filepath"
	"runtime/debug"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace/noop"
)

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
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				serviceVersion = s.Value[:7]
				break
			}
		}
	}

	ctx := context.Background()

	exporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		// Fall back to no-op rather than crashing the host binary.
		otel.SetTracerProvider(noop.NewTracerProvider())
		return noopShutdown
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
	)
	if err != nil {
		res = resource.Default()
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
