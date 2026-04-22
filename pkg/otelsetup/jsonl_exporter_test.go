package otelsetup

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestJSONLExporter_ExportAndShutdown(t *testing.T) {
	// Use temp dir to avoid polluting ~/.engram.
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "spans.jsonl")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	exp := &JSONLExporter{
		file: f,
		enc:  json.NewEncoder(f),
	}

	// Create a stub span using tracetest.
	now := time.Now()
	stub := tracetest.SpanStub{
		Name:      "test/operation",
		SpanKind:  trace.SpanKindInternal,
		StartTime: now,
		EndTime:   now.Add(42 * time.Millisecond),
		Status: sdktrace.Status{
			Code:        codes.Ok,
			Description: "",
		},
		Attributes: []attribute.KeyValue{
			attribute.String("service", "test"),
			attribute.Int("count", 5),
		},
	}

	roSpan := stub.Snapshot()

	err = exp.ExportSpans(context.Background(), []sdktrace.ReadOnlySpan{roSpan})
	if err != nil {
		t.Fatalf("ExportSpans: %v", err)
	}

	err = exp.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// Read and verify.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var span JSONLSpan
	if err := json.Unmarshal(data, &span); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if span.Name != "test/operation" {
		t.Errorf("name = %q, want %q", span.Name, "test/operation")
	}
	if span.DurationMs < 40 || span.DurationMs > 50 {
		t.Errorf("duration = %f, want ~42", span.DurationMs)
	}
	if span.Attributes["service"] != "test" {
		t.Errorf("attr service = %q, want %q", span.Attributes["service"], "test")
	}
	if span.StatusCode != "Ok" {
		t.Errorf("status = %q, want %q", span.StatusCode, "Ok")
	}
}

func TestJSONLExporter_MultipleSpans(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "spans.jsonl")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	exp := &JSONLExporter{file: f, enc: json.NewEncoder(f)}

	now := time.Now()
	stubs := []tracetest.SpanStub{
		{Name: "span-1", StartTime: now, EndTime: now.Add(time.Millisecond)},
		{Name: "span-2", StartTime: now, EndTime: now.Add(2 * time.Millisecond)},
		{Name: "span-3", StartTime: now, EndTime: now.Add(3 * time.Millisecond)},
	}

	roSpans := make([]sdktrace.ReadOnlySpan, len(stubs))
	for i, s := range stubs {
		roSpans[i] = s.Snapshot()
	}

	if err := exp.ExportSpans(context.Background(), roSpans); err != nil {
		t.Fatal(err)
	}
	_ = exp.Shutdown(context.Background())

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
}

func TestJSONLExporter_ShutdownIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "spans.jsonl")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	exp := &JSONLExporter{file: f, enc: json.NewEncoder(f)}

	if err := exp.Shutdown(context.Background()); err != nil {
		t.Fatalf("first shutdown: %v", err)
	}
	if err := exp.Shutdown(context.Background()); err != nil {
		t.Fatalf("second shutdown: %v", err)
	}
}

func TestNewJSONLExporter_DefaultSession(t *testing.T) {
	// Override HOME to use temp dir.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	exp, err := NewJSONLExporter("")
	if err != nil {
		t.Fatal(err)
	}
	defer exp.Shutdown(context.Background())

	// Check that the file was created in the right place.
	expected := filepath.Join(tmpHome, ".engram", "traces", "default", "spans.jsonl")
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("expected file at %s: %v", expected, err)
	}
}
