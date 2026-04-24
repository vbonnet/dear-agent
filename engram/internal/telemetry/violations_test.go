package telemetry

import (
	"testing"
	"time"
)

func TestValidateViolationEvent_Valid(t *testing.T) {
	event := ViolationEvent{
		ID:              "test-id",
		Timestamp:       time.Now(),
		InstructionType: "tool_usage",
		InstructionRule: "never_use_cd_and",
		ViolationType:   "bash_command_pattern",
		Confidence:      ConfidenceHigh,
		Agent:           "claude-code",
		Context:         "cd /path && ls",
		DetectionMethod: DetectionExternal,
	}

	err := validateViolationEvent(event)
	if err != nil {
		t.Errorf("validateViolationEvent() error = %v, want nil", err)
	}
}

func TestValidateViolationEvent_MissingID(t *testing.T) {
	event := ViolationEvent{
		Timestamp:       time.Now(),
		InstructionType: "tool_usage",
		InstructionRule: "never_use_cd_and",
		ViolationType:   "bash_command_pattern",
		Confidence:      ConfidenceHigh,
		Agent:           "claude-code",
		DetectionMethod: DetectionExternal,
	}

	err := validateViolationEvent(event)
	if err == nil {
		t.Error("validateViolationEvent() expected error for missing ID, got nil")
	}
}

func TestValidateViolationEvent_InvalidConfidence(t *testing.T) {
	event := ViolationEvent{
		ID:              "test-id",
		Timestamp:       time.Now(),
		InstructionType: "tool_usage",
		InstructionRule: "never_use_cd_and",
		ViolationType:   "bash_command_pattern",
		Confidence:      Confidence("INVALID"),
		Agent:           "claude-code",
		DetectionMethod: DetectionExternal,
	}

	err := validateViolationEvent(event)
	if err == nil {
		t.Error("validateViolationEvent() expected error for invalid confidence, got nil")
	}
}

func TestValidateViolationEvent_InvalidDetectionMethod(t *testing.T) {
	event := ViolationEvent{
		ID:              "test-id",
		Timestamp:       time.Now(),
		InstructionType: "tool_usage",
		InstructionRule: "never_use_cd_and",
		ViolationType:   "bash_command_pattern",
		Confidence:      ConfidenceHigh,
		Agent:           "claude-code",
		DetectionMethod: DetectionMethod("invalid"),
	}

	err := validateViolationEvent(event)
	if err == nil {
		t.Error("validateViolationEvent() expected error for invalid detection_method, got nil")
	}
}
