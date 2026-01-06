package tokenlogger

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/vbonnet/engram/core/pkg/telemetry"
)

// writeEvent writes a telemetry event to the specified log file in JSONL format.
// Uses O_APPEND for atomic writes and creates file with 0600 permissions (owner read/write only).
func writeEvent(path string, event *telemetry.Event) error {
	// Open file with append mode, create if missing, owner-only permissions
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open telemetry file: %w", err)
	}
	defer file.Close()

	// Marshal event to JSON
	line, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Append newline for JSONL format
	line = append(line, '\n')

	// Write to file
	if _, err := file.Write(line); err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}

	return nil
}
