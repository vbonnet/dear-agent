package evalcase

import "time"

// Pillar mirrors agenttrace.Pillar as a plain string type. It is re-declared
// here (rather than imported) so the on-disk Trace/EvalCase JSON is decoupled
// from the instrumentation package's Go types — a stored eval case stays valid
// even if pkg/agenttrace changes — while carrying the identical string values
// so the bridge in bridge.go maps one to the other without translation.
type Pillar string

// Pillar values, one per trace pillar plus PillarOther for non-pillar spans.
// They mirror the agenttrace.Pillar string values.
const (
	PillarToolCall        Pillar = "tool_call"
	PillarReasoning       Pillar = "reasoning"
	PillarStateTransition Pillar = "state_transition"
	PillarMemory          Pillar = "memory"
	PillarOther           Pillar = "other"
)

// Outcome is the terminal status of a completed trace, as the DEAR Audit phase
// records it.
type Outcome string

const (
	// OutcomeUnknown is the zero value: the audit did not classify the run.
	OutcomeUnknown Outcome = ""
	// OutcomeSuccess means the agent achieved the task's goal.
	OutcomeSuccess Outcome = "success"
	// OutcomeError means the run terminated in an error state.
	OutcomeError Outcome = "error"
	// OutcomeStalled means the agent got stuck making no progress.
	OutcomeStalled Outcome = "stalled"
)

// Span is a serializable, pillar-tagged view of one span in a completed trace.
// Attributes hold the four-pillar gen_ai.* attributes verbatim (the span helpers
// already truncate oversized free-text values); numeric attributes round-trip
// through JSON as float64, which the typed accessors below normalise.
type Span struct {
	Pillar      Pillar         `json:"pillar"`
	Name        string         `json:"name"`
	Attributes  map[string]any `json:"attributes,omitempty"`
	// ErrorType is the error.type attribute set when the span failed; empty for
	// a clean span.
	ErrorType string `json:"error_type,omitempty"`
	// StatusError is true when the span carried an OTel error status.
	StatusError bool `json:"status_error,omitempty"`
	// DurationMS is the span's wall-clock duration in milliseconds.
	DurationMS int64 `json:"duration_ms,omitempty"`
}

// failed reports whether the span is in any error state.
func (s Span) failed() bool { return s.StatusError || s.ErrorType != "" }

// str returns the named attribute as a string, or "" if absent/non-string.
func (s Span) str(key string) string {
	if v, ok := s.Attributes[key]; ok {
		if str, ok := v.(string); ok {
			return str
		}
	}
	return ""
}

// num returns the named attribute as a float64. It accepts the numeric types
// JSON and the OTel attribute bridge can produce (float64, int, int64), so a
// retry count written as an int and one decoded from JSON as a float64 both
// read back correctly. The bool reports whether a numeric value was present.
func (s Span) num(key string) (float64, bool) {
	v, ok := s.Attributes[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

// Trace is the serializable view of one completed agent run that the DEAR Audit
// phase reads. It is the input unit of the pipeline.
type Trace struct {
	// TraceID is the source trace identifier; it becomes the eval case ID.
	TraceID string `json:"trace_id"`
	// Task is the task description / prompt the run was given.
	Task string `json:"task,omitempty"`
	// SuccessCriteria is the expected behaviour (the DEAR Define exit
	// conditions, or the trace's success criteria). It seeds the eval case's
	// ExpectedBehavior.
	SuccessCriteria string `json:"success_criteria,omitempty"`
	// Outcome is the terminal status the audit assigned.
	Outcome Outcome `json:"outcome,omitempty"`
	// EvalScores are the per-eval scores attached to the trace (the
	// eval-as-span-attribute output), keyed by eval name, conventionally [0,1].
	EvalScores map[string]float64 `json:"eval_scores,omitempty"`
	// Spans are the four-pillar spans of the run.
	Spans []Span `json:"spans,omitempty"`
	// StartedAt / EndedAt bound the run, when known.
	StartedAt time.Time `json:"started_at,omitzero"`
	EndedAt   time.Time `json:"ended_at,omitzero"`
}

// spansByPillar groups the trace's spans by pillar, preserving order.
func (t Trace) spansByPillar() map[Pillar][]Span {
	out := make(map[Pillar][]Span)
	for _, s := range t.Spans {
		out[s.Pillar] = append(out[s.Pillar], s)
	}
	return out
}
