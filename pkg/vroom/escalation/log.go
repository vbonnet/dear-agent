package escalation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// EscalationEvent is one append-only record: a single state transition of one
// escalation. All events for an escalation share EscalationID. The schema is
// the contract the three analyses are written against (see the package doc and
// the field comments).
type EscalationEvent struct {
	EventID      string    `json:"event_id"`
	EscalationID string    `json:"escalation_id"`
	Timestamp    time.Time `json:"timestamp"`
	Phase        Phase     `json:"phase"`
	Kind         Kind      `json:"kind"`
	Mode         Mode      `json:"mode"`

	// Who / routing.
	OriginSessionID string   `json:"origin_session_id"`
	OriginRole      string   `json:"origin_role,omitempty"`
	FromSessionID   string   `json:"from_session_id,omitempty"`
	FromRole        string   `json:"from_role,omitempty"`
	ToSessionID     string   `json:"to_session_id,omitempty"`
	ToRole          string   `json:"to_role,omitempty"`
	ChainDepth      int      `json:"chain_depth"`
	Chain           []string `json:"chain,omitempty"`

	// Content for analysis. QuestionNorm/QuestionHash make frequency and
	// "many agents, same question" robust to trivial wording differences:
	//   - frequent question types → GROUP BY question_hash | topic | kind
	//   - many agents same question (missing prompt context) →
	//     GROUP BY question_hash HAVING COUNT(DISTINCT origin_session_id) > N
	Question     string `json:"question,omitempty"`
	QuestionNorm string `json:"question_norm,omitempty"`
	QuestionHash string `json:"question_hash,omitempty"`
	Topic        string `json:"topic,omitempty"`

	Answer         string `json:"answer,omitempty"`
	AnsweredBy     string `json:"answered_by,omitempty"`
	AnsweredByRole string `json:"answered_by_role,omitempty"`

	// Classification.
	Disposition      Disposition `json:"disposition,omitempty"`
	MustReachHuman   bool        `json:"must_reach_human,omitempty"`
	ClassifierReason string      `json:"classifier_reason,omitempty"`

	// Quality signals, backfilled by a later review/judge pass — empty at write
	// time. Incorrect/misaligned answers → WHERE outcome IN ('incorrect',
	// 'misaligned').
	Outcome      string `json:"outcome,omitempty"`
	Misalignment string `json:"misalignment,omitempty"`

	// LatencyMs is set on answered/auto-resolved: ms since the escalation was
	// raised.
	LatencyMs int64 `json:"latency_ms,omitempty"`
}

// Sink is the append-only destination for escalation events. JSONLSink writes
// the canonical analysis log; MultiSink fans out (e.g. JSONL + the VROOM
// decision trail).
type Sink interface {
	Record(ctx context.Context, ev EscalationEvent) error
	Close() error
}

// JSONLSink writes one JSON object per line to an append-only file. The write is
// a single os.File.Write of one line (< PIPE_BUF), so concurrent multi-process
// appends interleave at line granularity but never corrupt a record — the same
// guarantee decisiontrail.JSONLTrail relies on.
type JSONLSink struct {
	mu     sync.Mutex
	w      io.WriteCloser
	closed bool
}

// OpenJSONLSink opens (creating parent dirs) path for append-only writes.
func OpenJSONLSink(path string) (*JSONLSink, error) {
	if path == "" {
		return nil, fmt.Errorf("escalation: empty sink path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("escalation: mkdir %q: %w", filepath.Dir(path), err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("escalation: open %q: %w", path, err)
	}
	return NewJSONLSink(f), nil
}

// NewJSONLSink wraps an existing writer (useful for tests).
func NewJSONLSink(w io.WriteCloser) *JSONLSink { return &JSONLSink{w: w} }

// Record implements Sink.
func (s *JSONLSink) Record(ctx context.Context, ev EscalationEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("escalation: marshal event: %w", err)
	}
	line = append(line, '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("escalation: sink is closed")
	}
	if _, err := s.w.Write(line); err != nil {
		return fmt.Errorf("escalation: write event: %w", err)
	}
	return nil
}

// Close implements Sink.
func (s *JSONLSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.w.Close()
}

// MultiSink fans an event out to several sinks. A failure in one sink does not
// stop the others; the first error is returned after all have been tried.
type MultiSink struct{ sinks []Sink }

// NewMultiSink composes sinks.
func NewMultiSink(sinks ...Sink) *MultiSink { return &MultiSink{sinks: sinks} }

