package tokentracking

import (
	"sync"

	"github.com/vbonnet/dear-agent/internal/telemetry"
)

// SessionSummary contains aggregated token usage for an entire CLI session.
type SessionSummary struct {
	TotalInputTokens         int
	TotalOutputTokens        int
	TotalCacheCreationTokens int
	TotalCacheReadTokens     int
	TotalTokens              int
	ResponseCount            int
	HighestSeverity          telemetry.Level
}

// TokenSummaryListener accumulates token usage across a Claude CLI session.
// Implements EventListener interface for integration with P1 Telemetry.
//
// Thread-safe: Multiple goroutines can call OnEvent() concurrently.
type TokenSummaryListener struct {
	mu              sync.Mutex
	responses       []TokenUsage // All individual responses
	totalInput      int
	totalOutput     int
	totalCache      int
	totalCacheRead  int
	responseCount   int
	highestSeverity telemetry.Level
}

// NewTokenSummaryListener creates a new listener instance.
func NewTokenSummaryListener() *TokenSummaryListener {
	return &TokenSummaryListener{
		responses:       make([]TokenUsage, 0),
		highestSeverity: telemetry.LevelInfo,
	}
}

// OnEvent handles telemetry events and extracts token usage.
// Filters for "claude.api.response" events and accumulates token counts.
//
// Expected event data format:
//   - "usage": TokenUsage struct with token counts
func (l *TokenSummaryListener) OnEvent(event *telemetry.Event) error {
	// Filter: Only process Claude API response events
	if event.Type != "claude.api.response" {
		return nil
	}

	// Extract TokenUsage from event data
	usageData, ok := event.Data["usage"]
	if !ok {
		return nil
	}

	usage, ok := usageData.(TokenUsage)
	if !ok {
		// Try pointer type
		usagePtr, ok := usageData.(*TokenUsage)
		if !ok {
			return nil
		}
		usage = *usagePtr
	}

	// Accumulate (thread-safe)
	l.mu.Lock()
	defer l.mu.Unlock()

	l.responses = append(l.responses, usage)
	l.totalInput += usage.InputTokens
	l.totalOutput += usage.OutputTokens
	l.totalCache += usage.CacheCreationTokens
	l.totalCacheRead += usage.CacheReadTokens
	l.responseCount++

	// Track highest severity seen in session
	severity := DetermineSeverityLevel(usage.TotalTokens)
	if severity > l.highestSeverity {
		l.highestSeverity = severity
	}

	return nil
}

// GetSummary returns aggregated statistics for the entire session.
// Thread-safe: Can be called while OnEvent() is processing events.
func (l *TokenSummaryListener) GetSummary() SessionSummary {
	l.mu.Lock()
	defer l.mu.Unlock()

	return SessionSummary{
		TotalInputTokens:         l.totalInput,
		TotalOutputTokens:        l.totalOutput,
		TotalCacheCreationTokens: l.totalCache,
		TotalCacheReadTokens:     l.totalCacheRead,
		TotalTokens:              l.totalInput + l.totalOutput,
		ResponseCount:            l.responseCount,
		HighestSeverity:          l.highestSeverity,
	}
}

// MinLevel returns the minimum telemetry level this listener wants to receive.
// Implements EventListener interface requirement.
func (l *TokenSummaryListener) MinLevel() telemetry.Level {
	return telemetry.LevelInfo // Accept all events
}

// Reset clears all accumulated state (useful for testing).
func (l *TokenSummaryListener) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.responses = make([]TokenUsage, 0)
	l.totalInput = 0
	l.totalOutput = 0
	l.totalCache = 0
	l.totalCacheRead = 0
	l.responseCount = 0
	l.highestSeverity = telemetry.LevelInfo
}
