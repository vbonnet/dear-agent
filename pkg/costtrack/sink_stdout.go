package costtrack

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// StdoutSink writes cost events to stderr as JSON
type StdoutSink struct {
	prefix string // Prefix for log lines (default: "[COST_TRACKING]")
}

// NewStdoutSink creates a new stdout cost sink
func NewStdoutSink() *StdoutSink {
	return &StdoutSink{
		prefix: "[COST_TRACKING]",
	}
}

// Record writes cost event to stderr as JSON
func (s *StdoutSink) Record(ctx context.Context, cost *CostInfo, meta *CostMetadata) error {
	event := map[string]any{
		"timestamp": meta.Timestamp.Format("2006-01-02T15:04:05.000Z07:00"),
		"operation": meta.Operation,
		"provider":  cost.Provider,
		"model":     cost.Model,
		"tokens": map[string]int{
			"input":       cost.Tokens.Input,
			"output":      cost.Tokens.Output,
			"cache_write": cost.Tokens.CacheWrite,
			"cache_read":  cost.Tokens.CacheRead,
		},
		"cost": map[string]float64{
			"input":       cost.Cost.Input,
			"output":      cost.Cost.Output,
			"cache_write": cost.Cost.CacheWrite,
			"cache_read":  cost.Cost.CacheRead,
			"total":       cost.Cost.Total,
		},
	}

	// Add component if provided
	if cost.Component != "" {
		event["component"] = cost.Component
	}

	// Add cache metrics if available
	if cost.Cache != nil {
		event["cache"] = map[string]float64{
			"hit_rate": cost.Cache.HitRate,
			"savings":  cost.Cache.Savings,
		}
	}

	// Add context if provided
	if meta.Context != "" {
		event["context"] = meta.Context
	}

	// Add request ID if provided
	if meta.RequestID != "" {
		event["request_id"] = meta.RequestID
	}

	// Marshal to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal cost event: %w", err)
	}

	// Write to stderr with prefix
	fmt.Fprintf(os.Stderr, "%s %s\n", s.prefix, string(data))

	return nil
}

// Close is a no-op for stdout sink
func (s *StdoutSink) Close(ctx context.Context) error {
	return nil
}
