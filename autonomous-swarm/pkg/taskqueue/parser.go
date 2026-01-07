package taskqueue

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ParseTaskQueue reads and validates a TASK-QUEUE.yaml file
func ParseTaskQueue(filePath string) (*TaskQueue, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read task queue file: %w", err)
	}

	var tq TaskQueue
	if err := yaml.Unmarshal(data, &tq); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if err := validateTaskQueue(&tq); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return &tq, nil
}

// validateTaskQueue validates the TaskQueue structure
func validateTaskQueue(tq *TaskQueue) error {
	// Validate schema version
	if tq.SchemaVersion == "" {
		return fmt.Errorf("schema_version is required")
	}

	// Validate all beads in all sections
	sections := map[string][]Bead{
		"ready":       tq.Ready,
		"in_progress": tq.InProgress,
		"blocked":     tq.Blocked,
		"completed":   tq.Completed,
	}

	for section, beads := range sections {
		for i, bead := range beads {
			if err := validateBead(&bead, section, i); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateBead validates a single Bead
func validateBead(bead *Bead, section string, index int) error {
	// Required fields
	if bead.ID == "" {
		return fmt.Errorf("%s[%d]: id is required", section, index)
	}
	if bead.Title == "" {
		return fmt.Errorf("%s[%d]: title is required for bead %s", section, index, bead.ID)
	}

	// Tier constraints (1-4 based on research patterns)
	if bead.Tier < 1 || bead.Tier > 4 {
		return fmt.Errorf("%s[%d]: tier must be 1-4, got %d for bead %s", section, index, bead.Tier, bead.ID)
	}

	// Prompts validation
	if bead.Prompts.Start == "" {
		return fmt.Errorf("%s[%d]: prompts.start is required for bead %s", section, index, bead.ID)
	}

	return nil
}
