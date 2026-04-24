package tokentracking

import (
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/telemetry"
)

// TestIntegration_FullSessionWorkflow simulates a complete CLI session
// with multiple API requests and responses.
func TestIntegration_FullSessionWorkflow(t *testing.T) {
	// Setup: Create listener (simulating session start)
	listener := NewTokenSummaryListener()

	// Simulate a realistic CLI session with 5 API responses
	sessionEvents := []struct {
		description string
		usage       TokenUsage
		wantLevel   telemetry.Level
	}{
		{
			description: "Small initial query",
			usage:       TokenUsage{InputTokens: 500, OutputTokens: 200, TotalTokens: 700},
			wantLevel:   telemetry.LevelInfo,
		},
		{
			description: "Medium code generation",
			usage:       TokenUsage{InputTokens: 5000, OutputTokens: 3000, TotalTokens: 8000},
			wantLevel:   telemetry.LevelInfo,
		},
		{
			description: "Large context analysis",
			usage:       TokenUsage{InputTokens: 20000, OutputTokens: 15000, TotalTokens: 35000},
			wantLevel:   telemetry.LevelInfo,
		},
		{
			description: "High usage warning threshold",
			usage:       TokenUsage{InputTokens: 30000, OutputTokens: 25000, TotalTokens: 55000},
			wantLevel:   telemetry.LevelWarn,
		},
		{
			description: "Final summary query",
			usage:       TokenUsage{InputTokens: 1000, OutputTokens: 500, TotalTokens: 1500},
			wantLevel:   telemetry.LevelWarn, // Highest severity persists
		},
	}

	expectedTotalInput := 0
	expectedTotalOutput := 0

	// Simulate event processing
	for i, test := range sessionEvents {
		event := &telemetry.Event{
			Type:      "claude.api.response",
			Timestamp: time.Now(),
			Level:     test.wantLevel,
			Agent:     "test",
			Data: map[string]interface{}{
				"usage":       test.usage,
				"request_id":  i + 1,
				"description": test.description,
			},
		}

		_ = listener.OnEvent(event)

		expectedTotalInput += test.usage.InputTokens
		expectedTotalOutput += test.usage.OutputTokens

		// Verify incremental state
		summary := listener.GetSummary()
		if summary.ResponseCount != i+1 {
			t.Errorf("After event %d: ResponseCount = %d, want %d", i+1, summary.ResponseCount, i+1)
		}
	}

	// Verify final session summary
	summary := listener.GetSummary()

	if summary.ResponseCount != len(sessionEvents) {
		t.Errorf("Final ResponseCount = %d, want %d", summary.ResponseCount, len(sessionEvents))
	}

	if summary.TotalInputTokens != expectedTotalInput {
		t.Errorf("TotalInputTokens = %d, want %d", summary.TotalInputTokens, expectedTotalInput)
	}

	if summary.TotalOutputTokens != expectedTotalOutput {
		t.Errorf("TotalOutputTokens = %d, want %d", summary.TotalOutputTokens, expectedTotalOutput)
	}

	expectedTotal := expectedTotalInput + expectedTotalOutput
	if summary.TotalTokens != expectedTotal {
		t.Errorf("TotalTokens = %d, want %d", summary.TotalTokens, expectedTotal)
	}

	// Highest severity should be WARN (from event 4)
	if summary.HighestSeverity != telemetry.LevelWarn {
		t.Errorf("HighestSeverity = %v, want WARN", summary.HighestSeverity)
	}
}

