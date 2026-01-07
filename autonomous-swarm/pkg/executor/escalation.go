package executor

import (
	"fmt"
	"strings"
)

// EscalationDetector detects escalation signals in session output
type EscalationDetector struct {
	keyword string
}

// NewEscalationDetector creates a new escalation detector
func NewEscalationDetector() *EscalationDetector {
	return &EscalationDetector{
		keyword: "ESCALATE:",
	}
}

// DetectEscalation scans session output for escalation keyword
func (d *EscalationDetector) DetectEscalation(sessionOutput string) (bool, string) {
	if !strings.Contains(sessionOutput, d.keyword) {
		return false, ""
	}

	// Extract escalation reason (text after keyword)
	lines := strings.Split(sessionOutput, "\n")
	for _, line := range lines {
		if idx := strings.Index(line, d.keyword); idx != -1 {
			reason := strings.TrimSpace(line[idx+len(d.keyword):])
			if reason == "" {
				reason = "unspecified reason"
			}
			return true, reason
		}
	}

	return true, "unspecified reason"
}

// CreateEscalationError creates an escalation error from detected signal
func (d *EscalationDetector) CreateEscalationError(beadID string, iteration int, reason string) *ExecutionError {
	return NewEscalationError(
		beadID,
		iteration,
		fmt.Sprintf("explicit escalation requested: %s", reason),
		nil,
	)
}

// IsEscalationError checks if an error is an escalation error
func IsEscalationError(err error) bool {
	if err == nil {
		return false
	}

	if execErr, ok := err.(*ExecutionError); ok {
		return execErr.Type == ErrorEscalation
	}

	return false
}
