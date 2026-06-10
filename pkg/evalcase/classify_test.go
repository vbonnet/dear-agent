package evalcase

import (
	"testing"

	"github.com/vbonnet/dear-agent/pkg/agenttrace"
)

func toolSpan(name string, errType string, retries int) Span {
	s := Span{
		Pillar: PillarToolCall,
		Name:   agenttrace.SpanToolCall,
		Attributes: map[string]any{
			agenttrace.AttrToolName:       name,
			agenttrace.AttrToolRetryCount: retries,
		},
	}
	if errType != "" {
		s.ErrorType = errType
		s.StatusError = true
	}
	return s
}

func memSpan(op agenttrace.MemoryOp, store string, relevance float64) Span {
	return Span{
		Pillar: PillarMemory,
		Name:   agenttrace.SpanMemory,
		Attributes: map[string]any{
			agenttrace.AttrMemoryOperation: string(op),
			agenttrace.AttrMemoryStore:     store,
			agenttrace.AttrMemoryRelevance: relevance,
		},
	}
}

func TestClassify_CleanTraceNotProblematic(t *testing.T) {
	tr := Trace{
		TraceID: "abc",
		Outcome: OutcomeSuccess,
		Spans:   []Span{toolSpan("search", "", 0)},
		EvalScores: map[string]float64{
			"correctness": 0.95,
		},
	}
	v := DefaultClassifierConfig().Classify(tr)
	if v.Problematic {
		t.Fatalf("clean trace flagged problematic: %+v", v)
	}
	if v.Primary != ClassNone {
		t.Fatalf("clean trace primary = %q, want none", v.Primary)
	}
}

func TestClassify_ToolError(t *testing.T) {
	tr := Trace{
		TraceID: "t1",
		Outcome: OutcomeError,
		Spans:   []Span{toolSpan("write_file", "*os.PathError", 0)},
	}
	v := DefaultClassifierConfig().Classify(tr)
	if !v.Problematic {
		t.Fatal("tool error not flagged")
	}
	if v.Primary != ClassToolError {
		t.Fatalf("primary = %q, want tool_error", v.Primary)
	}
	// error_outcome must be suppressed when a specific span error explains it.
	for _, c := range v.Classes {
		if c == ClassErrorOutcome {
			t.Fatalf("error_outcome should be suppressed when tool_error present: %+v", v.Classes)
		}
	}
}

func TestClassify_StallFromRetries(t *testing.T) {
	tr := Trace{
		TraceID: "t2",
		Spans:   []Span{toolSpan("flaky", "", 5)},
	}
	v := DefaultClassifierConfig().Classify(tr)
	if v.Primary != ClassStall {
		t.Fatalf("primary = %q, want stall", v.Primary)
	}
}

func TestClassify_StallFromOutcome(t *testing.T) {
	tr := Trace{TraceID: "t3", Outcome: OutcomeStalled}
	v := DefaultClassifierConfig().Classify(tr)
	if !v.Problematic || v.Primary != ClassStall {
		t.Fatalf("stalled outcome: got problematic=%v primary=%q", v.Problematic, v.Primary)
	}
}

func TestClassify_LowEvalScore(t *testing.T) {
	tr := Trace{
		TraceID:    "t4",
		Outcome:    OutcomeSuccess,
		EvalScores: map[string]float64{"helpfulness": 0.3},
	}
	v := DefaultClassifierConfig().Classify(tr)
	if v.Primary != ClassLowEvalScore {
		t.Fatalf("primary = %q, want low_eval_score", v.Primary)
	}
}

func TestClassify_MemoryLowRelevance(t *testing.T) {
	tr := Trace{
		TraceID: "t5",
		Spans:   []Span{memSpan(agenttrace.MemoryQuery, "engram-kb", 0.05)},
	}
	v := DefaultClassifierConfig().Classify(tr)
	if v.Primary != ClassMemoryError {
		t.Fatalf("primary = %q, want memory_error", v.Primary)
	}
}

func TestClassify_MemoryWriteRelevanceIgnored(t *testing.T) {
	// A write's relevance score is meaningless and must not flag a memory error.
	tr := Trace{
		TraceID: "t6",
		Outcome: OutcomeSuccess,
		Spans:   []Span{memSpan(agenttrace.MemoryWrite, "engram-kb", 0.0)},
	}
	v := DefaultClassifierConfig().Classify(tr)
	if v.Problematic {
		t.Fatalf("memory write flagged on relevance: %+v", v)
	}
}

func TestClassify_ErrorOutcomeFallback(t *testing.T) {
	// Error outcome with no span-level cause → generic error_outcome.
	tr := Trace{TraceID: "t7", Outcome: OutcomeError}
	v := DefaultClassifierConfig().Classify(tr)
	if v.Primary != ClassErrorOutcome {
		t.Fatalf("primary = %q, want error_outcome", v.Primary)
	}
}

func TestClassify_PrimaryPrefersSpecific(t *testing.T) {
	// Tool error + low eval score + stall: primary should be the most specific
	// (tool_error), but all classes recorded.
	tr := Trace{
		TraceID:    "t8",
		Outcome:    OutcomeError,
		Spans:      []Span{toolSpan("x", "boom", 9)},
		EvalScores: map[string]float64{"q": 0.1},
	}
	v := DefaultClassifierConfig().Classify(tr)
	if v.Primary != ClassToolError {
		t.Fatalf("primary = %q, want tool_error", v.Primary)
	}
	want := map[FailureClass]bool{ClassToolError: false, ClassStall: false, ClassLowEvalScore: false}
	for _, c := range v.Classes {
		if _, ok := want[c]; ok {
			want[c] = true
		}
	}
	for c, seen := range want {
		if !seen {
			t.Fatalf("expected class %q in %+v", c, v.Classes)
		}
	}
}

func TestClassify_RetryThresholdDisabled(t *testing.T) {
	cfg := DefaultClassifierConfig()
	cfg.MaxToolRetries = 0 // disable
	tr := Trace{TraceID: "t9", Outcome: OutcomeSuccess, Spans: []Span{toolSpan("x", "", 100)}}
	v := cfg.Classify(tr)
	if v.Problematic {
		t.Fatalf("retry check disabled but still flagged: %+v", v)
	}
}