// Record implements Sink.
func (m *MultiSink) Record(ctx context.Context, ev EscalationEvent) error {
	var firstErr error
	for _, s := range m.sinks {
		if err := s.Record(ctx, ev); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Close implements Sink.
func (m *MultiSink) Close() error {
	var firstErr error
	for _, s := range m.sinks {
		if err := s.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// instrumentationScope names this package's tracer/meter.
const instrumentationScope = "github.com/vbonnet/dear-agent/pkg/vroom/escalation"

// Logger records an event to the Sink and mirrors it to OpenTelemetry (a span
// plus metrics), so a single Record call satisfies "log everything via OTel"
// and the structured-analysis requirement at once. OTel is no-op until a
// collector is configured, so this is zero-cost by default.
type Logger struct {
	sink   Sink
	tracer trace.Tracer

	events  metric.Int64Counter
	raised  metric.Int64Counter
	human   metric.Int64Counter
	latency metric.Int64Histogram
}

// NewLogger builds a Logger over sink. Metric instruments are created from the
// global MeterProvider (no-op unless telemetry.InitMeter installed a real one).
func NewLogger(sink Sink) *Logger {
	m := otel.Meter(instrumentationScope)
	events, _ := m.Int64Counter("vroom.escalation.events",
		metric.WithDescription("Escalation state-transition events, by phase/kind/disposition"),
		metric.WithUnit("{event}"))
	raised, _ := m.Int64Counter("vroom.escalation.raised",
		metric.WithDescription("Escalations raised, by kind/topic"),
		metric.WithUnit("{escalation}"))
	human, _ := m.Int64Counter("vroom.escalation.dispatched_to_human",
		metric.WithDescription("Escalations that reached the human via Dispatch"),
		metric.WithUnit("{escalation}"))
	latency, _ := m.Int64Histogram("vroom.escalation.latency_ms",
		metric.WithDescription("Time from raised to resolved"),
		metric.WithUnit("ms"))
	return &Logger{
		sink:    sink,
		tracer:  otel.Tracer(instrumentationScope),
		events:  events,
		raised:  raised,
		human:   human,
		latency: latency,
	}
}

// Record fills in derived fields (EventID, Timestamp, normalised question +
// hash), writes to the Sink, and emits the OTel span + metrics.
func (l *Logger) Record(ctx context.Context, ev EscalationEvent) error {
	if ev.EventID == "" {
		ev.EventID = uuid.New().String()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	} else {
		ev.Timestamp = ev.Timestamp.UTC()
	}
	if ev.Question != "" && ev.QuestionNorm == "" {
		ev.QuestionNorm = NormalizeQuestion(ev.Question)
		ev.QuestionHash = HashQuestion(ev.QuestionNorm)
	}

	// Span: one short span per transition, attributes mirror the analysis keys.
	_, span := l.tracer.Start(ctx, "vroom.escalation."+string(ev.Phase),
		trace.WithAttributes(
			attribute.String("escalation.id", ev.EscalationID),
			attribute.String("escalation.phase", string(ev.Phase)),
			attribute.String("escalation.kind", string(ev.Kind)),
			attribute.String("escalation.mode", string(ev.Mode)),
			attribute.String("escalation.disposition", string(ev.Disposition)),
			attribute.Bool("escalation.must_reach_human", ev.MustReachHuman),
			attribute.String("escalation.origin_session_id", ev.OriginSessionID),
			attribute.String("escalation.from_session_id", ev.FromSessionID),
			attribute.String("escalation.to_session_id", ev.ToSessionID),
			attribute.String("escalation.topic", ev.Topic),
			attribute.String("escalation.question_hash", ev.QuestionHash),
			attribute.Int("escalation.chain_depth", ev.ChainDepth),
		))
	span.End()

	if l.events != nil {
		l.events.Add(ctx, 1, metric.WithAttributes(
			attribute.String("phase", string(ev.Phase)),
			attribute.String("kind", string(ev.Kind)),
			attribute.String("disposition", string(ev.Disposition)),
		))
	}
	//nolint:exhaustive // only these phases drive a dedicated metric; the rest intentionally no-op (the generic events counter above covers them).
	switch ev.Phase {
	case PhaseRaised:
		if l.raised != nil {
			l.raised.Add(ctx, 1, metric.WithAttributes(
				attribute.String("kind", string(ev.Kind)),
				attribute.String("topic", ev.Topic),
			))
		}
	case PhaseDispatchedToHuman:
		if l.human != nil {
			l.human.Add(ctx, 1, metric.WithAttributes(attribute.String("kind", string(ev.Kind))))
		}
	case PhaseAnswered, PhaseAutoResolved:
		if l.latency != nil && ev.LatencyMs > 0 {
			l.latency.Record(ctx, ev.LatencyMs, metric.WithAttributes(
				attribute.String("kind", string(ev.Kind)),
				attribute.String("phase", string(ev.Phase)),
			))
		}
	}

	return l.sink.Record(ctx, ev)
}

// Close closes the underlying sink.
func (l *Logger) Close() error { return l.sink.Close() }

var wsRe = regexp.MustCompile(`\s+`)
var nonWordRe = regexp.MustCompile(`[^\p{L}\p{N}\s]+`)

// NormalizeQuestion lowercases, strips punctuation, and collapses whitespace so
// that two phrasings of the same question hash equal. It is deliberately simple
// and stable (no stemming/stopwords) so the hash is reproducible across builds.
func NormalizeQuestion(q string) string {
	q = strings.ToLower(q)
	q = nonWordRe.ReplaceAllString(q, " ")
	q = wsRe.ReplaceAllString(q, " ")
	return strings.TrimSpace(q)
}

// HashQuestion returns a stable hex sha256 of the normalised question, the
// grouping key for the frequency and "same question across agents" analyses.
func HashQuestion(norm string) string {
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])
}
