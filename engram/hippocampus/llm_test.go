package hippocampus

import (
	"context"
	"fmt"
	"testing"
)

func TestNoopLLM_ExtractSignals(t *testing.T) {
	llm := &NoopLLM{}
	signals, err := llm.ExtractSignals(context.Background(), "some transcript text")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if signals != nil {
		t.Errorf("expected nil signals, got %v", signals)
	}
}

func TestNoopLLM_DetectContradictions(t *testing.T) {
	llm := &NoopLLM{}
	existing := []string{"- always use Go"}
	incoming := []string{"- always use Python"}
	contradictions, err := llm.DetectContradictions(context.Background(), existing, incoming)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if contradictions != nil {
		t.Errorf("expected nil contradictions, got %v", contradictions)
	}
}

func TestNoopLLM_ImplementsInterface(t *testing.T) {
	var _ LLMProvider = (*NoopLLM)(nil)
}

func TestSonnetLLM_ImplementsInterface(t *testing.T) {
	var _ LLMProvider = (*SonnetLLM)(nil)
}

func TestSonnetLLM_ExtractSignals(t *testing.T) {
	mockQuery := func(_ context.Context, _, _ string, _ int) (string, error) {
		return `{"signals": [
			{"type": "correction", "content": "Don't use mocks in integration tests", "confidence": 0.9},
			{"type": "preference", "content": "User prefers table-driven tests", "confidence": 0.8},
			{"type": "fact", "content": "Low confidence noise", "confidence": 0.3}
		]}`, nil
	}

	llm := NewSonnetLLM(mockQuery)
	signals, err := llm.ExtractSignals(context.Background(), "user: no actually don't mock the database\nassistant: understood")
	if err != nil {
		t.Fatalf("ExtractSignals: %v", err)
	}

	// Should filter out low confidence (0.3 < 0.5)
	if len(signals) != 2 {
		t.Fatalf("expected 2 signals (filtered low confidence), got %d", len(signals))
	}

	if signals[0].Type != SignalCorrection {
		t.Errorf("signal[0].Type = %q, want correction", signals[0].Type)
	}
	if signals[0].Confidence != 0.9 {
		t.Errorf("signal[0].Confidence = %f, want 0.9", signals[0].Confidence)
	}
	if signals[1].Type != SignalPreference {
		t.Errorf("signal[1].Type = %q, want preference", signals[1].Type)
	}
}

func TestSonnetLLM_ExtractSignals_MarkdownWrapped(t *testing.T) {
	mockQuery := func(_ context.Context, _, _ string, _ int) (string, error) {
		return "```json\n{\"signals\": [{\"type\": \"decision\", \"content\": \"Use REST API\", \"confidence\": 0.85}]}\n```", nil
	}

	llm := NewSonnetLLM(mockQuery)
	signals, err := llm.ExtractSignals(context.Background(), "user: let's go with REST")
	if err != nil {
		t.Fatalf("ExtractSignals: %v", err)
	}

	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}
	if signals[0].Type != SignalDecision {
		t.Errorf("type = %q, want decision", signals[0].Type)
	}
}

func TestSonnetLLM_ExtractSignals_EmptyResult(t *testing.T) {
	mockQuery := func(_ context.Context, _, _ string, _ int) (string, error) {
		return `{"signals": []}`, nil
	}

	llm := NewSonnetLLM(mockQuery)
	signals, err := llm.ExtractSignals(context.Background(), "user: hello\nassistant: hi there")
	if err != nil {
		t.Fatalf("ExtractSignals: %v", err)
	}

	if len(signals) != 0 {
		t.Errorf("expected 0 signals, got %d", len(signals))
	}
}

func TestSonnetLLM_ExtractSignals_NilSideQuery(t *testing.T) {
	llm := NewSonnetLLM(nil)
	signals, err := llm.ExtractSignals(context.Background(), "transcript")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if signals != nil {
		t.Errorf("expected nil signals, got %v", signals)
	}
}

func TestSonnetLLM_ExtractSignals_LLMError(t *testing.T) {
	mockQuery := func(_ context.Context, _, _ string, _ int) (string, error) {
		return "", fmt.Errorf("API rate limit exceeded")
	}

	llm := NewSonnetLLM(mockQuery)
	_, err := llm.ExtractSignals(context.Background(), "transcript")
	if err == nil {
		t.Fatal("expected error from failed LLM call")
	}
}

func TestSonnetLLM_ExtractSignals_TruncatesLongTranscripts(t *testing.T) {
	var receivedLen int
	mockQuery := func(_ context.Context, _, userPrompt string, _ int) (string, error) {
		receivedLen = len(userPrompt)
		return `{"signals": []}`, nil
	}

	llm := NewSonnetLLM(mockQuery)
	longTranscript := string(make([]byte, 50000))
	llm.ExtractSignals(context.Background(), longTranscript)

	// Should be truncated to ~20000 + "... (truncated)" suffix
	if receivedLen > 20100 {
		t.Errorf("transcript not truncated: received %d chars", receivedLen)
	}
}

func TestSonnetLLM_DetectContradictions(t *testing.T) {
	mockQuery := func(_ context.Context, _, _ string, _ int) (string, error) {
		return `{"contradictions": [
			{
				"existing": "Always use tabs for indentation",
				"new": "Always use spaces for indentation",
				"resolution": "User changed preference from tabs to spaces",
				"winner": "new"
			}
		]}`, nil
	}

	llm := NewSonnetLLM(mockQuery)
	existing := []string{"- Always use tabs for indentation"}
	incoming := []string{"- Always use spaces for indentation"}

	contradictions, err := llm.DetectContradictions(context.Background(), existing, incoming)
	if err != nil {
		t.Fatalf("DetectContradictions: %v", err)
	}

	if len(contradictions) != 1 {
		t.Fatalf("expected 1 contradiction, got %d", len(contradictions))
	}

	c := contradictions[0]
	if c.Winner != "new" {
		t.Errorf("winner = %q, want new", c.Winner)
	}
	if c.Resolution == "" {
		t.Error("expected non-empty resolution")
	}
}

