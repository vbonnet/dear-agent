package telemetry

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
)

func TestEmitQualityEvent(t *testing.T) {
	tmpDir := t.TempDir()
	telemetryPath := filepath.Join(tmpDir, "telemetry.jsonl")

	event := QualityAssessedEvent{
		Phase:          "D4",
		Score:          8.5,
		InputTokens:    1200,
		OutputTokens:   350,
		ContextSources: []string{"SPEC.md", "ARCHITECTURE.md"},
		JudgeModel:     "claude-3-haiku",
		ProjectName:    "test-project",
	}

	err := EmitQualityEvent(context.Background(), event, telemetryPath)
	if err != nil {
		t.Fatalf("EmitQualityEvent returned error: %v", err)
	}

	// Read back and verify
	data, err := os.ReadFile(telemetryPath)
	if err != nil {
		t.Fatalf("failed to read telemetry file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	var decoded QualityAssessedEvent
	if err := json.Unmarshal([]byte(lines[0]), &decoded); err != nil {
		t.Fatalf("failed to unmarshal event: %v", err)
	}

	if decoded.Type != EventType {
		t.Errorf("expected type %q, got %q", EventType, decoded.Type)
	}
	if decoded.Phase != "D4" {
		t.Errorf("expected phase D4, got %s", decoded.Phase)
	}
	if decoded.Score != 8.5 {
		t.Errorf("expected score 8.5, got %f", decoded.Score)
	}
	if decoded.InputTokens != 1200 {
		t.Errorf("expected input_tokens 1200, got %d", decoded.InputTokens)
	}
	if decoded.OutputTokens != 350 {
		t.Errorf("expected output_tokens 350, got %d", decoded.OutputTokens)
	}
	if decoded.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestEmitQualityEventMultiple(t *testing.T) {
	tmpDir := t.TempDir()
	telemetryPath := filepath.Join(tmpDir, "telemetry.jsonl")

	events := []QualityAssessedEvent{
		{Phase: "D3", Score: 7.0, ProjectName: "proj-a"},
		{Phase: "D4", Score: 9.0, ProjectName: "proj-a"},
		{Phase: "S6", Score: 8.5, ProjectName: "proj-b"},
	}

	for _, e := range events {
		if err := EmitQualityEvent(context.Background(), e, telemetryPath); err != nil {
			t.Fatalf("EmitQualityEvent returned error: %v", err)
		}
	}

	data, err := os.ReadFile(telemetryPath)
	if err != nil {
		t.Fatalf("failed to read telemetry file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
}

func TestEmitQualityEventPreservesTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	telemetryPath := filepath.Join(tmpDir, "telemetry.jsonl")

	fixedTime := time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)
	event := QualityAssessedEvent{
		Phase:     "D4",
		Score:     8.0,
		Timestamp: fixedTime,
	}

	if err := EmitQualityEvent(context.Background(), event, telemetryPath); err != nil {
		t.Fatalf("EmitQualityEvent returned error: %v", err)
	}

	data, err := os.ReadFile(telemetryPath)
	if err != nil {
		t.Fatalf("failed to read telemetry file: %v", err)
	}

	var decoded QualityAssessedEvent
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if !decoded.Timestamp.Equal(fixedTime) {
		t.Errorf("expected timestamp %v, got %v", fixedTime, decoded.Timestamp)
	}
}

func TestEmitQualityEventCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	telemetryPath := filepath.Join(tmpDir, "sub", "dir", "telemetry.jsonl")

	event := QualityAssessedEvent{Phase: "D4", Score: 8.0}
	if err := EmitQualityEvent(context.Background(), event, telemetryPath); err != nil {
		t.Fatalf("EmitQualityEvent returned error: %v", err)
	}

	if _, err := os.Stat(telemetryPath); err != nil {
		t.Fatalf("telemetry file was not created: %v", err)
	}
}

func TestEmitQualityEventPopulatesTraceIDFromSpanContext(t *testing.T) {
	tmpDir := t.TempDir()
	telemetryPath := filepath.Join(tmpDir, "telemetry.jsonl")

	// Create a span context with known TraceID and SpanID
	traceID, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	event := QualityAssessedEvent{Phase: "D4", Score: 8.5}
	if err := EmitQualityEvent(ctx, event, telemetryPath); err != nil {
		t.Fatalf("EmitQualityEvent returned error: %v", err)
	}

	data, err := os.ReadFile(telemetryPath)
	if err != nil {
		t.Fatalf("failed to read telemetry file: %v", err)
	}

	var decoded QualityAssessedEvent
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.TraceID != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("expected trace_id 0af7651916cd43dd8448eb211c80319c, got %s", decoded.TraceID)
	}
	if decoded.SpanID != "00f067aa0ba902b7" {
		t.Errorf("expected span_id 00f067aa0ba902b7, got %s", decoded.SpanID)
	}
}

func TestEmitQualityEventNoSpanContextOmitsIDs(t *testing.T) {
	tmpDir := t.TempDir()
	telemetryPath := filepath.Join(tmpDir, "telemetry.jsonl")

	event := QualityAssessedEvent{Phase: "D4", Score: 8.0}
	if err := EmitQualityEvent(context.Background(), event, telemetryPath); err != nil {
		t.Fatalf("EmitQualityEvent returned error: %v", err)
	}

	data, err := os.ReadFile(telemetryPath)
	if err != nil {
		t.Fatalf("failed to read telemetry file: %v", err)
	}

	var decoded QualityAssessedEvent
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.TraceID != "" {
		t.Errorf("expected empty trace_id without span context, got %s", decoded.TraceID)
	}
	if decoded.SpanID != "" {
		t.Errorf("expected empty span_id without span context, got %s", decoded.SpanID)
	}
}

func TestDefaultTelemetryPath(t *testing.T) {
	path, err := DefaultTelemetryPath()
	if err != nil {
		t.Fatalf("DefaultTelemetryPath returned error: %v", err)
	}

	if !strings.HasSuffix(path, filepath.Join(".claude", "telemetry.jsonl")) {
		t.Errorf("unexpected path: %s", path)
	}
}
