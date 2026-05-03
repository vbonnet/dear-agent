package costtrack

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// FileSink writes cost events to a JSONL file
type FileSink struct {
	path string
	file *os.File
	mu   sync.Mutex // Thread-safe writes
}

// NewFileSink creates a new file cost sink
func NewFileSink(path string) (*FileSink, error) {
	// Open file in append mode, create if doesn't exist
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to open cost file: %w", err)
	}

	return &FileSink{
		path: path,
		file: file,
	}, nil
}

// Record writes cost event to file as JSONL
func (s *FileSink) Record(ctx context.Context, cost *CostInfo, meta *CostMetadata) error {
	s.mu.Lock()
	defer s.mu.Unlock()

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

	// Write JSONL (one JSON object per line)
	if _, err := fmt.Fprintf(s.file, "%s\n", string(data)); err != nil {
		return fmt.Errorf("failed to write cost event: %w", err)
	}

	return nil
}

// Close flushes and closes the file
func (s *FileSink) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.file != nil {
		if err := s.file.Sync(); err != nil {
			return fmt.Errorf("failed to sync file: %w", err)
		}
		if err := s.file.Close(); err != nil {
			return fmt.Errorf("failed to close file: %w", err)
		}
		s.file = nil
	}

	return nil
}
