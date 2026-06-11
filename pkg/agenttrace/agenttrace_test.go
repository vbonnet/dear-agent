package agenttrace

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// newRecorder installs an in-memory span recorder as the global provider and
// returns it. It restores the previous provider via t.Cleanup so tests don't
// leak tracer state into one another.
func newRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	prev := otel.GetTracerProvider()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	return sr
}

// attrMap flattens a span's attributes into a lookup keyed by attribute name.
func attrMap(kvs []attribute.KeyValue) map[attribute.Key]attribute.Value {
	m := make(map[attribute.Key]attribute.Value, len(kvs))
	for _, kv := range kvs {
		m[kv.Key] = kv.Value
	}
	return m
}

func soleSpan(t *testing.T, sr *tracetest.SpanRecorder) sdktrace.ReadOnlySpan {
	t.Helper()
	ended := sr.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected exactly 1 ended span, got %d", len(ended))
	}
	return ended[0]
}

func TestToolCallSpan_RecordsAllFields(t *testing.T) {
	sr := newRecorder(t)

	ctx, span := StartToolCall(context.Background(), "search_files")
	span.SetCallID("call_abc123")
	span.SetArguments(`{"pattern":"foo"}`)
	span.SetOutput("match: foo.go:42")
	span.IncRetry()
	span.IncRetry()
	span.End(nil)
	_ = ctx

	s := soleSpan(t, sr)
	if s.Name() != SpanToolCall {
		t.Errorf("span name = %q, want %q", s.Name(), SpanToolCall)
	}
	m := attrMap(s.Attributes())
	if got := m[attrToolName].AsString(); got != "search_files" {
		t.Errorf("tool name = %q, want search_files", got)
	}
	if got := m[attrToolCallID].AsString(); got != "call_abc123" {
		t.Errorf("call id = %q", got)
	}
	if got := m[attrToolArguments].AsString(); got != `{"pattern":"foo"}` {
		t.Errorf("arguments = %q", got)
	}
	if got := m[attrToolOutput].AsString(); got != "match: foo.go:42" {
		t.Errorf("output = %q", got)
	}
	if got := m[attrToolRetryCount].AsInt64(); got != 2 {
		t.Errorf("retry count = %d, want 2", got)
	}
	if _, ok := m[attrToolDurationMS]; !ok {
		t.Errorf("expected duration attribute to be set")
	}
	if s.Status().Code != codes.Unset {
		t.Errorf("status = %v, want Unset for clean call", s.Status().Code)
	}
}

func TestToolCallSpan_SetRetryCountAbsolute(t *testing.T) {
	sr := newRecorder(t)
	_, span := StartToolCall(context.Background(), "t")
	span.SetRetryCount(5)
	span.End(nil)

	m := attrMap(soleSpan(t, sr).Attributes())
	if got := m[attrToolRetryCount].AsInt64(); got != 5 {
		t.Errorf("retry count = %d, want 5", got)
	}
}

func TestToolCallSpan_ErrorState(t *testing.T) {
	sr := newRecorder(t)
	_, span := StartToolCall(context.Background(), "t")
	span.End(errors.New("boom"))

	s := soleSpan(t, sr)
	if s.Status().Code != codes.Error {
		t.Errorf("status = %v, want Error", s.Status().Code)
	}
	m := attrMap(s.Attributes())
	if got := m[attrErrorType].AsString(); !strings.Contains(got, "errorString") {
		t.Errorf("error.type = %q, want *errors.errorString", got)
	}
	if len(s.Events()) == 0 {
		t.Errorf("expected an exception event from RecordError")
	}
}

