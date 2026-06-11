package lifecycle_test

import (
	"context"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/lifecycle"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

func setupTestTracer(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(noop.NewTracerProvider())
		exp.Reset()
	})
	return exp
}

func TestRecordSessionLifecycleSpan_STOPPED(t *testing.T) {
	exp := setupTestTracer(t)

	lifecycle.RecordSessionLifecycleSpan(context.Background(), "my-session", "STOPPED", "stop-hook")

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	if s.Name != "session.lifecycle" {
		t.Errorf("span name = %q, want %q", s.Name, "session.lifecycle")
	}
	wantAttrs := map[string]string{
		"session.name":         "my-session",
		"session.state":        "STOPPED",
		"session.state.source": "stop-hook",
	}
	for _, attr := range s.Attributes {
		if want, ok := wantAttrs[string(attr.Key)]; ok {
			if got := attr.Value.AsString(); got != want {
				t.Errorf("attr %s = %q, want %q", attr.Key, got, want)
			}
			delete(wantAttrs, string(attr.Key))
		}
	}
	if len(wantAttrs) > 0 {
		t.Errorf("missing attributes: %v", wantAttrs)
	}
}

func TestRecordSessionLifecycleSpan_OFFLINE(t *testing.T) {
	exp := setupTestTracer(t)

	lifecycle.RecordSessionLifecycleSpan(context.Background(), "other-session", "OFFLINE", "manual")

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name != "session.lifecycle" {
		t.Errorf("span name = %q, want %q", spans[0].Name, "session.lifecycle")
	}
}

func TestRecordSessionLifecycleSpan_NonTerminalNoSpan(t *testing.T) {
	exp := setupTestTracer(t)

	for _, state := range []string{"READY", "THINKING", "PERMISSION_PROMPT", "COMPACTING"} {
		lifecycle.RecordSessionLifecycleSpan(context.Background(), "s", state, "hook")
	}

	if len(exp.GetSpans()) != 0 {
		t.Errorf("expected 0 spans for non-terminal states, got %d", len(exp.GetSpans()))
	}
}
