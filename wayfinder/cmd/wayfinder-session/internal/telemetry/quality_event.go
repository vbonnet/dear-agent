// Package telemetry provides quality-assessed event emission and analytics
// for correlating token usage with documentation quality scores.
package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// EventType identifies the kind of telemetry event.
const EventType = "wayfinder.quality.assessed"

// QualityAssessedEvent records a documentation quality assessment
// along with token usage data for correlation analysis.
type QualityAssessedEvent struct {
	Type           string    `json:"type"`
	Phase          string    `json:"phase"`
	Score          float64   `json:"score"`
	InputTokens    int       `json:"input_tokens"`
	OutputTokens   int       `json:"output_tokens"`
	ContextSources []string  `json:"context_sources"`
	JudgeModel     string    `json:"judge_model"`
	Timestamp      time.Time `json:"timestamp"`
	ProjectName    string    `json:"project_name"`
	TraceID        string    `json:"trace_id,omitempty"`
	SpanID         string    `json:"span_id,omitempty"`
}

// DefaultTelemetryPath returns the default telemetry file path (~/.claude/telemetry.jsonl).
func DefaultTelemetryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".claude", "telemetry.jsonl"), nil
}

// EmitQualityEvent appends a QualityAssessedEvent to the telemetry JSONL file
// and emits an OTel span with the event attributes (dual-write).
func EmitQualityEvent(ctx context.Context, event QualityAssessedEvent, telemetryPath string) error {
	// Ensure the type field is set
	event.Type = EventType

	// Set timestamp if not already set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Extract span context and populate TraceID/SpanID
	sc := trace.SpanFromContext(ctx).SpanContext()
	if sc.HasTraceID() {
		event.TraceID = sc.TraceID().String()
	}
	if sc.HasSpanID() {
		event.SpanID = sc.SpanID().String()
	}

	// Dual-write: emit OTel span
	tracer := otel.Tracer("engram/wayfinder/telemetry")
	_, span := tracer.Start(ctx, "quality.assessed",
		trace.WithAttributes(
			attribute.String("quality.phase", event.Phase),
			attribute.Float64("quality.score", event.Score),
			attribute.Int("quality.input_tokens", event.InputTokens),
			attribute.Int("quality.output_tokens", event.OutputTokens),
			attribute.String("quality.judge_model", event.JudgeModel),
			attribute.String("quality.project_name", event.ProjectName),
		))
	defer span.End()

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal quality event: %w", err)
	}

	// Ensure parent directory exists
	dir := filepath.Dir(telemetryPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("failed to create telemetry directory: %w", err)
	}

	// Append to JSONL file
	f, err := os.OpenFile(telemetryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return fmt.Errorf("failed to open telemetry file: %w", err)
	}
	defer f.Close()

	// Write JSON line with newline
	data = append(data, '\n')
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write quality event: %w", err)
	}

	return nil
}
