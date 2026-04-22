package tokentracking

import (
	"bytes"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/telemetry"
)

// Benchmark token extraction (should be <100μs per S7 requirements)
func BenchmarkExtractTokens(b *testing.B) {
	response := &APIResponse{
		Usage: struct {
			InputTokens         int `json:"input_tokens"`
			OutputTokens        int `json:"output_tokens"`
			CacheCreationTokens int `json:"cache_creation_input_tokens"`
			CacheReadTokens     int `json:"cache_read_input_tokens"`
		}{
			InputTokens:  1000,
			OutputTokens: 500,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ExtractTokens(response)
	}
}

// Benchmark JSON extraction (should be <500μs per S7 requirements)
func BenchmarkExtractTokensFromJSON(b *testing.B) {
	responseJSON := []byte(`{
		"usage": {
			"input_tokens": 1000,
			"output_tokens": 500,
			"cache_creation_input_tokens": 100,
			"cache_read_input_tokens": 50
		}
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ExtractTokensFromJSON(responseJSON)
	}
}

// Benchmark severity determination (should be <10μs per S7 requirements)
func BenchmarkDetermineSeverityLevel(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DetermineSeverityLevel(75000)
	}
}

// Benchmark listener event processing (should be <100μs per S7 requirements)
func BenchmarkListener_OnEvent(b *testing.B) {
	listener := NewTokenSummaryListener()
	event := &telemetry.Event{
		Type:      "claude.api.response",
		Timestamp: time.Now(),
		Level:     telemetry.LevelInfo,
		Agent:     "benchmark",
		Data: map[string]interface{}{
			"usage": TokenUsage{
				InputTokens:  1000,
				OutputTokens: 500,
				TotalTokens:  1500,
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = listener.OnEvent(event)
	}
}

// Benchmark concurrent listener access (thread-safety performance)
func BenchmarkListener_ConcurrentAccess(b *testing.B) {
	listener := NewTokenSummaryListener()
	event := &telemetry.Event{
		Type:      "claude.api.response",
		Timestamp: time.Now(),
		Level:     telemetry.LevelInfo,
		Agent:     "benchmark",
		Data: map[string]interface{}{
			"usage": TokenUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150},
		},
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = listener.OnEvent(event)
		}
	})
}

// Benchmark tracker recording (end-to-end, should be <500μs per S7)
func BenchmarkTracker_RecordResponse(b *testing.B) {
	tracker := NewTokenTracker()
	_ = tracker.Initialize(nil)

	responseJSON := []byte(`{"usage":{"input_tokens":1000,"output_tokens":500}}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tracker.RecordResponse(responseJSON)
	}
}

// Benchmark display summary generation (should be <1ms per S7)
func BenchmarkTracker_DisplaySummary(b *testing.B) {
	tracker := NewTokenTracker()
	_ = tracker.Initialize(nil)

	// Add some sample data
	for i := 0; i < 10; i++ {
		responseJSON := []byte(`{"usage":{"input_tokens":1000,"output_tokens":500}}`)
		_, _ = tracker.RecordResponse(responseJSON)
	}

	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		_ = tracker.DisplaySummary(&buf)
	}
}

// Benchmark JSON summary generation
func BenchmarkTracker_DisplaySummaryJSON(b *testing.B) {
	tracker := NewTokenTracker()
	_ = tracker.Initialize(nil)

	// Add sample data
	for i := 0; i < 10; i++ {
		responseJSON := []byte(`{"usage":{"input_tokens":1000,"output_tokens":500}}`)
		_, _ = tracker.RecordResponse(responseJSON)
	}

	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		_ = tracker.DisplaySummaryJSON(&buf)
	}
}

// Benchmark full session workflow (initialization → recording → display)
func BenchmarkFullSessionWorkflow(b *testing.B) {
	responseJSON := []byte(`{"usage":{"input_tokens":1000,"output_tokens":500}}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker := NewTokenTracker()
		_ = tracker.Initialize(nil)

		// Simulate 5 API calls
		for j := 0; j < 5; j++ {
			_, _ = tracker.RecordResponse(responseJSON)
		}

		// Display summary
		var buf bytes.Buffer
		_ = tracker.DisplaySummary(&buf)
		_ = tracker.Close()
	}
}

// Memory allocation benchmark for listener
func BenchmarkListener_MemoryAllocation(b *testing.B) {
	listener := NewTokenSummaryListener()
	event := &telemetry.Event{
		Type:      "claude.api.response",
		Timestamp: time.Now(),
		Level:     telemetry.LevelInfo,
		Agent:     "benchmark",
		Data: map[string]interface{}{
			"usage": TokenUsage{InputTokens: 1000, OutputTokens: 500, TotalTokens: 1500},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		listener.OnEvent(event)
	}
}

// Memory allocation benchmark for extraction
func BenchmarkExtractTokens_MemoryAllocation(b *testing.B) {
	response := &APIResponse{
		Usage: struct {
			InputTokens         int `json:"input_tokens"`
			OutputTokens        int `json:"output_tokens"`
			CacheCreationTokens int `json:"cache_creation_input_tokens"`
			CacheReadTokens     int `json:"cache_read_input_tokens"`
		}{
			InputTokens:  1000,
			OutputTokens: 500,
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ExtractTokens(response)
	}
}
