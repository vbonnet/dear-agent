package tokentracking

import (
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/telemetry"
)

func TestNewTokenSummaryListener(t *testing.T) {
	listener := NewTokenSummaryListener()
	if listener == nil {
		t.Fatal("NewTokenSummaryListener() returned nil")
	}

	summary := listener.GetSummary()
	if summary.ResponseCount != 0 {
		t.Errorf("New listener should have 0 responses, got %d", summary.ResponseCount)
	}
	if summary.TotalTokens != 0 {
		t.Errorf("New listener should have 0 total tokens, got %d", summary.TotalTokens)
	}
	if summary.HighestSeverity != telemetry.LevelInfo {
		t.Errorf("New listener should have INFO severity, got %v", summary.HighestSeverity)
	}
}

func TestOnEvent_FiltersByEventType(t *testing.T) {
	listener := NewTokenSummaryListener()

	// Non-matching event types should be ignored
	ignoredEvents := []string{
		"claude.api.request",
		"system.startup",
		"user.input",
		"",
	}

	for _, eventType := range ignoredEvents {
		event := &telemetry.Event{
			Type:      eventType,
			Timestamp: time.Now(),
			Level:     telemetry.LevelInfo,
			Agent:     "test",
			Data: map[string]interface{}{
				"usage": TokenUsage{
					InputTokens:  100,
					OutputTokens: 50,
					TotalTokens:  150,
				},
			},
		}
		_ = listener.OnEvent(event)
	}

	summary := listener.GetSummary()
	if summary.ResponseCount != 0 {
		t.Errorf("Listener processed %d events, should ignore non-matching types", summary.ResponseCount)
	}
}

func TestOnEvent_AccumulatesTokenUsage(t *testing.T) {
	listener := NewTokenSummaryListener()

	// Simulate 3 API responses
	responses := []TokenUsage{
		{InputTokens: 100, OutputTokens: 50, CacheCreationTokens: 10, CacheReadTokens: 5, TotalTokens: 150},
		{InputTokens: 200, OutputTokens: 100, CacheCreationTokens: 20, CacheReadTokens: 10, TotalTokens: 300},
		{InputTokens: 50, OutputTokens: 25, CacheCreationTokens: 5, CacheReadTokens: 2, TotalTokens: 75},
	}

	for _, usage := range responses {
		event := &telemetry.Event{
			Type:      "claude.api.response",
			Timestamp: time.Now(),
			Level:     telemetry.LevelInfo,
			Agent:     "test",
			Data: map[string]interface{}{
				"usage": usage,
			},
		}
		_ = listener.OnEvent(event)
	}

	summary := listener.GetSummary()

	// Validate aggregated totals
	expectedInput := 100 + 200 + 50
	expectedOutput := 50 + 100 + 25
	expectedCacheCreation := 10 + 20 + 5
	expectedCacheRead := 5 + 10 + 2
	expectedTotal := expectedInput + expectedOutput

	if summary.TotalInputTokens != expectedInput {
		t.Errorf("TotalInputTokens = %d, want %d", summary.TotalInputTokens, expectedInput)
	}
	if summary.TotalOutputTokens != expectedOutput {
		t.Errorf("TotalOutputTokens = %d, want %d", summary.TotalOutputTokens, expectedOutput)
	}
	if summary.TotalCacheCreationTokens != expectedCacheCreation {
		t.Errorf("TotalCacheCreationTokens = %d, want %d", summary.TotalCacheCreationTokens, expectedCacheCreation)
	}
	if summary.TotalCacheReadTokens != expectedCacheRead {
		t.Errorf("TotalCacheReadTokens = %d, want %d", summary.TotalCacheReadTokens, expectedCacheRead)
	}
	if summary.TotalTokens != expectedTotal {
		t.Errorf("TotalTokens = %d, want %d", summary.TotalTokens, expectedTotal)
	}
	if summary.ResponseCount != 3 {
		t.Errorf("ResponseCount = %d, want 3", summary.ResponseCount)
	}
}

