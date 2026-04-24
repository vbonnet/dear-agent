package tokentracking

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// TokenTracker provides the main API for tracking Claude API token usage
// throughout a CLI session. Integrates with P1 Telemetry Foundation when
// available, operates standalone otherwise.
//
// Thread-safe: All methods can be called concurrently.
type TokenTracker struct {
	listener       *TokenSummaryListener
	telemetryAvail bool // True if P1 Telemetry is available
	sessionStarted time.Time
}

// NewTokenTracker creates a new tracker instance.
// Does NOT register with telemetry - call Initialize() for that.
func NewTokenTracker() *TokenTracker {
	return &TokenTracker{
		listener:       NewTokenSummaryListener(),
		telemetryAvail: false,
		sessionStarted: time.Now(),
	}
}

// Initialize sets up the tracker for a new CLI session.
// Registers with P1 Telemetry Collector when provided.
//
// Parameters:
//
//	collector: P1 Telemetry Collector instance (nil means standalone mode)
//
// Returns error only on critical failures (nil means success or graceful degradation).
func (t *TokenTracker) Initialize(collector *telemetry.Collector) error {
	t.sessionStarted = time.Now()

	if collector != nil {
		collector.AddListener(t.listener)
		t.telemetryAvail = true
	} else {
		t.telemetryAvail = false
	}

	return nil
}

// RecordResponse processes a Claude API response and extracts token usage.
// Automatically records to telemetry if available.
//
// Parameters:
//
//	responseJSON: Raw JSON response from Claude API
//
// Returns:
//
//	usage: Extracted token counts (nil on error)
//	error: Parsing or validation failure
func (t *TokenTracker) RecordResponse(responseJSON []byte) (*TokenUsage, error) {
	// Extract tokens from response
	usage, err := ExtractTokensFromJSON(responseJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to extract tokens: %w", err)
	}

	// Record to telemetry or directly to listener
	if !t.telemetryAvail {
		// Standalone mode: directly notify listener
		event := &telemetry.Event{
			Type:      "claude.api.response",
			Timestamp: time.Now(),
			Level:     DetermineSeverityLevel(usage.TotalTokens),
			Agent:     "tokentracker",
			Data: map[string]interface{}{
				"usage": *usage,
			},
		}
		_ = t.listener.OnEvent(event) // Ignore error (graceful degradation)
	}
	// If telemetry available, events flow through collector automatically

	return usage, nil
}

// RecordResponseCtx processes a Claude API response with OTel span creation.
// Creates a "claude_api_call" leaf span with gen_ai.* semantic convention attributes.
func (t *TokenTracker) RecordResponseCtx(ctx context.Context, responseJSON []byte) (*TokenUsage, error) {
	tracer := otel.Tracer("engram/tokentracking")
	_, span := tracer.Start(ctx, "claude_api_call")
	defer span.End()

	usage, err := t.RecordResponse(responseJSON)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(
		attribute.Int("gen_ai.usage.input_tokens", usage.InputTokens),
		attribute.Int("gen_ai.usage.output_tokens", usage.OutputTokens),
		attribute.String("gen_ai.system", "anthropic"),
	)

	// Try to extract model from the response JSON
	var partial struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(responseJSON, &partial) == nil && partial.Model != "" {
		span.SetAttributes(attribute.String("gen_ai.request.model", partial.Model))
	}

	return usage, nil
}

// RecordResponseFromStruct processes a pre-parsed API response.
// Convenience method for when response is already unmarshaled.
func (t *TokenTracker) RecordResponseFromStruct(response *APIResponse) (*TokenUsage, error) {
	usage, err := ExtractTokens(response)
	if err != nil {
		return nil, fmt.Errorf("failed to extract tokens: %w", err)
	}

	// Record to listener (same logic as RecordResponse)
	if !t.telemetryAvail {
		event := &telemetry.Event{
			Type:      "claude.api.response",
			Timestamp: time.Now(),
			Level:     DetermineSeverityLevel(usage.TotalTokens),
			Agent:     "tokentracker",
			Data: map[string]interface{}{
				"usage": *usage,
			},
		}
		_ = t.listener.OnEvent(event) // Ignore error
	}

	return usage, nil
}

// GetSummary returns current session statistics.
// Can be called at any time during the session.
func (t *TokenTracker) GetSummary() SessionSummary {
	return t.listener.GetSummary()
}

