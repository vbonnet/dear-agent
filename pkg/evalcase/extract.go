package evalcase

import (
	"fmt"
	"strings"
	"time"
)

// defaultMaxExcerptsPerPillar bounds how many spans of each pillar an eval case
// carries, so a long run does not produce a megabyte case. Errored / triggering
// spans are preferred over clean ones within the budget.
const defaultMaxExcerptsPerPillar = 3

// ExtractConfig tunes eval-case extraction.
type ExtractConfig struct {
	// MaxExcerptsPerPillar caps span excerpts per pillar. Zero uses the default.
	MaxExcerptsPerPillar int
}

func (c ExtractConfig) maxPerPillar() int {
	if c.MaxExcerptsPerPillar <= 0 {
		return defaultMaxExcerptsPerPillar
	}
	return c.MaxExcerptsPerPillar
}

// Extract turns a problematic trace and its verdict into an EvalCase. now stamps
// GeneratedAt (injected rather than read from the clock so extraction is
// deterministic and testable). It assumes v.Problematic is true; callers gate on
// Classify first (the Pipeline does).
func Extract(t Trace, v Verdict, cfg ExtractConfig, now time.Time) EvalCase {
	return EvalCase{
		SchemaVersion:      SchemaVersion,
		ID:                 sanitizeID(t.TraceID),
		SourceTraceID:      t.TraceID,
		Task:               t.Task,
		ExpectedBehavior:   expectedBehavior(t),
		ActualBehavior:     actualBehavior(t, v),
		Classification:     v.Primary,
		AllClassifications: v.Classes,
		Reasons:            v.Reasons,
		Outcome:            t.Outcome,
		EvalScores:         t.EvalScores,
		SpanExcerpts:       selectExcerpts(t, cfg.maxPerPillar()),
		GeneratedAt:        now,
	}
}

// expectedBehavior is the trace's success criteria, or a sensible default when
// the audit did not record one — an eval case always states what *should* have
// happened.
func expectedBehavior(t Trace) string {
	if strings.TrimSpace(t.SuccessCriteria) != "" {
		return t.SuccessCriteria
	}
	return "The agent completes the task successfully with no tool, reasoning, state, or memory errors."
}

// actualBehavior synthesises a one-paragraph description of what went wrong from
// the outcome and the classifier's reasons.
func actualBehavior(t Trace, v Verdict) string {
	var b strings.Builder
	outcome := t.Outcome
	if outcome == OutcomeUnknown {
		outcome = "unrecorded"
	}
	fmt.Fprintf(&b, "Run terminated with outcome %q.", outcome)
	if len(v.Reasons) > 0 {
		b.WriteString(" Detected issues: ")
		b.WriteString(strings.Join(v.Reasons, "; "))
		b.WriteString(".")
	}
	return b.String()
}

// selectExcerpts picks up to maxPerPillar spans from each pillar, errored /
// failed spans first so the cause of the failure is always represented, then
// clean spans for context until the budget is spent.
func selectExcerpts(t Trace, maxPerPillar int) []SpanExcerpt {
	var out []SpanExcerpt
	byPillar := t.spansByPillar()
	// Deterministic pillar order.
	for _, pillar := range []Pillar{PillarToolCall, PillarReasoning, PillarStateTransition, PillarMemory, PillarOther} {
		spans := byPillar[pillar]
		if len(spans) == 0 {
			continue
		}
		var failed, clean []Span
		for _, s := range spans {
			if s.failed() {
				failed = append(failed, s)
			} else {
				clean = append(clean, s)
			}
		}
		picked := 0
		for _, group := range [][]Span{failed, clean} {
			for _, s := range group {
				if picked >= maxPerPillar {
					break
				}
				out = append(out, excerptOf(s))
				picked++
			}
		}
	}
	return out
}

func excerptOf(s Span) SpanExcerpt {
	return SpanExcerpt{
		Pillar:      s.Pillar,
		Name:        s.Name,
		Attributes:  s.Attributes,
		ErrorType:   s.ErrorType,
		StatusError: s.StatusError,
		DurationMS:  s.DurationMS,
	}
}
