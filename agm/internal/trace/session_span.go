package trace

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelTrace "go.opentelemetry.io/otel/trace"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

const tracerName = "agm/session"

// RecordSessionLifecycleSpan emits a single OTel span covering the full session
// lifetime from m.CreatedAt to endTime.
//
// The span uses the global tracer provider initialised by otelsetup.InitTracer.
// When OTEL_EXPORTER_OTLP_ENDPOINT is not set the provider is a no-op and the
// call is effectively free.
//
// exitCode is the process exit code; pass -1 when it is not available (e.g.
// when called from a state-reporter hook that does not receive the exit code).
func RecordSessionLifecycleSpan(ctx context.Context, m *manifest.Manifest, endTime time.Time, exitCode int) {
	startTime := m.CreatedAt
	if startTime.IsZero() {
		startTime = endTime
	}

	tracer := otel.Tracer(tracerName)
	_, span := tracer.Start(ctx, "session.lifecycle",
		otelTrace.WithTimestamp(startTime),
	)

	span.SetAttributes(
		attribute.String("session.id", m.SessionID),
		attribute.String("session.name", m.Name),
		attribute.String("session.model", m.Model),
		attribute.String("session.harness", m.Harness),
		attribute.String("session.workspace", m.Workspace),
		attribute.Int("session.exit_code", exitCode),
		attribute.Int64("session.duration_ms", endTime.Sub(startTime).Milliseconds()),
	)

	span.End(otelTrace.WithTimestamp(endTime))
}
