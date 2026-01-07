package executor

import (
	"testing"
)

func TestNewEscalationDetector(t *testing.T) {
	detector := NewEscalationDetector()

	if detector == nil {
		t.Fatal("NewEscalationDetector should not return nil")
	}

	if detector.keyword != "ESCALATE:" {
		t.Errorf("keyword: got %q, want %q", detector.keyword, "ESCALATE:")
	}
}

func TestDetectEscalation(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		wantDetected bool
		wantReason   string
	}{
		{
			name:         "no escalation",
			output:       "Normal session output\nNo issues found\n",
			wantDetected: false,
			wantReason:   "",
		},
		{
			name:         "explicit escalation with reason",
			output:       "Task failed\nESCALATE: requires manual intervention\nEnd of output",
			wantDetected: true,
			wantReason:   "requires manual intervention",
		},
		{
			name:         "escalation without reason",
			output:       "ESCALATE:\n",
			wantDetected: true,
			wantReason:   "unspecified reason",
		},
		{
			name:         "escalation with whitespace",
			output:       "ESCALATE:   needs review  \n",
			wantDetected: true,
			wantReason:   "needs review",
		},
		{
			name:         "multiple lines with escalation",
			output:       "Line 1\nLine 2\nESCALATE: blocked on dependency\nLine 4\n",
			wantDetected: true,
			wantReason:   "blocked on dependency",
		},
		{
			name:         "escalate in middle of line",
			output:       "Error: ESCALATE: configuration error",
			wantDetected: true,
			wantReason:   "configuration error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := NewEscalationDetector()
			detected, reason := detector.DetectEscalation(tt.output)

			if detected != tt.wantDetected {
				t.Errorf("detected: got %v, want %v", detected, tt.wantDetected)
			}

			if tt.wantDetected && reason != tt.wantReason {
				t.Errorf("reason: got %q, want %q", reason, tt.wantReason)
			}
		})
	}
}

func TestCreateEscalationError(t *testing.T) {
	detector := NewEscalationDetector()
	err := detector.CreateEscalationError("bead-1", 2, "test reason")

	if err == nil {
		t.Fatal("CreateEscalationError should not return nil")
	}

	if err.Type != ErrorEscalation {
		t.Errorf("error type: got %v, want %v", err.Type, ErrorEscalation)
	}

	if err.BeadID != "bead-1" {
		t.Errorf("bead ID: got %q, want %q", err.BeadID, "bead-1")
	}

	if err.Iteration != 2 {
		t.Errorf("iteration: got %d, want 2", err.Iteration)
	}

	expectedMsg := "explicit escalation requested: test reason"
	if err.Message != expectedMsg {
		t.Errorf("message: got %q, want %q", err.Message, expectedMsg)
	}
}
