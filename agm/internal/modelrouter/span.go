package modelrouter

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelTrace "go.opentelemetry.io/otel/trace"
)

const tracerName = "agm/modelrouter"

// RecordRoutingDecision emits an OTel span recording the model routing
// decision. The span is intentionally short (start==end) since it captures a
// point-in-time decision, not a duration.
//
// Attributes emitted:
//
//	router.harness      — harness name (claude-code, gemini-cli, …)
//	router.tier         — cheap | mid | expensive
//	router.model        — harness-specific model alias chosen
//	router.reason       — human-readable reason for the tier
//	router.explicit     — true when --model-tier was passed explicitly
func RecordRoutingDecision(ctx context.Context, harness string, d *Decision) {
	if d == nil {
		return
	}
	tracer := otel.Tracer(tracerName)
	now := time.Now()
	_, span := tracer.Start(ctx, "model.routing_decision",
		otelTrace.WithTimestamp(now),
	)
	span.SetAttributes(
		attribute.String("router.harness", harness),
		attribute.String("router.tier", string(d.Tier)),
		attribute.String("router.model", d.Model),
		attribute.String("router.reason", d.Reason),
		attribute.Bool("router.explicit", d.ExplicitTier),
	)
	span.End(otelTrace.WithTimestamp(now))
}