func TestOnEvent_TracksHighestSeverity(t *testing.T) {
	listener := NewTokenSummaryListener()

	// Start with INFO level response
	_ = listener.OnEvent(&telemetry.Event{
		Type:      "claude.api.response",
		Timestamp: time.Now(),
		Level:     telemetry.LevelInfo,
		Agent:     "test",
		Data: map[string]interface{}{
			"usage": TokenUsage{InputTokens: 1000, OutputTokens: 500, TotalTokens: 1500},
		},
	})

	summary := listener.GetSummary()
	if summary.HighestSeverity != telemetry.LevelInfo {
		t.Errorf("HighestSeverity = %v, want INFO", summary.HighestSeverity)
	}

	// Add WARN level response (50K tokens)
	_ = listener.OnEvent(&telemetry.Event{
		Type:      "claude.api.response",
		Timestamp: time.Now(),
		Level:     telemetry.LevelInfo,
		Agent:     "test",
		Data: map[string]interface{}{
			"usage": TokenUsage{InputTokens: 30000, OutputTokens: 25000, TotalTokens: 55000},
		},
	})

	summary = listener.GetSummary()
	if summary.HighestSeverity != telemetry.LevelWarn {
		t.Errorf("HighestSeverity = %v, want WARN after 55K tokens", summary.HighestSeverity)
	}

	// Add ERROR level response (100K tokens)
	_ = listener.OnEvent(&telemetry.Event{
		Type:      "claude.api.response",
		Timestamp: time.Now(),
		Level:     telemetry.LevelInfo,
		Agent:     "test",
		Data: map[string]interface{}{
			"usage": TokenUsage{InputTokens: 60000, OutputTokens: 50000, TotalTokens: 110000},
		},
	})

	summary = listener.GetSummary()
	if summary.HighestSeverity != telemetry.LevelError {
		t.Errorf("HighestSeverity = %v, want ERROR after 110K tokens", summary.HighestSeverity)
	}

	// Add another INFO level - should NOT downgrade severity
	_ = listener.OnEvent(&telemetry.Event{
		Type:      "claude.api.response",
		Timestamp: time.Now(),
		Level:     telemetry.LevelInfo,
		Agent:     "test",
		Data: map[string]interface{}{
			"usage": TokenUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150},
		},
	})

	summary = listener.GetSummary()
	if summary.HighestSeverity != telemetry.LevelError {
		t.Errorf("HighestSeverity = %v, should stay at ERROR (no downgrade)", summary.HighestSeverity)
	}
}

func TestOnEvent_HandlesPointerUsage(t *testing.T) {
	listener := NewTokenSummaryListener()

	// Test with pointer to TokenUsage (alternative metadata format)
	usage := &TokenUsage{
		InputTokens:  100,
		OutputTokens: 50,
		TotalTokens:  150,
	}

	event := &telemetry.Event{
		Type:      "claude.api.response",
		Timestamp: time.Now(),
		Level:     telemetry.LevelInfo,
		Agent:     "test",
		Data: map[string]interface{}{
			"usage": usage,
		},
	}
	_ = listener.OnEvent(event)

	summary := listener.GetSummary()
	if summary.ResponseCount != 1 {
		t.Errorf("Should process pointer usage, got %d responses", summary.ResponseCount)
	}
	if summary.TotalInputTokens != 100 {
		t.Errorf("TotalInputTokens = %d, want 100", summary.TotalInputTokens)
	}
}

func TestOnEvent_IgnoresInvalidMetadata(t *testing.T) {
	listener := NewTokenSummaryListener()

	invalidEvents := []*telemetry.Event{
		{
			Type:      "claude.api.response",
			Timestamp: time.Now(),
			Level:     telemetry.LevelInfo,
			Agent:     "test",
			Data:      map[string]interface{}{}, // Missing "usage" key
		},
		{
			Type:      "claude.api.response",
			Timestamp: time.Now(),
			Level:     telemetry.LevelInfo,
			Agent:     "test",
			Data: map[string]interface{}{
				"usage": "invalid type", // Wrong type
			},
		},
		{
			Type:      "claude.api.response",
			Timestamp: time.Now(),
			Level:     telemetry.LevelInfo,
			Agent:     "test",
			Data: map[string]interface{}{
				"usage": 12345, // Wrong type
			},
		},
		{
			Type:      "claude.api.response",
			Timestamp: time.Now(),
			Level:     telemetry.LevelInfo,
			Agent:     "test",
			Data:      nil, // Nil metadata
		},
	}

	for i, event := range invalidEvents {
		_ = listener.OnEvent(event)
		summary := listener.GetSummary()
		if summary.ResponseCount != 0 {
			t.Errorf("Test %d: Should ignore invalid metadata, got %d responses", i, summary.ResponseCount)
		}
	}
}

