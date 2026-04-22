package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestTraceContextFromHook_WithTraceparent(t *testing.T) {
	otel.SetTextMapPropagator(propagation.TraceContext{})
	tp := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	ctx := traceContextFromHook(tp)
	sc := trace.SpanContextFromContext(ctx)
	if !sc.HasTraceID() {
		t.Error("expected trace ID to be set from traceparent")
	}
	if sc.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("unexpected trace ID: %s", sc.TraceID().String())
	}
}

func TestTraceContextFromHook_WithoutTraceparent(t *testing.T) {
	ctx := traceContextFromHook("")
	sc := trace.SpanContextFromContext(ctx)
	if sc.HasTraceID() {
		t.Error("expected no trace ID when traceparent is empty")
	}
}

func TestTraceContextFromHook_FallbackToEnv(t *testing.T) {
	otel.SetTextMapPropagator(propagation.TraceContext{})
	tp := "00-abcdef1234567890abcdef1234567890-1234567890abcdef-01"
	t.Setenv("TRACEPARENT", tp)
	ctx := traceContextFromHook("")
	sc := trace.SpanContextFromContext(ctx)
	if !sc.HasTraceID() {
		t.Error("expected trace ID from TRACEPARENT env var")
	}
	if sc.TraceID().String() != "abcdef1234567890abcdef1234567890" {
		t.Errorf("unexpected trace ID: %s", sc.TraceID().String())
	}
}

func TestRunHookWithTraceparent(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "stdin-*.json")
	if err != nil {
		t.Fatal(err)
	}
	input := HookInput{
		ToolName:    "Bash",
		Cwd:         "~/src",
		Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	}
	if err := json.NewEncoder(tmpFile).Encode(input); err != nil {
		t.Fatal(err)
	}
	if _, err := tmpFile.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	stderrFile, err := os.CreateTemp(t.TempDir(), "stderr-*")
	if err != nil {
		t.Fatal(err)
	}

	code := runHook(tmpFile, stderrFile)
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunHookEmptyInput(t *testing.T) {
	// Create a temp file with empty JSON as stdin.
	tmpFile, err := os.CreateTemp(t.TempDir(), "stdin-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmpFile.WriteString("{}"); err != nil {
		t.Fatal(err)
	}
	if _, err := tmpFile.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	stderrFile, err := os.CreateTemp(t.TempDir(), "stderr-*")
	if err != nil {
		t.Fatal(err)
	}

	code := runHook(tmpFile, stderrFile)
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunHookNoSession(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "stdin-*.json")
	if err != nil {
		t.Fatal(err)
	}
	input := HookInput{ToolName: "Bash", Cwd: "~/src"}
	if err := json.NewEncoder(tmpFile).Encode(input); err != nil {
		t.Fatal(err)
	}
	if _, err := tmpFile.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	stderrFile, err := os.CreateTemp(t.TempDir(), "stderr-*")
	if err != nil {
		t.Fatal(err)
	}

	code := runHook(tmpFile, stderrFile)
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestShouldCheckSampling(t *testing.T) {
	// Use a unique session ID for this test.
	sessionID := "test-sampling-" + t.Name()
	counterPath := "/tmp/cost-guard-" + sessionID + ".count"
	// Clean up.
	os.Remove(counterPath)
	t.Cleanup(func() { os.Remove(counterPath) })

	checkResults := make([]bool, 20)
	for i := range 20 {
		checkResults[i] = shouldCheck(sessionID)
	}

	// Should be true on 10th and 20th calls (index 9 and 19).
	for i, result := range checkResults {
		expected := (i+1)%sampleInterval == 0
		if result != expected {
			t.Errorf("call %d: expected shouldCheck=%v, got %v", i+1, expected, result)
		}
	}
}

func TestComputeSessionCost(t *testing.T) {
	// Create a temporary JSONL file with known entries.
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "test-session.jsonl")

	lines := []string{
		// Sonnet assistant message: 1000 input, 500 output, 100 cache_read, 50 cache_write
		`{"type":"assistant","message":{"model":"claude-3-5-sonnet-20241022","usage":{"input_tokens":1000,"output_tokens":500,"cache_read_input_tokens":100,"cache_creation_input_tokens":50}}}`,
		// User message — should be skipped.
		`{"type":"user","message":{"model":"claude-3-5-sonnet-20241022","usage":{"input_tokens":100,"output_tokens":0}}}`,
		// Opus assistant message: 2000 input, 1000 output.
		`{"type":"assistant","message":{"model":"claude-opus-4-6","usage":{"input_tokens":2000,"output_tokens":1000,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
		// Invalid JSON — should be skipped.
		`{invalid json`,
	}

	f, err := os.Create(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		if _, err := f.WriteString(line + "\n"); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()

	cost, err := computeSessionCost(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}

	// GetPricing returns per-token prices (divided by 1M already).
	// Sonnet per-token: input=0.000003, output=0.000015, cache_read=0.0000003, cache_write=0.00000375
	// Sonnet: 1000*0.000003 + 500*0.000015 + 100*0.0000003 + 50*0.00000375
	//       = 0.003 + 0.0075 + 0.00003 + 0.0001875 = 0.0107175
	// Opus per-token: input=0.000015, output=0.000075
	// Opus:  2000*0.000015 + 1000*0.000075
	//       = 0.03 + 0.075 = 0.105
	// Total: 0.1157175
	expected := 0.1157175
	if cost < expected-0.0001 || cost > expected+0.0001 {
		t.Errorf("expected cost ~%.7f, got %.7f", expected, cost)
	}
}

func TestComputeSessionCostWarningThreshold(t *testing.T) {
	// Create a JSONL file that crosses the $50 warning threshold.
	// Opus per-token output: $0.000075. Need $50 worth = 666667 tokens.
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "expensive-session.jsonl")

	line := `{"type":"assistant","message":{"model":"claude-opus-4-6","usage":{"input_tokens":0,"output_tokens":700000,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`

	if err := os.WriteFile(jsonlPath, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cost, err := computeSessionCost(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}

	// 700000 * 0.000075 = $52.50
	if cost < 52.0 || cost > 53.0 {
		t.Errorf("expected cost ~$52.50, got $%.2f", cost)
	}

	if cost < warnThreshold {
		t.Errorf("expected cost %.2f >= warning threshold %.2f", cost, warnThreshold)
	}
}

func TestComputeSessionCostUnknownModel(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "unknown-model.jsonl")

	line := `{"type":"assistant","message":{"model":"unknown-model-xyz","usage":{"input_tokens":1000,"output_tokens":500}}}`
	if err := os.WriteFile(jsonlPath, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cost, err := computeSessionCost(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}

	// Unknown model should be skipped (zero cost).
	if cost != 0 {
		t.Errorf("expected zero cost for unknown model, got %.7f", cost)
	}
}
