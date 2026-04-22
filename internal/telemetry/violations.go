package telemetry

import (
	"encoding/json"
	"fmt"
)

// ReportViolation records a violation event (self-reported or external)
func (c *Collector) ReportViolation(violation ViolationEvent) error {
	if !c.enabled {
		return nil // Telemetry disabled
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Validate violation event
	if err := validateViolationEvent(violation); err != nil {
		return fmt.Errorf("invalid violation event: %w", err)
	}

	// Write to JSONL first (source of truth)
	if err := c.writeViolationJSONL(violation); err != nil {
		return fmt.Errorf("JSONL write failed: %w", err)
	}

	// TODO: Add SQLite dual-write in future iteration
	// For V1, JSONL-only storage is sufficient for observability

	return nil
}

// writeViolationJSONL appends violation to JSONL log
func (c *Collector) writeViolationJSONL(v ViolationEvent) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal violation: %w", err)
	}

	if _, err := c.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write to JSONL: %w", err)
	}

	return nil
}

// validateViolationEvent checks required fields and value constraints
func validateViolationEvent(v ViolationEvent) error {
	if v.ID == "" {
		return fmt.Errorf("missing required field: id")
	}
	if v.Timestamp.IsZero() {
		return fmt.Errorf("missing required field: timestamp")
	}
	if v.InstructionType == "" {
		return fmt.Errorf("missing required field: instruction_type")
	}
	if v.InstructionRule == "" {
		return fmt.Errorf("missing required field: instruction_rule")
	}
	if v.ViolationType == "" {
		return fmt.Errorf("missing required field: violation_type")
	}
	if v.Agent == "" {
		return fmt.Errorf("missing required field: agent")
	}

	// Validate confidence enum
	switch v.Confidence {
	case ConfidenceHigh, ConfidenceMedium, ConfidenceLow:
		// valid
	default:
		return fmt.Errorf("invalid confidence: %q (must be HIGH, MEDIUM, or LOW)", v.Confidence)
	}

	// Validate detection method enum
	switch v.DetectionMethod {
	case DetectionExternal, DetectionSelfReported:
		// valid
	default:
		return fmt.Errorf("invalid detection_method: %q (must be external or self_reported)", v.DetectionMethod)
	}

	return nil
}
