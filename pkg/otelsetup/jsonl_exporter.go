// Package otelsetup provides otelsetup-related functionality.
// JSONL SpanExporter writes completed spans to a JSONL file for offline
// analysis by the session summary and retrospective hooks.
//
// Spans are written to ~/.engram/traces/<session-id>/spans.jsonl where
// session-id comes from the ENGRAM_SESSION_ID environment variable.
package otelsetup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// JSONLSpan is the on-disk representation of a single span.
type JSONLSpan struct {
	TraceID      string            `json:"trace_id"`
	SpanID       string            `json:"span_id"`
	ParentSpanID string            `json:"parent_span_id,omitempty"`
	Name         string            `json:"name"`
	Kind         string            `json:"kind"`
	StartTime    time.Time         `json:"start_time"`
	EndTime      time.Time         `json:"end_time"`
	DurationMs   float64           `json:"duration_ms"`
	StatusCode   string            `json:"status_code"`
	StatusMsg    string            `json:"status_message,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
	Events       []JSONLEvent      `json:"events,omitempty"`
}

// JSONLEvent is a simplified span event.
type JSONLEvent struct {
	Name       string            `json:"name"`
	Time       time.Time         `json:"time"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// JSONLExporter implements sdktrace.SpanExporter, writing spans to a
// JSONL file on disk.
type JSONLExporter struct {
	mu   sync.Mutex
	file *os.File
	enc  *json.Encoder
}

// Compile-time interface check.
var _ sdktrace.SpanExporter = (*JSONLExporter)(nil)

// NewJSONLExporter creates a JSONLExporter that writes to the traces directory
// for the given session ID. If sessionID is empty, "default" is used.
func NewJSONLExporter(sessionID string) (*JSONLExporter, error) {
	if sessionID == "" {
		sessionID = "default"
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("jsonl exporter: home dir: %w", err)
	}

	dir := filepath.Join(home, ".engram", "traces", sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("jsonl exporter: mkdir: %w", err)
	}

	path := filepath.Join(dir, "spans.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("jsonl exporter: open: %w", err)
	}

	return &JSONLExporter{
		file: f,
		enc:  json.NewEncoder(f),
	}, nil
}

// ExportSpans writes completed spans to the JSONL file.
func (e *JSONLExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.file == nil {
		return nil
	}

	for _, s := range spans {
		js := JSONLSpan{
			TraceID:    s.SpanContext().TraceID().String(),
			SpanID:     s.SpanContext().SpanID().String(),
			Name:       s.Name(),
			Kind:       s.SpanKind().String(),
			StartTime:  s.StartTime(),
			EndTime:    s.EndTime(),
			DurationMs: float64(s.EndTime().Sub(s.StartTime()).Microseconds()) / 1000.0,
			StatusCode: s.Status().Code.String(),
			StatusMsg:  s.Status().Description,
		}

		if s.Parent().HasSpanID() {
			js.ParentSpanID = s.Parent().SpanID().String()
		}

		// Flatten attributes to string map.
		if attrs := s.Attributes(); len(attrs) > 0 {
			js.Attributes = make(map[string]string, len(attrs))
			for _, kv := range attrs {
				js.Attributes[string(kv.Key)] = kv.Value.Emit()
			}
		}

		// Convert events.
		for _, ev := range s.Events() {
			je := JSONLEvent{
				Name: ev.Name,
				Time: ev.Time,
			}
			if len(ev.Attributes) > 0 {
				je.Attributes = make(map[string]string, len(ev.Attributes))
				for _, kv := range ev.Attributes {
					je.Attributes[string(kv.Key)] = kv.Value.Emit()
				}
			}
			js.Events = append(js.Events, je)
		}

		if err := e.enc.Encode(js); err != nil {
			return fmt.Errorf("jsonl exporter: encode: %w", err)
		}
	}

	return e.file.Sync()
}

// Shutdown flushes and closes the JSONL file.
func (e *JSONLExporter) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.file == nil {
		return nil
	}

	err := e.file.Close()
	e.file = nil
	return err
}