func TestReasoningSpan_RecordsFields(t *testing.T) {
	sr := newRecorder(t)
	_, span := StartReasoning(context.Background(), "wayfinder/PLAN")
	span.SetPlan("split god-file into 3 packages")
	span.SetAction("create pkg/agenttrace")
	span.SetObservation("compiles clean")
	span.SetNextDecision("add tests")
	span.End(nil)

	s := soleSpan(t, sr)
	if s.Name() != SpanReasoning {
		t.Errorf("name = %q, want %q", s.Name(), SpanReasoning)
	}
	m := attrMap(s.Attributes())
	cases := map[attribute.Key]string{
		attrReasoningStep:         "wayfinder/PLAN",
		attrReasoningPlan:         "split god-file into 3 packages",
		attrReasoningAction:       "create pkg/agenttrace",
		attrReasoningObservation:  "compiles clean",
		attrReasoningNextDecision: "add tests",
	}
	for k, want := range cases {
		if got := m[k].AsString(); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestStateTransitionSpan_RecordsFields(t *testing.T) {
	sr := newRecorder(t)
	_, span := StartStateTransition(context.Background(), "active", "compacted")
	span.SetContextEdits("dropped 40 stale tool results")
	span.SetHandoffPayload(`{"summary":"..."}`)
	span.End(nil)

	s := soleSpan(t, sr)
	if s.Name() != SpanStateTransition {
		t.Errorf("name = %q, want %q", s.Name(), SpanStateTransition)
	}
	m := attrMap(s.Attributes())
	if got := m[attrStateFrom].AsString(); got != "active" {
		t.Errorf("from = %q", got)
	}
	if got := m[attrStateTo].AsString(); got != "compacted" {
		t.Errorf("to = %q", got)
	}
	if got := m[attrStateContextEdits].AsString(); got != "dropped 40 stale tool results" {
		t.Errorf("context edits = %q", got)
	}
	if got := m[attrStateHandoff].AsString(); got != `{"summary":"..."}` {
		t.Errorf("handoff = %q", got)
	}
}

func TestMemoryOpSpan_RecordsFields(t *testing.T) {
	sr := newRecorder(t)
	_, span := StartMemoryOp(context.Background(), MemoryQuery, "engram-kb")
	span.SetQuery("otel four pillar schema")
	span.SetRelevanceScore(0.87)
	span.SetFreshness(90 * time.Second)
	span.SetResultCount(3)
	span.End(nil)

	s := soleSpan(t, sr)
	if s.Name() != SpanMemory {
		t.Errorf("name = %q, want %q", s.Name(), SpanMemory)
	}
	m := attrMap(s.Attributes())
	if got := m[attrMemoryOperation].AsString(); got != "query" {
		t.Errorf("operation = %q, want query", got)
	}
	if got := m[attrMemoryStore].AsString(); got != "engram-kb" {
		t.Errorf("store = %q", got)
	}
	if got := m[attrMemoryQuery].AsString(); got != "otel four pillar schema" {
		t.Errorf("query = %q", got)
	}
	if got := m[attrMemoryRelevance].AsFloat64(); got != 0.87 {
		t.Errorf("relevance = %v, want 0.87", got)
	}
	if got := m[attrMemoryFreshnessSeconds].AsInt64(); got != 90 {
		t.Errorf("freshness = %d, want 90", got)
	}
	if got := m[attrMemoryResultCount].AsInt64(); got != 3 {
		t.Errorf("result count = %d, want 3", got)
	}
}

func TestEmptyStringAttributesAreSkipped(t *testing.T) {
	sr := newRecorder(t)
	_, span := StartReasoning(context.Background(), "step")
	span.SetPlan("") // empty — must not be recorded
	span.End(nil)

	m := attrMap(soleSpan(t, sr).Attributes())
	if _, ok := m[attrReasoningPlan]; ok {
		t.Errorf("empty plan attribute should be skipped")
	}
}

func TestTruncateLongAttribute(t *testing.T) {
	sr := newRecorder(t)
	big := strings.Repeat("x", maxAttrLen+500)
	_, span := StartToolCall(context.Background(), "t")
	span.SetOutput(big)
	span.End(nil)

	got := attrMap(soleSpan(t, sr).Attributes())[attrToolOutput].AsString()
	if len(got) <= maxAttrLen {
		t.Fatalf("expected truncated value longer than cap due to marker, got len %d", len(got))
	}
	if !strings.Contains(got, "truncated 500 bytes") {
		t.Errorf("expected truncation marker, got tail %q", got[len(got)-40:])
	}
}

func TestTruncateUnit(t *testing.T) {
	if got := truncate("short"); got != "short" {
		t.Errorf("short string should pass through, got %q", got)
	}
	if got := truncate(strings.Repeat("a", maxAttrLen)); len(got) != maxAttrLen {
		t.Errorf("value at exactly the cap should not be truncated, len = %d", len(got))
	}
}

func TestTruncateNeverSplitsRune(t *testing.T) {
	// A run of 3-byte runes (… = U+2026, 3 bytes) so the byte cap at maxAttrLen
	// would land mid-rune if truncation were byte-oriented.
	s := strings.Repeat("…", maxAttrLen) // 3*maxAttrLen bytes
	got := truncate(s)
	prefix := strings.TrimSuffix(got, got[strings.LastIndex(got, "…[truncated"):])
	if !utf8.ValidString(prefix) {
		t.Fatalf("truncated prefix is not valid UTF-8")
	}
	if !utf8.ValidString(got) {
		t.Fatalf("truncated value is not valid UTF-8")
	}
	if len(prefix) > maxAttrLen {
		t.Fatalf("prefix %d bytes exceeds cap %d", len(prefix), maxAttrLen)
	}
}

func TestInstrumentToolCall_PanicClosesSpanAndReraises(t *testing.T) {
	sr := newRecorder(t)
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic to propagate")
		}
		// Span must still have been ended despite the panic.
		s := soleSpan(t, sr)
		if s.Status().Code != codes.Error {
			t.Errorf("panicked span should be marked Error, got %v", s.Status().Code)
		}
	}()
	_, _ = InstrumentToolCall(context.Background(), "boom", "",
		func(ctx context.Context) (string, error) { panic("kaboom") })
}