func TestOnEvent_ThreadSafety(t *testing.T) {
	listener := NewTokenSummaryListener()

	// Simulate concurrent API responses from multiple goroutines
	const numGoroutines = 10
	const eventsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				event := &telemetry.Event{
					Type:      "claude.api.response",
					Timestamp: time.Now(),
					Level:     telemetry.LevelInfo,
					Agent:     "test",
					Data: map[string]interface{}{
						"usage": TokenUsage{
							InputTokens:  10,
							OutputTokens: 5,
							TotalTokens:  15,
						},
					},
				}
				_ = listener.OnEvent(event)
			}
		}()
	}

	wg.Wait()

	summary := listener.GetSummary()
	expectedCount := numGoroutines * eventsPerGoroutine
	expectedInput := expectedCount * 10
	expectedOutput := expectedCount * 5
	expectedTotal := expectedCount * 15

	if summary.ResponseCount != expectedCount {
		t.Errorf("ResponseCount = %d, want %d (lost updates due to race)", summary.ResponseCount, expectedCount)
	}
	if summary.TotalInputTokens != expectedInput {
		t.Errorf("TotalInputTokens = %d, want %d (lost updates due to race)", summary.TotalInputTokens, expectedInput)
	}
	if summary.TotalOutputTokens != expectedOutput {
		t.Errorf("TotalOutputTokens = %d, want %d (lost updates due to race)", summary.TotalOutputTokens, expectedOutput)
	}
	if summary.TotalTokens != expectedTotal {
		t.Errorf("TotalTokens = %d, want %d (lost updates due to race)", summary.TotalTokens, expectedTotal)
	}
}

func TestReset(t *testing.T) {
	listener := NewTokenSummaryListener()

	// Add some events
	for i := 0; i < 5; i++ {
		_ = listener.OnEvent(&telemetry.Event{
			Type:      "claude.api.response",
			Timestamp: time.Now(),
			Level:     telemetry.LevelInfo,
			Agent:     "test",
			Data: map[string]interface{}{
				"usage": TokenUsage{
					InputTokens:  100,
					OutputTokens: 50,
					TotalTokens:  150,
				},
			},
		})
	}

	// Verify data accumulated
	summary := listener.GetSummary()
	if summary.ResponseCount != 5 {
		t.Fatalf("Setup failed: expected 5 responses, got %d", summary.ResponseCount)
	}

	// Reset and verify cleared
	listener.Reset()
	summary = listener.GetSummary()

	if summary.ResponseCount != 0 {
		t.Errorf("After Reset: ResponseCount = %d, want 0", summary.ResponseCount)
	}
	if summary.TotalInputTokens != 0 {
		t.Errorf("After Reset: TotalInputTokens = %d, want 0", summary.TotalInputTokens)
	}
	if summary.TotalOutputTokens != 0 {
		t.Errorf("After Reset: TotalOutputTokens = %d, want 0", summary.TotalOutputTokens)
	}
	if summary.TotalTokens != 0 {
		t.Errorf("After Reset: TotalTokens = %d, want 0", summary.TotalTokens)
	}
	if summary.HighestSeverity != telemetry.LevelInfo {
		t.Errorf("After Reset: HighestSeverity = %v, want INFO", summary.HighestSeverity)
	}
}

func TestGetSummary_ThreadSafety(t *testing.T) {
	listener := NewTokenSummaryListener()

	// Simulate concurrent reads and writes
	var wg sync.WaitGroup
	wg.Add(2)

	// Writer goroutine
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_ = listener.OnEvent(&telemetry.Event{
				Type:      "claude.api.response",
				Timestamp: time.Now(),
				Level:     telemetry.LevelInfo,
				Agent:     "test",
				Data: map[string]interface{}{
					"usage": TokenUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
				},
			})
		}
	}()

	// Reader goroutine
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_ = listener.GetSummary() // Should not panic or deadlock
		}
	}()

	wg.Wait()

	// Verify final state is consistent
	summary := listener.GetSummary()
	if summary.ResponseCount != 1000 {
		t.Errorf("Final ResponseCount = %d, want 1000", summary.ResponseCount)
	}
	expectedTotal := summary.TotalInputTokens + summary.TotalOutputTokens
	if summary.TotalTokens != expectedTotal {
		t.Errorf("TotalTokens inconsistent: %d != %d + %d", summary.TotalTokens, summary.TotalInputTokens, summary.TotalOutputTokens)
	}
}
