package evalcase

import (
	"context"
	"errors"
	"testing"

	"github.com/vbonnet/dear-agent/internal/telemetry"
	"github.com/vbonnet/dear-agent/pkg/agenttrace"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestFromReadOnlySpans_RoundTripThroughPipeline emits real four-pillar spans
// with pkg/agenttrace, captures them with an in-memory recorder, bridges them to
// Traces, and confirms the classifier sees the injected failures — the full
// live-span → eval-case path.
func TestFromReadOnlySpans_RoundTripThroughPipeline(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(prev)

	// Root span ties the pillar spans into one trace; agenttrace helpers parent
	// off the context.
	ctx, root := tp.Tracer("test").Start(context.Background(), "run")

	// A low online eval score, attached to the active (root) span.
	telemetry.RecordEvalScore(ctx, root.SpanContext(), "correctness", 0.2)

	// Pillar 1: a tool call that retried past the ceiling and errored.
	_, tc := agenttrace.StartToolCall(ctx, "flaky_tool")
	tc.SetRetryCount(5)
	tc.End(errors.New("boom"))

	// Pillar 4: a memory query that came back with poor relevance.
	_, mo := agenttrace.StartMemoryOp(ctx, agenttrace.MemoryQuery, "engram-kb")
	mo.SetRelevanceScore(0.05)
	mo.End(nil)

	root.End()
	if err := tp.ForceFlush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	traces := FromReadOnlySpans(sr.Ended())
	if len(traces) != 1 {
		t.Fatalf("traces = %d, want 1", len(traces))
	}
	tr := traces[0]

	if got := tr.EvalScores["correctness"]; got != 0.2 {
		t.Errorf("eval score = %v, want 0.2", got)
	}
	if len(tr.Spans) != 3 {
		t.Errorf("spans = %d, want 3 (root + tool + memory)", len(tr.Spans))
	}

	// Confirm pillars were resolved from span names.
	pillars := map[Pillar]int{}
	for _, s := range tr.Spans {
		pillars[s.Pillar]++
	}
	if pillars[PillarToolCall] != 1 || pillars[PillarMemory] != 1 {
		t.Errorf("pillar counts = %+v", pillars)
	}

	v := DefaultClassifierConfig().Classify(tr)
	if !v.Problematic {
		t.Fatal("bridged trace not flagged problematic")
	}
	want := map[FailureClass]bool{ClassToolError: false, ClassStall: false, ClassMemoryError: false, ClassLowEvalScore: false}
	for _, c := range v.Classes {
		if _, ok := want[c]; ok {
			want[c] = true
		}
	}
	for c, seen := range want {
		if !seen {
			t.Errorf("expected class %q from bridged trace, got %+v", c, v.Classes)
		}
	}
}

func TestFromReadOnlySpans_GroupsByTraceID(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(prev)

	// Two independent root spans → two distinct traces.
	for _, name := range []string{"flaky_a", "flaky_b"} {
		ctx, root := tp.Tracer("test").Start(context.Background(), "run")
		_, tc := agenttrace.StartToolCall(ctx, name)
		tc.End(errors.New("boom"))
		root.End()
	}
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	traces := FromReadOnlySpans(sr.Ended())
	if len(traces) != 2 {
		t.Fatalf("traces = %d, want 2", len(traces))
	}
	for _, tr := range traces {
		if tr.TraceID == "" {
			t.Error("empty trace id")
		}
	}
}

func TestEnrichTrace(t *testing.T) {
	traces := []Trace{{TraceID: "t1"}, {TraceID: "t2"}}
	traces = EnrichTrace(traces, map[string]TraceMeta{
		"t1": {Task: "do x", SuccessCriteria: "x done", Outcome: OutcomeError},
	})
	if traces[0].Task != "do x" || traces[0].Outcome != OutcomeError {
		t.Errorf("t1 not enriched: %+v", traces[0])
	}
	if traces[1].Task != "" {
		t.Errorf("t2 should be untouched: %+v", traces[1])
	}
}