func TestSonnetLLM_DetectContradictions_NoConflicts(t *testing.T) {
	mockQuery := func(_ context.Context, _, _ string, _ int) (string, error) {
		return `{"contradictions": []}`, nil
	}

	llm := NewSonnetLLM(mockQuery)
	existing := []string{"- Use Go for backend"}
	incoming := []string{"- Use Python for scripts"}

	contradictions, err := llm.DetectContradictions(context.Background(), existing, incoming)
	if err != nil {
		t.Fatalf("DetectContradictions: %v", err)
	}

	if len(contradictions) != 0 {
		t.Errorf("expected 0 contradictions, got %d", len(contradictions))
	}
}

func TestSonnetLLM_DetectContradictions_EmptyInputs(t *testing.T) {
	llm := NewSonnetLLM(func(_ context.Context, _, _ string, _ int) (string, error) {
		t.Fatal("sideQuery should not be called with empty inputs")
		return "", nil
	})

	// Empty existing
	contradictions, err := llm.DetectContradictions(context.Background(), nil, []string{"entry"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contradictions != nil {
		t.Error("expected nil for empty existing")
	}

	// Empty incoming
	contradictions, err = llm.DetectContradictions(context.Background(), []string{"entry"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contradictions != nil {
		t.Error("expected nil for empty incoming")
	}
}

func TestSonnetLLM_DetectContradictions_InvalidWinner(t *testing.T) {
	mockQuery := func(_ context.Context, _, _ string, _ int) (string, error) {
		return `{"contradictions": [
			{"existing": "a", "new": "b", "resolution": "conflict", "winner": "invalid_value"}
		]}`, nil
	}

	llm := NewSonnetLLM(mockQuery)
	contradictions, err := llm.DetectContradictions(context.Background(), []string{"a"}, []string{"b"})
	if err != nil {
		t.Fatalf("DetectContradictions: %v", err)
	}

	if len(contradictions) != 1 {
		t.Fatalf("expected 1 contradiction, got %d", len(contradictions))
	}
	// Invalid winner should default to "new"
	if contradictions[0].Winner != "new" {
		t.Errorf("winner = %q, want 'new' (default)", contradictions[0].Winner)
	}
}

func TestParseSignalType(t *testing.T) {
	tests := []struct {
		input string
		want  SignalType
	}{
		{"correction", SignalCorrection},
		{"preference", SignalPreference},
		{"decision", SignalDecision},
		{"learning", SignalLearning},
		{"fact", SignalFact},
		{"unknown", SignalFact},
		{"", SignalFact},
	}

	for _, tt := range tests {
		got := parseSignalType(tt.input)
		if got != tt.want {
			t.Errorf("parseSignalType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestUnmarshalLLMJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"direct JSON", `{"signals": []}`, false},
		{"markdown wrapped", "```json\n{\"signals\": []}\n```", false},
		{"text prefix", "Here is the result:\n{\"signals\": []}", false},
		{"no JSON", "just plain text", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result llmSignalResponse
			err := unmarshalLLMJSON(tt.input, &result)
			if (err != nil) != tt.wantErr {
				t.Errorf("unmarshalLLMJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestSonnetLLM_V1V2Comparison demonstrates that V2 LLM extraction can capture
// signals that V1 regex misses, and that both can run side by side.
func TestSonnetLLM_V1V2Comparison(t *testing.T) {
	transcript := `user: I've been thinking about it and we should definitely use PostgreSQL instead of MySQL for the new service
assistant: Got it, I'll update the configuration to use PostgreSQL.
user: Also, I noticed that the retry logic is wrong - it should use exponential backoff, not linear
assistant: Good catch, I'll fix the retry to use exponential backoff.`

	// V1: regex-based
	v1Signals := extractSignalsV1(transcript, "test-session")

	// V2: LLM-based (mock)
	mockQuery := func(_ context.Context, _, _ string, _ int) (string, error) {
		return `{"signals": [
			{"type": "decision", "content": "Use PostgreSQL instead of MySQL for the new service", "confidence": 0.95},
			{"type": "correction", "content": "Retry logic should use exponential backoff, not linear", "confidence": 0.9},
			{"type": "preference", "content": "User prefers PostgreSQL over MySQL", "confidence": 0.7}
		]}`, nil
	}

	llm := NewSonnetLLM(mockQuery)
	v2Signals, err := llm.ExtractSignals(context.Background(), transcript)
	if err != nil {
		t.Fatalf("V2 extraction failed: %v", err)
	}

	// V2 should capture more nuanced signals
	t.Logf("V1 found %d signals, V2 found %d signals", len(v1Signals), len(v2Signals))

	if len(v2Signals) < len(v1Signals) {
		t.Errorf("V2 should find at least as many signals as V1: V1=%d, V2=%d", len(v1Signals), len(v2Signals))
	}

	// V2 should have higher average confidence
	var v2ConfSum float64
	for _, s := range v2Signals {
		v2ConfSum += s.Confidence
	}
	v2AvgConf := v2ConfSum / float64(len(v2Signals))
	if v2AvgConf < 0.7 {
		t.Errorf("V2 average confidence = %f, expected >= 0.7", v2AvgConf)
	}
}
