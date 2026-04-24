package telemetry

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadQualityEventsFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	telemetryPath := filepath.Join(tmpDir, "telemetry.jsonl")

	// Write events using the emitter
	events := []QualityAssessedEvent{
		{Phase: "D3", Score: 7.5, InputTokens: 800, OutputTokens: 200, ProjectName: "proj-a"},
		{Phase: "D4", Score: 9.0, InputTokens: 1500, OutputTokens: 400, ProjectName: "proj-a"},
	}
	for _, e := range events {
		if err := EmitQualityEvent(context.Background(), e, telemetryPath); err != nil {
			t.Fatalf("EmitQualityEvent error: %v", err)
		}
	}

	// Read them back
	got, err := ReadQualityEvents(telemetryPath)
	if err != nil {
		t.Fatalf("ReadQualityEvents error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got))
	}

	if got[0].Phase != "D3" {
		t.Errorf("expected phase D3, got %s", got[0].Phase)
	}
	if got[1].Score != 9.0 {
		t.Errorf("expected score 9.0, got %f", got[1].Score)
	}
}

func TestReadQualityEventsMissingFile(t *testing.T) {
	events, err := ReadQualityEvents("/tmp/nonexistent-telemetry-file-12345.jsonl")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected empty slice, got %d events", len(events))
	}
}

func TestReadQualityEventsEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	telemetryPath := filepath.Join(tmpDir, "telemetry.jsonl")

	if err := os.WriteFile(telemetryPath, []byte(""), 0o640); err != nil {
		t.Fatalf("failed to write empty file: %v", err)
	}

	events, err := ReadQualityEvents(telemetryPath)
	if err != nil {
		t.Fatalf("ReadQualityEvents error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected empty slice, got %d events", len(events))
	}
}

func TestReadQualityEventsSkipsNonQualityLines(t *testing.T) {
	tmpDir := t.TempDir()
	telemetryPath := filepath.Join(tmpDir, "telemetry.jsonl")

	// Mix quality events with other event types
	lines := []string{
		`{"type":"wayfinder.quality.assessed","phase":"D4","score":8.5,"input_tokens":100,"output_tokens":50,"timestamp":"2026-03-24T12:00:00Z"}`,
		`{"type":"other.event","data":"something"}`,
		`{"type":"wayfinder.quality.assessed","phase":"S6","score":9.0,"input_tokens":200,"output_tokens":75,"timestamp":"2026-03-24T13:00:00Z"}`,
		`not valid json at all`,
		``,
	}

	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(telemetryPath, []byte(content), 0o640); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	events, err := ReadQualityEvents(telemetryPath)
	if err != nil {
		t.Fatalf("ReadQualityEvents error: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 quality events, got %d", len(events))
	}

	if events[0].Phase != "D4" {
		t.Errorf("expected phase D4, got %s", events[0].Phase)
	}
	if events[1].Phase != "S6" {
		t.Errorf("expected phase S6, got %s", events[1].Phase)
	}
}

func TestGenerateCSV(t *testing.T) {
	ts := time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)

	events := []QualityAssessedEvent{
		{
			Phase:          "D4",
			Score:          8.5,
			InputTokens:    1200,
			OutputTokens:   350,
			ContextSources: []string{"SPEC.md", "ARCHITECTURE.md"},
			ProjectName:    "test-project",
			Timestamp:      ts,
		},
		{
			Phase:          "S6",
			Score:          9.0,
			InputTokens:    2000,
			OutputTokens:   500,
			ContextSources: []string{"ARCHITECTURE.md"},
			ProjectName:    "test-project",
			Timestamp:      ts,
		},
	}

	var buf bytes.Buffer
	if err := GenerateCSV(events, &buf); err != nil {
		t.Fatalf("GenerateCSV error: %v", err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header + 2 rows), got %d", len(lines))
	}

	// Check header
	expectedHeader := "phase,score,input_tokens,output_tokens,context_sources,project_name,timestamp"
	if lines[0] != expectedHeader {
		t.Errorf("unexpected header:\n  got:  %s\n  want: %s", lines[0], expectedHeader)
	}

	// Check first data row
	if !strings.HasPrefix(lines[1], "D4,8.5,1200,350,") {
		t.Errorf("unexpected first row: %s", lines[1])
	}
	if !strings.Contains(lines[1], "SPEC.md;ARCHITECTURE.md") {
		t.Errorf("expected context sources in row: %s", lines[1])
	}
}

func TestGenerateCSVEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := GenerateCSV(nil, &buf); err != nil {
		t.Fatalf("GenerateCSV error: %v", err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected only header line, got %d lines", len(lines))
	}
}

func TestGenerateCSVQuotesSpecialCharacters(t *testing.T) {
	ts := time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)

	events := []QualityAssessedEvent{
		{
			Phase:          "D3",
			Score:          7.0,
			ContextSources: []string{`file "with" quotes`, "normal.md"},
			Timestamp:      ts,
		},
	}

	var buf bytes.Buffer
	if err := GenerateCSV(events, &buf); err != nil {
		t.Fatalf("GenerateCSV error: %v", err)
	}

	output := buf.String()
	// The quotes in the source name should be doubled
	if !strings.Contains(output, `""with""`) {
		t.Errorf("expected escaped quotes in output: %s", output)
	}
}
