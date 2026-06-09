package evalcase

import (
	"github.com/vbonnet/dear-agent/pkg/agenttrace"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// evalScoreNameKey / evalScoreValueKey mirror the attribute keys
// telemetry.RecordEvalScore attaches to a span. The bridge reads them back so a
// score recorded online during a run surfaces as a Trace.EvalScores entry the
// classifier can act on. Kept as local constants to avoid importing the
// internal/telemetry package just for two strings.
const (
	evalScoreNameKey  = "gen_ai.eval.name"
	evalScoreValueKey = "gen_ai.eval.score"
)

// FromReadOnlySpans converts captured OTel spans — e.g. those an in-memory
// SpanExporter (tracetest.SpanRecorder) collected from a run instrumented with
// pkg/agenttrace — into Traces grouped by trace ID, one Trace per distinct
// trace. This is the live-span path into the pipeline; the serialized-JSON path
// (an Audit phase that persisted Traces) decodes Trace directly.
//
// Each span becomes a pillar-tagged Span (pillar resolved from the span name via
// agenttrace.PillarForSpan, attributes copied verbatim, error state taken from
// the OTel status and error.type). Any gen_ai.eval.name/score pair found on a
// span is folded into the owning trace's EvalScores. Task, SuccessCriteria, and
// Outcome are not carried on spans, so callers enrich those afterwards (see
// EnrichTrace); the returned Traces are otherwise ready to classify.
//
// Traces are returned in first-seen order of their trace ID for determinism.
func FromReadOnlySpans(spans []sdktrace.ReadOnlySpan) []Trace {
	byID := map[string]*Trace{}
	var order []string

	for _, ro := range spans {
		if ro == nil {
			continue
		}
		tid := ro.SpanContext().TraceID().String()
		tr, ok := byID[tid]
		if !ok {
			tr = &Trace{TraceID: tid}
			byID[tid] = tr
			order = append(order, tid)
		}

		attrs, errType, evalName, evalScore, haveEval := readAttrs(ro.Attributes())
		statusErr := ro.Status().Code == codes.Error

		// Bound the run window across spans.
		if tr.StartedAt.IsZero() || ro.StartTime().Before(tr.StartedAt) {
			tr.StartedAt = ro.StartTime()
		}
		if ro.EndTime().After(tr.EndedAt) {
			tr.EndedAt = ro.EndTime()
		}

		if haveEval {
			if tr.EvalScores == nil {
				tr.EvalScores = map[string]float64{}
			}
			tr.EvalScores[evalName] = evalScore
		}

		tr.Spans = append(tr.Spans, Span{
			Pillar:      Pillar(agenttrace.PillarForSpan(ro.Name())),
			Name:        ro.Name(),
			Attributes:  attrs,
			ErrorType:   errType,
			StatusError: statusErr,
			DurationMS:  ro.EndTime().Sub(ro.StartTime()).Milliseconds(),
		})
	}

	out := make([]Trace, 0, len(order))
	for _, tid := range order {
		out = append(out, *byID[tid])
	}
	return out
}

// EnrichTrace sets the run-level fields that spans do not carry (the task, its
// success criteria, and the terminal outcome) on every trace whose ID is keyed
// in meta. A trace absent from meta is left unchanged. Returns the slice for
// chaining. This lets a caller pair FromReadOnlySpans with the task context the
// DEAR Audit phase holds.
func EnrichTrace(traces []Trace, meta map[string]TraceMeta) []Trace {
	for i := range traces {
		if m, ok := meta[traces[i].TraceID]; ok {
			traces[i].Task = m.Task
			traces[i].SuccessCriteria = m.SuccessCriteria
			traces[i].Outcome = m.Outcome
		}
	}
	return traces
}

// TraceMeta is the run-level context the Audit phase attaches to a trace ID.
type TraceMeta struct {
	Task            string
	SuccessCriteria string
	Outcome         Outcome
}

// readAttrs copies an OTel attribute set into a string-keyed map of Go values
// and pulls out the error.type and eval name/score, which get dedicated
// handling. Numeric attributes are preserved as int64/float64 so the classifier
// can read retry counts and relevance scores.
func readAttrs(kvs []attribute.KeyValue) (attrs map[string]any, errType, evalName string, evalScore float64, haveEval bool) {
	attrs = make(map[string]any, len(kvs))
	var sawName, sawScore bool
	for _, kv := range kvs {
		key := string(kv.Key)
		val := attrValue(kv.Value)
		switch key {
		case agenttrace.AttrErrorType:
			if s, ok := val.(string); ok {
				errType = s
			}
		case evalScoreNameKey:
			if s, ok := val.(string); ok {
				evalName, sawName = s, true
			}
		case evalScoreValueKey:
			if f, ok := toFloat(val); ok {
				evalScore, sawScore = f, true
			}
		}
		attrs[key] = val
	}
	haveEval = sawName && sawScore
	return attrs, errType, evalName, evalScore, haveEval
}

// attrValue unwraps an OTel attribute value into a plain Go value.
func attrValue(v attribute.Value) any {
	switch v.Type() {
	case attribute.STRING:
		return v.AsString()
	case attribute.INT64:
		return v.AsInt64()
	case attribute.FLOAT64:
		return v.AsFloat64()
	case attribute.BOOL:
		return v.AsBool()
	default:
		return v.Emit()
	}
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}
