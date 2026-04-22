package enforcement

import (
	"strings"
	"testing"
)

func TestGenerateRejectionMessage(t *testing.T) {
	p := &Pattern{
		ID:           "cd-command",
		Reason:       "Using cd command",
		Alternative:  "Use -C flag",
		Tier1Example: "BAD: cd /repo && git push\nGOOD: git -C /repo push",
	}

	msg := GenerateRejectionMessage(p, "cd /repo")
	if !strings.Contains(msg, "cd-command") {
		t.Error("message should contain pattern ID")
	}
	if !strings.Contains(msg, "cd /repo") {
		t.Error("message should contain command")
	}
	if !strings.Contains(msg, "Using cd command") {
		t.Error("message should contain reason")
	}
	if !strings.Contains(msg, "Use -C flag") {
		t.Error("message should contain alternative")
	}
	if !strings.Contains(msg, "BAD:") {
		t.Error("message should contain tier1 example")
	}
}

func TestGenerateRejectionMessageWithExamples(t *testing.T) {
	p := &Pattern{
		ID:          "test",
		Reason:      "test reason",
		Alternative: "test alt",
		Examples:    []string{"example1", "example2"},
	}

	msg := GenerateRejectionMessage(p, "")
	if !strings.Contains(msg, "example1") {
		t.Error("message should contain examples when no tier1_example")
	}
	if strings.Contains(msg, "Command:") {
		t.Error("message should not contain Command: when empty")
	}
}

func TestGenerateShortRejectionMessage(t *testing.T) {
	p := &Pattern{ID: "cd-command", Reason: "Using cd command"}
	msg := GenerateShortRejectionMessage(p)
	if msg != "[cd-command] Using cd command" {
		t.Errorf("unexpected short message: %q", msg)
	}
}

func TestGenerateRejectionMessageWithSeverity(t *testing.T) {
	tests := []struct {
		severity string
		contains string
	}{
		{"critical", "reliability"},
		{"high", "flagged for review"},
		{"medium", "alternative approach"},
	}

	for _, tt := range tests {
		p := &Pattern{
			ID:          "test",
			Reason:      "reason",
			Alternative: "alt",
			Severity:    tt.severity,
		}
		msg := GenerateRejectionMessageWithSeverity(p, "cmd")
		if !strings.Contains(msg, tt.contains) {
			t.Errorf("severity=%q: expected message to contain %q", tt.severity, tt.contains)
		}
		if !strings.Contains(msg, strings.ToUpper(tt.severity)) {
			t.Errorf("severity=%q: expected message to contain severity in header", tt.severity)
		}
	}
}

func TestFormatHookDenial(t *testing.T) {
	p := &Pattern{
		ID:          "cd-command",
		PatternName: "cd command",
		Remediation: "Use absolute paths",
		Alternative: "Use -C flag",
	}

	name, remediation := FormatHookDenial(p)
	if name != "cd command" {
		t.Errorf("expected name 'cd command', got %q", name)
	}
	if remediation != "Use absolute paths" {
		t.Errorf("expected remediation 'Use absolute paths', got %q", remediation)
	}

	// Fallback to ID and Alternative
	p2 := &Pattern{ID: "test-id", Alternative: "test alt"}
	name, remediation = FormatHookDenial(p2)
	if name != "test-id" {
		t.Errorf("expected name 'test-id', got %q", name)
	}
	if remediation != "test alt" {
		t.Errorf("expected remediation 'test alt', got %q", remediation)
	}
}
