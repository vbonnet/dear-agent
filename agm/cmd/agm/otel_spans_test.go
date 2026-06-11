package main

import (
	"context"
	"fmt"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// setupTestTracer installs an in-memory span recorder as the global provider
// and returns the recorder plus a cleanup that restores the prior provider.
func setupTestTracer(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	old := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(old) })
	return recorder
}

// kvMap converts a span's attribute slice to a string→string map for easy lookup.
func kvMap(attrs []attribute.KeyValue) map[string]string {
	m := make(map[string]string, len(attrs))
	for _, kv := range attrs {
		m[string(kv.Key)] = fmt.Sprintf("%v", kv.Value.AsInterface())
	}
	return m
}

func TestStartRegisterSpan(t *testing.T) {
	recorder := setupTestTracer(t)

	ctx, span := startRegisterSpan(context.Background(), "abc-uuid-123", "oss")
	span.End()

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	if s.Name() != "agm.session.register" {
		t.Errorf("span name = %q, want %q", s.Name(), "agm.session.register")
	}
	if !s.SpanContext().IsValid() {
		t.Error("span context should be valid")
	}
	if ctx == nil {
		t.Error("returned context should not be nil")
	}

	attrs := kvMap(s.Attributes())
	if v := attrs["session.uuid"]; v != "abc-uuid-123" {
		t.Errorf("session.uuid = %q, want %q", v, "abc-uuid-123")
	}
	if v := attrs["operation"]; v != "register" {
		t.Errorf("operation = %q, want %q", v, "register")
	}
	if v := attrs["workspace"]; v != "oss" {
		t.Errorf("workspace = %q, want %q", v, "oss")
	}
}

func TestStartStateSetSpan(t *testing.T) {
	recorder := setupTestTracer(t)

	ctx, span := startStateSetSpan(context.Background(), "my-session", "READY", "hook")
	span.End()

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	if s.Name() != "agm.session.state_set" {
		t.Errorf("span name = %q, want %q", s.Name(), "agm.session.state_set")
	}
	if ctx == nil {
		t.Error("returned context should not be nil")
	}

	attrs := kvMap(s.Attributes())
	if v := attrs["session.name"]; v != "my-session" {
		t.Errorf("session.name = %q, want %q", v, "my-session")
	}
	if v := attrs["session.state"]; v != "READY" {
		t.Errorf("session.state = %q, want %q", v, "READY")
	}
	if v := attrs["state.source"]; v != "hook" {
		t.Errorf("state.source = %q, want %q", v, "hook")
	}
	if v := attrs["operation"]; v != "state_set" {
		t.Errorf("operation = %q, want %q", v, "state_set")
	}
}

func TestStartListSpan(t *testing.T) {
	recorder := setupTestTracer(t)

	ctx, span := startListSpan(context.Background(), true, 3)
	span.End()

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	if s.Name() != "agm.session.list" {
		t.Errorf("span name = %q, want %q", s.Name(), "agm.session.list")
	}
	if ctx == nil {
		t.Error("returned context should not be nil")
	}

	attrs := kvMap(s.Attributes())
	if v := attrs["filter.all"]; v != "true" {
		t.Errorf("filter.all = %q, want %q", v, "true")
	}
	if v := attrs["filter.tag_count"]; v != "3" {
		t.Errorf("filter.tag_count = %q, want %q", v, "3")
	}
	if v := attrs["operation"]; v != "list" {
		t.Errorf("operation = %q, want %q", v, "list")
	}
}

func TestStartKillSpan_SoftKill(t *testing.T) {
	recorder := setupTestTracer(t)

	ctx, span := startKillSpan(context.Background(), "target-session", false, false)
	span.End()

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	if s.Name() != "agm.session.kill" {
		t.Errorf("span name = %q, want %q", s.Name(), "agm.session.kill")
	}
	if ctx == nil {
		t.Error("returned context should not be nil")
	}

	attrs := kvMap(s.Attributes())
	if v := attrs["session.name"]; v != "target-session" {
		t.Errorf("session.name = %q, want %q", v, "target-session")
	}
	if v := attrs["kill.mode"]; v != "soft" {
		t.Errorf("kill.mode = %q, want %q", v, "soft")
	}
	if v := attrs["kill.force"]; v != "false" {
		t.Errorf("kill.force = %q, want %q", v, "false")
	}
}

func TestStartKillSpan_HardKill(t *testing.T) {
	recorder := setupTestTracer(t)

	_, span := startKillSpan(context.Background(), "stuck-session", true, true)
	span.End()

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	attrs := kvMap(spans[0].Attributes())
	if v := attrs["kill.mode"]; v != "hard" {
		t.Errorf("kill.mode = %q, want %q", v, "hard")
	}
	if v := attrs["kill.force"]; v != "true" {
		t.Errorf("kill.force = %q, want %q", v, "true")
	}
}
