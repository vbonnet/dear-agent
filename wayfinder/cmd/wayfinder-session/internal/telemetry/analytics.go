package telemetry

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// ReadQualityEvents reads a JSONL telemetry file and returns only quality-assessed events.
// Returns an empty slice (not an error) if the file does not exist.
func ReadQualityEvents(telemetryPath string) ([]QualityAssessedEvent, error) {
	f, err := os.Open(telemetryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open telemetry file: %w", err)
	}
	defer f.Close()

	var events []QualityAssessedEvent
	scanner := bufio.NewScanner(f)

	// Allow lines up to 1MB
	const maxLineSize = 1 << 20
	scanner.Buffer(make([]byte, 0, maxLineSize), maxLineSize)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Quick pre-check: skip lines that cannot be quality events
		if !strings.Contains(line, EventType) {
			continue
		}

		var event QualityAssessedEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			// Skip malformed lines
			continue
		}

		if event.Type == EventType {
			events = append(events, event)
		}
	}

	if err := scanner.Err(); err != nil {
		return events, fmt.Errorf("error reading telemetry file: %w", err)
	}

	return events, nil
}

// GenerateCSV writes quality events as CSV to the given writer.
// Columns: phase, score, input_tokens, output_tokens, context_sources, project_name, timestamp
func GenerateCSV(events []QualityAssessedEvent, w io.Writer) error {
	// Write header
	if _, err := fmt.Fprintln(w, "phase,score,input_tokens,output_tokens,context_sources,project_name,timestamp"); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	for _, e := range events {
		sources := strings.Join(e.ContextSources, ";")
		// Quote sources field since it may contain special characters
		quotedSources := `"` + strings.ReplaceAll(sources, `"`, `""`) + `"`

		line := fmt.Sprintf("%s,%.1f,%d,%d,%s,%s,%s",
			e.Phase,
			e.Score,
			e.InputTokens,
			e.OutputTokens,
			quotedSources,
			e.ProjectName,
			e.Timestamp.UTC().Format("2006-01-02T15:04:05Z"),
		)

		if _, err := fmt.Fprintln(w, line); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	return nil
}
