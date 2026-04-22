// Package analysis provides tools for analyzing telemetry data.
//
// The analysis package includes:
//   - JSONL parser for streaming telemetry events
//   - Sanity check aggregator for weekly reports
//   - Output renderers (ASCII tables, accessible format)
//
// Example usage:
//
//	events, errs := ParseJSONL("~/.engram/telemetry/events.jsonl")
//	for event := range events {
//	    summary := AggregateSanityChecks(event)
//	    RenderASCIITable(summary)
//	}
package analysis

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// TelemetryEvent represents a parsed telemetry event
type TelemetryEvent struct {
	ID            string                 `json:"id,omitempty"`
	Timestamp     time.Time              `json:"timestamp"`
	Type          string                 `json:"type"`
	Agent         string                 `json:"agent"`
	SchemaVersion string                 `json:"schema_version,omitempty"`
	Data          map[string]interface{} `json:"data,omitempty"`
}

// ParseJSONL parses a JSONL telemetry file and streams events
//
// Returns two channels:
//   - events: Stream of successfully parsed events
//   - errs: Stream of parsing errors (does not stop parsing)
//
// The parser is resilient to:
//   - Malformed JSON (skips and continues)
//   - Truncated last line (skips and continues)
//   - Empty lines (skips)
//   - Mixed schema versions (handles via version field)
func ParseJSONL(path string) (<-chan *TelemetryEvent, <-chan error) {
	events := make(chan *TelemetryEvent, 100)
	errs := make(chan error, 10)

	go func() {
		defer close(events)
		defer close(errs)

		file, err := os.Open(path)
		if err != nil {
			errs <- fmt.Errorf("failed to open file: %w", err)
			return
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNum := 0

		for scanner.Scan() {
			lineNum++
			line := scanner.Text()

			// Skip empty lines
			if len(line) == 0 {
				continue
			}

			// Parse JSON
			var event TelemetryEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				// Log error but continue parsing (resilience)
				// Non-blocking send to prevent parser from blocking if consumer is slow
				select {
				case errs <- fmt.Errorf("line %d: malformed JSON: %w", lineNum, err):
				default:
					// Consumer not keeping up with errors, drop this error
				}
				continue
			}

			// Default schema version to 1.0.0 if not set (backward compatibility - S7 P1.15)
			if event.SchemaVersion == "" {
				event.SchemaVersion = "1.0.0"
			}

			// Send event to channel
			events <- &event
		}

		if err := scanner.Err(); err != nil {
			select {
			case errs <- fmt.Errorf("scanner error: %w", err):
			default:
				// Consumer not keeping up with errors, drop this error
			}
		}
	}()

	return events, errs
}

// ParseJSONLSync parses a JSONL file synchronously and returns all events
//
// Use ParseJSONL for streaming large files (lower memory usage).
// Use ParseJSONLSync for small files or when you need all events at once.
func ParseJSONLSync(path string) ([]*TelemetryEvent, error) {
	events := make([]*TelemetryEvent, 0)
	eventsChan, errsChan := ParseJSONL(path)

	// Collect all errors
	var parseErrors []error

	// Drain all events, then drain any remaining errors.
	// ParseJSONL closes errsChan before eventsChan (defer LIFO),
	// so by the time eventsChan is exhausted, errsChan is already closed.
	for event := range eventsChan {
		events = append(events, event)
	}
	for err := range errsChan {
		parseErrors = append(parseErrors, err)
	}

	if len(parseErrors) > 0 {
		return events, fmt.Errorf("encountered %d parsing errors (first: %w)", len(parseErrors), parseErrors[0])
	}
	return events, nil
}