// TestIntegration_MixedEventTypes simulates a session with various event types
// where only API response events should be tracked.
func TestIntegration_MixedEventTypes(t *testing.T) {
	listener := NewTokenSummaryListener()

	allEvents := []*telemetry.Event{
		{
			Type:      "system.startup",
			Timestamp: time.Now(),
			Level:     telemetry.LevelInfo,
			Agent:     "test",
			Data:      map[string]interface{}{"version": "1.0.0"},
		},
		{
			Type:      "claude.api.response",
			Timestamp: time.Now(),
			Level:     telemetry.LevelInfo,
			Agent:     "test",
			Data: map[string]interface{}{
				"usage": TokenUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150},
			},
		},
		{
			Type:      "user.input",
			Timestamp: time.Now(),
			Level:     telemetry.LevelInfo,
			Agent:     "test",
			Data:      map[string]interface{}{"command": "help"},
		},
		{
			Type:      "claude.api.request",
			Timestamp: time.Now(),
			Level:     telemetry.LevelInfo,
			Agent:     "test",
			Data: map[string]interface{}{
				"usage": TokenUsage{InputTokens: 999, OutputTokens: 999, TotalTokens: 1998}, // Should be ignored
			},
		},
		{
			Type:      "claude.api.response",
			Timestamp: time.Now(),
			Level:     telemetry.LevelInfo,
			Agent:     "test",
			Data: map[string]interface{}{
				"usage": TokenUsage{InputTokens: 200, OutputTokens: 100, TotalTokens: 300},
			},
		},
		{
			Type:      "telemetry.flush",
			Timestamp: time.Now(),
			Level:     telemetry.LevelInfo,
			Agent:     "test",
			Data:      map[string]interface{}{},
		},
	}

	for _, event := range allEvents {
		_ = listener.OnEvent(event)
	}

	summary := listener.GetSummary()

	// Only 2 API response events should be counted
	if summary.ResponseCount != 2 {
		t.Errorf("ResponseCount = %d, want 2 (only API responses)", summary.ResponseCount)
	}

	// Should only count the 2 valid API responses
	if summary.TotalInputTokens != 300 { // 100 + 200
		t.Errorf("TotalInputTokens = %d, want 300", summary.TotalInputTokens)
	}

	if summary.TotalOutputTokens != 150 { // 50 + 100
		t.Errorf("TotalOutputTokens = %d, want 150", summary.TotalOutputTokens)
	}
}

// TestIntegration_ConcurrentSessionSimulation simulates multiple concurrent
// API requests happening in parallel (realistic for async operations).
func TestIntegration_ConcurrentSessionSimulation(t *testing.T) {
	listener := NewTokenSummaryListener()

	const numConcurrentRequests = 50
	var wg sync.WaitGroup
	wg.Add(numConcurrentRequests)

	// Simulate concurrent API responses (e.g., parallel tool calls)
	for i := 0; i < numConcurrentRequests; i++ {
		go func(requestID int) {
			defer wg.Done()

			// Each request has unique token counts
			event := &telemetry.Event{
				Type:      "claude.api.response",
				Timestamp: time.Now(),
				Level:     telemetry.LevelInfo,
				Agent:     "test",
				Data: map[string]interface{}{
					"usage": TokenUsage{
						InputTokens:  1000 + requestID,
						OutputTokens: 500 + requestID,
						TotalTokens:  1500 + (2 * requestID),
					},
					"request_id": requestID,
				},
			}
			_ = listener.OnEvent(event)
		}(i)
	}

	wg.Wait()

	summary := listener.GetSummary()

	if summary.ResponseCount != numConcurrentRequests {
		t.Errorf("ResponseCount = %d, want %d", summary.ResponseCount, numConcurrentRequests)
	}

	// Calculate expected totals: sum of (1000+i) for i=0..49
	// Sum = 1000*50 + (0+1+2+...+49) = 50000 + (49*50/2) = 50000 + 1225 = 51225
	expectedInput := 50000 + (49 * 50 / 2)
	expectedOutput := 25000 + (49 * 50 / 2)

	if summary.TotalInputTokens != expectedInput {
		t.Errorf("TotalInputTokens = %d, want %d", summary.TotalInputTokens, expectedInput)
	}

	if summary.TotalOutputTokens != expectedOutput {
		t.Errorf("TotalOutputTokens = %d, want %d", summary.TotalOutputTokens, expectedOutput)
	}

	expectedTotal := expectedInput + expectedOutput
	if summary.TotalTokens != expectedTotal {
		t.Errorf("TotalTokens = %d, want %d", summary.TotalTokens, expectedTotal)
	}
}