func TestInstrumentToolCall(t *testing.T) {
	sr := newRecorder(t)
	out, err := InstrumentToolCall(context.Background(), "echo", `{"v":1}`,
		func(ctx context.Context) (string, error) {
			return "result", nil
		})
	if err != nil || out != "result" {
		t.Fatalf("got (%q, %v), want (result, nil)", out, err)
	}
	m := attrMap(soleSpan(t, sr).Attributes())
	if m[attrToolArguments].AsString() != `{"v":1}` {
		t.Errorf("arguments not recorded")
	}
	if m[attrToolOutput].AsString() != "result" {
		t.Errorf("output not recorded")
	}
}

func TestInstrumentToolCall_PropagatesError(t *testing.T) {
	sr := newRecorder(t)
	wantErr := errors.New("fail")
	_, err := InstrumentToolCall(context.Background(), "t", "",
		func(ctx context.Context) (string, error) { return "", wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if soleSpan(t, sr).Status().Code != codes.Error {
		t.Errorf("span should be marked Error")
	}
}

func TestInstrumentReasoningAndMemory(t *testing.T) {
	sr := newRecorder(t)

	err := InstrumentReasoning(context.Background(), "decide",
		func(ctx context.Context, span *ReasoningSpan) error {
			span.SetAction("pick branch A")
			return nil
		})
	if err != nil {
		t.Fatalf("reasoning err = %v", err)
	}

	err = InstrumentMemoryOp(context.Background(), MemoryRead, "auto-memory",
		func(ctx context.Context, span *MemoryOpSpan) error {
			span.SetResultCount(1)
			return nil
		})
	if err != nil {
		t.Fatalf("memory err = %v", err)
	}

	if len(sr.Ended()) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(sr.Ended()))
	}
}

func TestNoopProviderDoesNotPanic(t *testing.T) {
	// With no recorder installed the default provider is a no-op; the helpers
	// must still be safe to call and must not panic.
	ctx, span := StartToolCall(context.Background(), "t")
	span.SetArguments("a")
	span.End(nil)

	_, r := StartReasoning(ctx, "s")
	r.End(errors.New("x"))

	_, st := StartStateTransition(ctx, "a", "b")
	st.End(nil)

	_, mem := StartMemoryOp(ctx, MemoryWrite, "store")
	mem.SetRelevanceScore(0.5)
	mem.End(nil)
}
