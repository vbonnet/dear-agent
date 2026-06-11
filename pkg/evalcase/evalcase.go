package evalcase

import (
	"strings"
	"time"
)

// SchemaVersion is the on-disk EvalCase schema version. Bump it on a breaking
// change to the JSON shape so loaders can migrate or reject old cases.
const SchemaVersion = 1

// SpanExcerpt is a trimmed, pillar-tagged span carried in an eval case so the
// case is self-contained: a reader (or a future replay harness) can see the
// concrete tool call / reasoning step / memory op that the failure turned on,
// without going back to the original trace store.
type SpanExcerpt struct {
	Pillar      Pillar         `json:"pillar"`
	Name        string         `json:"name"`
	Attributes  map[string]any `json:"attributes,omitempty"`
	ErrorType   string         `json:"error_type,omitempty"`
	StatusError bool           `json:"status_error,omitempty"`
	DurationMS  int64          `json:"duration_ms,omitempty"`
}

// EvalCase is a regression fixture distilled from one problematic production
// trace. It is the serializable unit stored in the evals/ dataset and the unit
// a future offline-eval / CI gate replays.
type EvalCase struct {
	// SchemaVersion is the schema this case was written with.
	SchemaVersion int `json:"schema_version"`
	// ID is the stable case identifier (derived from SourceTraceID) and the file
	// name stem under the store.
	ID string `json:"id"`
	// SourceTraceID is the production trace this case was extracted from.
	SourceTraceID string `json:"source_trace_id"`
	// Task is the task description / prompt the run was given.
	Task string `json:"task,omitempty"`
	// ExpectedBehavior is what should have happened (the trace's success
	// criteria, or a default when none was recorded).
	ExpectedBehavior string `json:"expected_behavior"`
	// ActualBehavior is what happened, synthesised from the outcome and the
	// failure reasons.
	ActualBehavior string `json:"actual_behavior"`
	// Classification is the primary failure class.
	Classification FailureClass `json:"classification"`
	// AllClassifications lists every matched failure class (specificity order).
	AllClassifications []FailureClass `json:"all_classifications,omitempty"`
	// Reasons are the human-readable failure explanations.
	Reasons []string `json:"reasons,omitempty"`
	// Outcome is the trace's terminal status.
	Outcome Outcome `json:"outcome,omitempty"`
	// EvalScores are the eval scores attached to the source trace.
	EvalScores map[string]float64 `json:"eval_scores,omitempty"`
	// SpanExcerpts are excerpts from the four pillars relevant to the failure.
	SpanExcerpts []SpanExcerpt `json:"span_excerpts,omitempty"`
	// GeneratedAt is when this case was extracted.
	GeneratedAt time.Time `json:"generated_at"`
}

// sanitizeID maps an arbitrary trace ID to a filesystem-safe case ID: it keeps
// [A-Za-z0-9._-] and replaces every other rune with '_'. OTel trace IDs (hex)
// pass through unchanged; an empty ID becomes "unknown" so a case is never
// written to a dotless/empty path.
func sanitizeID(traceID string) string {
	if traceID == "" {
		return "unknown"
	}
	var b strings.Builder
	b.Grow(len(traceID))
	for _, r := range traceID {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
