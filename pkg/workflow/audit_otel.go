package workflow

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// OTelEmitter is the narrow interface the OpenTelemetry audit sink uses
// to push events. Concrete OTel SDK wiring (collectors, exporters) lives
// outside pkg/workflow so this package keeps no SDK dependency. A
// production embedding plugs in an emitter that funnels events into the
// OTel Logs API or a custom span event.
//
// EmitEvent must be non-blocking; the runner fans many events per run
// through this sink during parallel loops.
type OTelEmitter interface {
	EmitEvent(ctx context.Context, name string, attrs map[string]any, ts time.Time) error
}

// OTelAuditSink converts AuditEvents into OTel events so dashboards and
// service-graph tools can join workflow runs to the surrounding traces.
// The conventions here mirror OpenTelemetry semantic conventions for
// "events" — the run_id is the resource attribute, the to_state is the
// event name, and the rest of the AuditEvent fields ride along as
// attributes.
type OTelAuditSink struct {
	Emitter OTelEmitter

	mu      sync.Mutex
	lastErr error
}

// Emit converts the event and forwards it to the emitter.
func (s *OTelAuditSink) Emit(ctx context.Context, ev AuditEvent) error {
	if s == nil || s.Emitter == nil {
		return nil
	}
	occurred := ev.OccurredAt
	if occurred.IsZero() {
		occurred = time.Now()
	}
	attrs := map[string]any{
		"workflow.run_id":      ev.RunID,
		"workflow.event_id":    ev.EventID,
		"workflow.from_state":  ev.FromState,
		"workflow.to_state":    ev.ToState,
		"workflow.reason":      ev.Reason,
		"workflow.actor":       ev.Actor,
	}
	if ev.NodeID != "" {
		attrs["workflow.node_id"] = ev.NodeID
	}
	if ev.AttemptNo > 0 {
		attrs["workflow.attempt_no"] = ev.AttemptNo
	}
	for k, v := range ev.Payload {
		attrs["workflow.payload."+k] = v
	}
	name := "workflow.transition." + ev.ToState
	if err := s.Emitter.EmitEvent(ctx, name, attrs, occurred); err != nil {
		s.mu.Lock()
		s.lastErr = err
		s.mu.Unlock()
		return fmt.Errorf("otel audit: %w", err)
	}
	return nil
}

// LastErr returns the last emitter error. Useful in tests.
func (s *OTelAuditSink) LastErr() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}
