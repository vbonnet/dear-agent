package evalcase

import (
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/agenttrace"
)

func TestExtract_PopulatesRequiredFields(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	tr := Trace{
		TraceID:         "deadbeef",
		Task:            "rename ValidateModel across the codebase",
		SuccessCriteria: "all call sites updated, build green",
		Outcome:         OutcomeError,
		Spans:           []Span{toolSpan("apply_patch", "*fs.PathError", 0)},
	}
	v := DefaultClassifierConfig().Classify(tr)
	c := Extract(tr, v, ExtractConfig{}, now)

	if c.SchemaVersion != SchemaVersion {
		t.Errorf("schema version = %d, want %d", c.SchemaVersion, SchemaVersion)
	}
	if c.ID != "deadbeef" || c.SourceTraceID != "deadbeef" {
		t.Errorf("id/source = %q/%q", c.ID, c.SourceTraceID)
	}
	if c.Task != tr.Task {
		t.Errorf("task = %q", c.Task)
	}
	if c.ExpectedBehavior != tr.SuccessCriteria {
		t.Errorf("expected = %q, want success criteria", c.ExpectedBehavior)
	}
	if !strings.Contains(c.ActualBehavior, "error") {
		t.Errorf("actual behaviour missing outcome: %q", c.ActualBehavior)
	}
	if c.Classification != ClassToolError {
		t.Errorf("classification = %q", c.Classification)
	}
	if !c.GeneratedAt.Equal(now) {
		t.Errorf("generatedAt = %v, want %v", c.GeneratedAt, now)
	}
	if len(c.SpanExcerpts) == 0 {
		t.Fatal("no span excerpts")
	}
}

func TestExtract_DefaultExpectedBehaviorWhenNoCriteria(t *testing.T) {
	tr := Trace{TraceID: "x", Outcome: OutcomeError}
	v := DefaultClassifierConfig().Classify(tr)
	c := Extract(tr, v, ExtractConfig{}, time.Now())
	if strings.TrimSpace(c.ExpectedBehavior) == "" {
		t.Fatal("expected behaviour empty with no success criteria")
	}
}

func TestExtract_SanitizesID(t *testing.T) {
	tr := Trace{TraceID: "trace/with spaces:weird", Outcome: OutcomeError}
	v := DefaultClassifierConfig().Classify(tr)
	c := Extract(tr, v, ExtractConfig{}, time.Now())
	if strings.ContainsAny(c.ID, "/ :") {
		t.Errorf("id not sanitised: %q", c.ID)
	}
	if c.SourceTraceID != tr.TraceID {
		t.Errorf("source trace id altered: %q", c.SourceTraceID)
	}
}

func TestExtract_ExcerptsPreferFailedSpansAndRespectBudget(t *testing.T) {
	// 5 tool spans: 2 failed, 3 clean. With budget 2, both failed ones must be
	// chosen (failed-first) and clean ones excluded.
	var spans []Span
	spans = append(spans, toolSpan("a", "boom1", 0))
	spans = append(spans, toolSpan("b", "boom2", 0))
	spans = append(spans, toolSpan("c", "", 0))
	spans = append(spans, toolSpan("d", "", 0))
	spans = append(spans, toolSpan("e", "", 0))
	tr := Trace{TraceID: "z", Outcome: OutcomeError, Spans: spans}
	v := DefaultClassifierConfig().Classify(tr)
	c := Extract(tr, v, ExtractConfig{MaxExcerptsPerPillar: 2}, time.Now())

	if len(c.SpanExcerpts) != 2 {
		t.Fatalf("excerpts = %d, want 2", len(c.SpanExcerpts))
	}
	for _, e := range c.SpanExcerpts {
		if e.ErrorType == "" {
			t.Errorf("clean span chosen over failed: %s", e.Name)
		}
	}
}

func TestExtract_ExcerptsAcrossPillars(t *testing.T) {
	tr := Trace{
		TraceID: "multi",
		Outcome: OutcomeError,
		Spans: []Span{
			toolSpan("t", "boom", 0),
			{Pillar: PillarReasoning, Name: agenttrace.SpanReasoning, ErrorType: "planErr", StatusError: true},
			memSpan(agenttrace.MemoryRead, "kb", 0.9),
		},
	}
	v := DefaultClassifierConfig().Classify(tr)
	c := Extract(tr, v, ExtractConfig{}, time.Now())
	seen := map[Pillar]bool{}
	for _, e := range c.SpanExcerpts {
		seen[e.Pillar] = true
	}
	for _, p := range []Pillar{PillarToolCall, PillarReasoning, PillarMemory} {
		if !seen[p] {
			t.Errorf("missing excerpt for pillar %q", p)
		}
	}
}
