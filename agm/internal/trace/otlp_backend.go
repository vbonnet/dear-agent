package trace

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// OTLPBackend exports TraceRecords as OpenTelemetry spans via gRPC.
type OTLPBackend struct {
	provider *sdktrace.TracerProvider
	tracer   trace.Tracer
}

// OTLPConfig configures the OTLP backend.
type OTLPConfig struct {
	Endpoint string // gRPC endpoint (e.g. "localhost:4317")
	Insecure bool   // use insecure connection (no TLS)
}

// NewOTLPBackend creates a backend that exports spans to an OTLP collector.
func NewOTLPBackend(ctx context.Context, cfg OTLPConfig) (*OTLPBackend, error) {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
	}
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
	)

	return &OTLPBackend{
		provider: provider,
		tracer:   provider.Tracer("agm.audit"),
	}, nil
}

func (b *OTLPBackend) Write(ctx context.Context, rec *TraceRecord) error {
	_, span := b.tracer.Start(ctx, string(rec.EventType),
		trace.WithTimestamp(rec.Timestamp),
	)

	span.SetAttributes(
		attribute.String("session.id", rec.SessionID),
		attribute.String("event.type", string(rec.EventType)),
	)

	// Add payload fields as span attributes
	for k, v := range rec.Payload {
		span.SetAttributes(attribute.String("payload."+k, fmt.Sprintf("%v", v)))
	}

	span.End(trace.WithTimestamp(rec.Timestamp))
	return nil
}

func (b *OTLPBackend) Flush(ctx context.Context) error {
	return b.provider.ForceFlush(ctx)
}

func (b *OTLPBackend) Close() error {
	return b.provider.Shutdown(context.Background())
}

// compile-time check
var _ Backend = (*OTLPBackend)(nil)