// DisplaySummary outputs a formatted session summary to the specified writer.
// Typically called at session end (pre-exit hook).
//
// Parameters:
//
//	w: Output destination (use os.Stderr for CLI)
//
// Returns error only on write failures.
func (t *TokenTracker) DisplaySummary(w io.Writer) error {
	summary := t.listener.GetSummary()

	// Don't display if no API calls were made
	if summary.ResponseCount == 0 {
		return nil
	}

	// Calculate session duration
	duration := time.Since(t.sessionStarted)

	// Format summary output
	output := fmt.Sprintf(`
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📊 Claude CLI Session Token Summary
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Session Duration:  %s
API Responses:     %d

Token Usage:
  Input:           %d tokens
  Output:          %d tokens
  Cache Creation:  %d tokens
  Cache Read:      %d tokens
  ─────────────────────────────────
  Total:           %d tokens

Severity Level:    %s
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

`,
		formatDuration(duration),
		summary.ResponseCount,
		summary.TotalInputTokens,
		summary.TotalOutputTokens,
		summary.TotalCacheCreationTokens,
		summary.TotalCacheReadTokens,
		summary.TotalTokens,
		formatSeverity(summary.HighestSeverity),
	)

	_, err := fmt.Fprint(w, output)
	return err
}

// DisplaySummaryJSON outputs session summary as JSON.
// Useful for programmatic consumption or logging.
func (t *TokenTracker) DisplaySummaryJSON(w io.Writer) error {
	summary := t.listener.GetSummary()

	type jsonSummary struct {
		SessionDuration          string `json:"session_duration"`
		ResponseCount            int    `json:"response_count"`
		TotalInputTokens         int    `json:"total_input_tokens"`
		TotalOutputTokens        int    `json:"total_output_tokens"`
		TotalCacheCreationTokens int    `json:"total_cache_creation_tokens"`
		TotalCacheReadTokens     int    `json:"total_cache_read_tokens"`
		TotalTokens              int    `json:"total_tokens"`
		HighestSeverity          string `json:"highest_severity"`
		Timestamp                string `json:"timestamp"`
	}

	js := jsonSummary{
		SessionDuration:          time.Since(t.sessionStarted).String(),
		ResponseCount:            summary.ResponseCount,
		TotalInputTokens:         summary.TotalInputTokens,
		TotalOutputTokens:        summary.TotalOutputTokens,
		TotalCacheCreationTokens: summary.TotalCacheCreationTokens,
		TotalCacheReadTokens:     summary.TotalCacheReadTokens,
		TotalTokens:              summary.TotalTokens,
		HighestSeverity:          formatSeverity(summary.HighestSeverity),
		Timestamp:                time.Now().Format(time.RFC3339),
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(js)
}

// Close performs cleanup and final telemetry flush.
// Should be called at session end, typically in pre-exit hook.
func (t *TokenTracker) Close() error {
	// In production with P1, this would:
	// - Unregister listener from telemetry
	// - Flush any pending events
	// - Clean up resources

	// For standalone mode, nothing to clean up
	return nil
}

// formatDuration converts duration to human-readable string
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

// formatSeverity converts Level to human-readable string with emoji
func formatSeverity(level telemetry.Level) string {
	switch level {
	case telemetry.LevelInfo:
		return "ℹ️  INFO (Normal usage)"
	case telemetry.LevelWarn:
		return "⚠️  WARN (High usage ≥50K tokens)"
	case telemetry.LevelError:
		return "🚨 ERROR (Very high usage ≥100K tokens)"
	case telemetry.LevelCritical:
		return "💥 CRITICAL (System failure)"
	default:
		return fmt.Sprintf("UNKNOWN (%d)", level)
	}
}

// Global singleton instance for CLI hooks to use
var defaultTracker *TokenTracker

// GetDefaultTracker returns the singleton tracker instance.
// Automatically initializes on first call (standalone mode with nil collector).
func GetDefaultTracker() *TokenTracker {
	if defaultTracker == nil {
		defaultTracker = NewTokenTracker()
		_ = defaultTracker.Initialize(nil) // Standalone mode
	}
	return defaultTracker
}

// ResetDefaultTracker resets the singleton (useful for testing).
func ResetDefaultTracker() {
	if defaultTracker != nil {
		_ = defaultTracker.Close()
	}
	defaultTracker = nil
}

// DisplaySummaryToStderr is a convenience function for CLI hooks.
// Displays summary to stderr using the default tracker.
func DisplaySummaryToStderr() error {
	return GetDefaultTracker().DisplaySummary(os.Stderr)
}
