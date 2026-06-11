// Package lifecycle emits OTel spans for session lifecycle transitions.
package lifecycle

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// terminalStates are states that mark the end of a session's active life.
var terminalStates = map[string]bool{
	"STOPPED": true,
	"OFFLINE": true,
}

// RecordSessionLifecycleSpan emits a "session.lifecycle" span for terminal state
// transitions (STOPPED, OFFLINE). Non-terminal states are silently ignored so
// callers can unconditionally call this after every state update.
func RecordSessionLifecycleSpan(ctx context.Context, sessionName, state, source string) {
	if !terminalStates[state] {
		return
	}

	_, span := otel.Tracer("agm").Start(ctx, "session.lifecycle",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	span.SetAttributes(
		attribute.String("session.name", sessionName),
		attribute.String("session.state", state),
		attribute.String("session.state.source", source),
	)
	span.SetStatus(codes.Ok, "")
}