// TestIntegration_ErrorRecovery verifies listener handles malformed events
// gracefully without crashing or corrupting state.
func TestIntegration_ErrorRecovery(t *testing.T) {
	listener := NewTokenSummaryListener()

	// Mix valid and invalid events
	events := []*telemetry.Event{
		{
			Type:      "claude.api.response",
			Timestamp: time.Now(),
			Level:     telemetry.LevelInfo,
			Agent:     "test",
			Data: map[string]interface{}{
				"usage": TokenUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150},
			},
		},
		{
			Type:      "claude.api.response",
			Timestamp: time.Now(),
			Level:     telemetry.LevelInfo,
			Agent:     "test",
			Data:      nil, // Invalid: nil metadata
		},
		{
			Type:      "claude.api.response",
			Timestamp: time.Now(),
			Level:     telemetry.LevelInfo,
			Agent:     "test",
			Data: map[string]interface{}{
				"usage": "invalid", // Invalid: wrong type
			},
		},
		{
			Type:      "claude.api.response",
			Timestamp: time.Now(),
			Level:     telemetry.LevelInfo,
			Agent:     "test",
			Data: map[string]interface{}{
				"usage": TokenUsage{InputTokens: 200, OutputTokens: 100, TotalTokens: 300},
			},
		},
		{
			Type:      "claude.api.response",
			Timestamp: time.Now(),
			Level:     telemetry.LevelInfo,
			Agent:     "test",
			Data:      map[string]interface{}{}, // Invalid: missing usage
		},
		{
			Type:      "claude.api.response",
			Timestamp: time.Now(),
			Level:     telemetry.LevelInfo,
			Agent:     "test",
			Data: map[string]interface{}{
				"usage": TokenUsage{InputTokens: 50, OutputTokens: 25, TotalTokens: 75},
			},
		},
	}

	for _, event := range events {
		// Should not panic
		_ = listener.OnEvent(event)
	}

	summary := listener.GetSummary()

	// Only 3 valid events should be counted
	if summary.ResponseCount != 3 {
		t.Errorf("ResponseCount = %d, want 3 (only valid events)", summary.ResponseCount)
	}

	expectedInput := 100 + 200 + 50
	if summary.TotalInputTokens != expectedInput {
		t.Errorf("TotalInputTokens = %d, want %d", summary.TotalInputTokens, expectedInput)
	}
}

// TestIntegration_CacheTokenTracking verifies cache token fields are
// properly tracked across the session.
func TestIntegration_CacheTokenTracking(t *testing.T) {
	listener := NewTokenSummaryListener()

	events := []TokenUsage{
		{
			InputTokens:         10000,
			OutputTokens:        5000,
			CacheCreationTokens: 2000,
			CacheReadTokens:     1000,
			TotalTokens:         15000,
		},
		{
			InputTokens:         5000,
			OutputTokens:        2500,
			CacheCreationTokens: 0, // No cache creation
			CacheReadTokens:     500,
			TotalTokens:         7500,
		},
		{
			InputTokens:         3000,
			OutputTokens:        1500,
			CacheCreationTokens: 100,
			CacheReadTokens:     0, // No cache read
			TotalTokens:         4500,
		},
	}

	for _, usage := range events {
		_ = listener.OnEvent(&telemetry.Event{
			Type:      "claude.api.response",
			Timestamp: time.Now(),
			Level:     telemetry.LevelInfo,
			Agent:     "test",
			Data: map[string]interface{}{
				"usage": usage,
			},
		})
	}

	summary := listener.GetSummary()

	expectedCacheCreation := 2000 + 0 + 100
	expectedCacheRead := 1000 + 500 + 0

	if summary.TotalCacheCreationTokens != expectedCacheCreation {
		t.Errorf("TotalCacheCreationTokens = %d, want %d", summary.TotalCacheCreationTokens, expectedCacheCreation)
	}

	if summary.TotalCacheReadTokens != expectedCacheRead {
		t.Errorf("TotalCacheReadTokens = %d, want %d", summary.TotalCacheReadTokens, expectedCacheRead)
	}
}
