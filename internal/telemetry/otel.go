// This file provides the OpenTelemetry metrics bootstrap.
//
// Tracing bootstrap already lives in pkg/otelsetup (InitTracer, plus the
// JSONL file fallback exporter) and is wired into the agm/engram binaries.
// This file adds the metrics counterpart — InitMeter — using the same OTLP
// gRPC transport so traces and metrics flow to the same collector
// (Grafana Tempo/Mimir at the OTEL_EXPORTER_OTLP_ENDPOINT). When no endpoint
// is configured the provider is a no-op, so instrumented code runs unchanged
// without a collector.

package telemetry

import (
	"context"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// OTLP export budget, kept in step with pkg/otelsetup (see the rationale on the
// matching constants there). otlpExportTimeout bounds a whole export — the
// initial gRPC attempt and every retry — so a black-holed collector cannot
// stall MeterProvider.Shutdown for the exporter's 10s default on process exit.
// Retry stays enabled so a brief collector blip still recovers, with intervals
// scaled to fit inside the budget.
const (
	otlpExportTimeout        = 3 * time.Second
	otlpRetryInitialInterval = 250 * time.Millisecond
	otlpRetryMaxInterval     = 1 * time.Second
)

// Option configures the OTel bootstrap (currently the meter; the signature is
// shared so tracing can adopt it later without churn).
type Option func(*config)

type config struct {
	endpoint string
}

// WithEndpoint overrides the OTLP collector endpoint (e.g. "localhost:4317").
// When unset the OTEL_EXPORTER_OTLP_ENDPOINT environment variable is used, and
// when that is also empty a no-op provider is installed.
func WithEndpoint(endpoint string) Option {
	return func(c *config) { c.endpoint = endpoint }
}

// meterProvider holds the active SDK MeterProvider so Shutdown can flush it.
// Guarded by mu because InitMeter and Shutdown may race on process teardown.
var (
	mu            sync.Mutex
	meterProvider *sdkmetric.MeterProvider
)

// InitMeter initialises the global OpenTelemetry MeterProvider.
//
// If neither WithEndpoint nor OTEL_EXPORTER_OTLP_ENDPOINT is set, a no-op
// provider is installed and the returned shutdown func is a harmless no-op.
// serviceName overrides OTEL_SERVICE_NAME when that env var is empty.
//
// The returned shutdown func is equivalent to calling Shutdown; it is returned
// for symmetry with otelsetup.InitTracer so callers can defer it directly.
func InitMeter(serviceName string, opts ...Option) (shutdown func(context.Context) error, err error) {
	noopShutdown := func(context.Context) error { return nil }

	cfg := config{endpoint: os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")}
	for _, o := range opts {
		o(&cfg)
	}

	if cfg.endpoint == "" {
		// No collector configured — discard metrics silently.
		otel.SetMeterProvider(noop.NewMeterProvider())
		return noopShutdown, nil
	}

	// Resolve service name: env var > explicit arg > binary name.
	if envName := os.Getenv("OTEL_SERVICE_NAME"); envName != "" {
		serviceName = envName
	} else if serviceName == "" {
		serviceName = filepath.Base(os.Args[0])
	}

	ctx := context.Background()

	// Normalize endpoint: strip http:// or https:// scheme — WithEndpoint expects
	// bare host:port for the gRPC transport.
	grpcEndpoint := cfg.endpoint
	if i := strings.Index(grpcEndpoint, "://"); i >= 0 {
		grpcEndpoint = grpcEndpoint[i+3:]
	}
	exporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(grpcEndpoint),
		otlpmetricgrpc.WithTimeout(otlpExportTimeout),
		otlpmetricgrpc.WithRetry(otlpmetricgrpc.RetryConfig{
			Enabled:         true,
			InitialInterval: otlpRetryInitialInterval,
			MaxInterval:     otlpRetryMaxInterval,
			MaxElapsedTime:  otlpExportTimeout,
		}),
	)
	if err != nil {
		// Fall back to no-op rather than crashing the host binary; the
		// exporter error is deliberately swallowed so a missing collector
		// never breaks an agm/engram invocation.
		otel.SetMeterProvider(noop.NewMeterProvider())
		return noopShutdown, nil //nolint:nilerr // graceful no-op fallback
	}

	svcRes := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(serviceVersion()),
	)
	res, mergeErr := resource.Merge(resource.Default(), svcRes)
	if mergeErr != nil {
		// Schema URL conflict — keep service name rather than silently losing it.
		res = svcRes
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	mu.Lock()
	meterProvider = mp
	mu.Unlock()

	return Shutdown, nil
}

// Shutdown flushes and stops the metrics provider installed by InitMeter.
// It is safe to call when InitMeter installed a no-op provider (it is a no-op).
//
// Trace shutdown is handled by the func returned from otelsetup.InitTracer;
// callers that init both should defer both.
func Shutdown(ctx context.Context) error {
	mu.Lock()
	mp := meterProvider
	meterProvider = nil
	mu.Unlock()

	if mp == nil {
		return nil
	}
	return mp.Shutdown(ctx)
}

// serviceVersion derives a short version string from the embedded VCS revision,
// matching otelsetup's tracer resource so traces and metrics agree.
func serviceVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				return s.Value[:7]
			}
		}
	}
	return "dev"
}
