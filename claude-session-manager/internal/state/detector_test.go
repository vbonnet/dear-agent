package state

import (
	"strings"
	"testing"
	"time"
)

func TestDetector_DetectState_Ready(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		name   string
		output string
	}{
		{
			name:   "Claude prompt at end",
			output: "Previous output here\n❯ ",
		},
		{
			name:   "Claude prompt with trailing space",
			output: "Some text\n❯  ",
		},
		{
			name:   "Multi-line with prompt",
			output: "Line 1\nLine 2\nLine 3\n❯ ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectState(tt.output, time.Now())

			if result.State != StateReady {
				t.Errorf("Expected StateReady, got %v", result.State)
			}

			if result.Confidence != "high" {
				t.Errorf("Expected high confidence, got %s", result.Confidence)
			}
		})
	}
}

func TestDetector_DetectState_Thinking(t *testing.T) {
	detector := NewDetector()

	// All spinner characters
	spinners := []rune{'⣾', '⣽', '⣻', '⢿', '⡿', '⣟', '⣯', '⣷'}

	for _, spinner := range spinners {
		t.Run(string(spinner), func(t *testing.T) {
			output := "Processing your request " + string(spinner) + " please wait"
			result := detector.DetectState(output, time.Now())

			if result.State != StateThinking {
				t.Errorf("Expected StateThinking for spinner %c, got %v", spinner, result.State)
			}

			if result.Confidence != "high" {
				t.Errorf("Expected high confidence, got %s", result.Confidence)
			}

			if !strings.Contains(result.Evidence, string(spinner)) {
				t.Errorf("Expected evidence to contain spinner, got: %s", result.Evidence)
			}
		})
	}
}

func TestDetector_DetectState_BlockedAuth(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		name   string
		output string
	}{
		{
			name:   "Standard y/N prompt",
			output: "Do you want to continue? (y/N): ",
		},
		{
			name:   "Capital Y/n",
			output: "Approve this action? (Y/n): ",
		},
		{
			name:   "Reversed n/Y",
			output: "Confirm deletion? (n/Y): ",
		},
		{
			name:   "With context",
			output: "This will modify 5 files. Proceed? (y/N): ",
		},
		{
			name:   "Case insensitive",
			output: "Allow access to filesystem? (Y/N): ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectState(tt.output, time.Now())

			if result.State != StateBlockedAuth {
				t.Errorf("Expected StateBlockedAuth, got %v", result.State)
			}

			if result.Confidence != "high" {
				t.Errorf("Expected high confidence, got %s", result.Confidence)
			}
		})
	}
}

func TestDetector_DetectState_BlockedInput(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		name   string
		output string
	}{
		{
			name: "Numbered options",
			output: `Which approach should I use?
1. Option A
2. Option B
3. Option C`,
		},
		{
			name: "Lettered options",
			output: `Choose a color:
A. Red
B. Blue
C. Green`,
		},
		{
			name:   "Choose keyword",
			output: "Choose between the following options:",
		},
		{
			name:   "Select keyword",
			output: "Select your preferred method:",
		},
		{
			name:   "Which keyword",
			output: "Which option do you prefer:",
		},
		{
			name:   "Bracketed options",
			output: "Select mode: [Development] [Production] [Staging]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectState(tt.output, time.Now())

			if result.State != StateBlockedInput {
				t.Errorf("Expected StateBlockedInput, got %v", result.State)
			}

			if result.Confidence != "high" {
				t.Errorf("Expected high confidence, got %s", result.Confidence)
			}
		})
	}
}

func TestDetector_DetectState_Stuck(t *testing.T) {
	detector := NewDetector()
	detector.SetStuckThreshold(60 * time.Second)

	// Simulate last output 90 seconds ago
	lastOutputTime := time.Now().Add(-90 * time.Second)

	output := "Working on something..." // No prompt, no spinner

	result := detector.DetectState(output, lastOutputTime)

	if result.State != StateStuck {
		t.Errorf("Expected StateStuck, got %v", result.State)
	}

	if result.Confidence != "medium" {
		t.Errorf("Expected medium confidence, got %s", result.Confidence)
	}

	if !strings.Contains(result.Evidence, "No tokens") {
		t.Errorf("Expected evidence about no tokens, got: %s", result.Evidence)
	}
}

func TestDetector_DetectState_NotStuckWhenReady(t *testing.T) {
	detector := NewDetector()
	detector.SetStuckThreshold(60 * time.Second)

	// Even if last output was long ago, if we're at ready prompt, not stuck
	lastOutputTime := time.Now().Add(-120 * time.Second)
	output := "Previous work completed\n❯ "

	result := detector.DetectState(output, lastOutputTime)

	if result.State != StateReady {
		t.Errorf("Expected StateReady (not stuck), got %v", result.State)
	}
}

func TestDetector_DetectState_Unknown(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		name   string
		output string
	}{
		{
			name:   "Plain text",
			output: "Just some regular output",
		},
		{
			name:   "Partial prompt",
			output: "Here is the result",
		},
		{
			name:   "Empty output",
			output: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectState(tt.output, time.Now())

			if result.State != StateUnknown {
				t.Errorf("Expected StateUnknown, got %v", result.State)
			}

			if result.Confidence != "low" {
				t.Errorf("Expected low confidence, got %s", result.Confidence)
			}
		})
	}
}

func TestDetector_PriorityOrder(t *testing.T) {
	detector := NewDetector()

	// Blocked auth should take priority over thinking spinner
	output := "Processing ⣾ Do you want to continue? (y/N): "

	result := detector.DetectState(output, time.Now())

	if result.State != StateBlockedAuth {
		t.Errorf("Expected StateBlockedAuth (should win over thinking), got %v", result.State)
	}
}

func TestState_IsBlocked(t *testing.T) {
	tests := []struct {
		state    State
		expected bool
	}{
		{StateBlockedAuth, true},
		{StateBlockedInput, true},
		{StateReady, false},
		{StateThinking, false},
		{StateStuck, false},
		{StateUnknown, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if tt.state.IsBlocked() != tt.expected {
				t.Errorf("IsBlocked() for %s: expected %v, got %v",
					tt.state, tt.expected, tt.state.IsBlocked())
			}
		})
	}
}

func TestState_IsActive(t *testing.T) {
	tests := []struct {
		state    State
		expected bool
	}{
		{StateThinking, true},
		{StateReady, false},
		{StateBlockedAuth, false},
		{StateBlockedInput, false},
		{StateStuck, false},
		{StateUnknown, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if tt.state.IsActive() != tt.expected {
				t.Errorf("IsActive() for %s: expected %v, got %v",
					tt.state, tt.expected, tt.state.IsActive())
			}
		})
	}
}

func TestState_IsIdle(t *testing.T) {
	tests := []struct {
		state    State
		expected bool
	}{
		{StateReady, true},
		{StateThinking, false},
		{StateBlockedAuth, false},
		{StateBlockedInput, false},
		{StateStuck, false},
		{StateUnknown, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if tt.state.IsIdle() != tt.expected {
				t.Errorf("IsIdle() for %s: expected %v, got %v",
					tt.state, tt.expected, tt.state.IsIdle())
			}
		})
	}
}

func TestDetector_CustomStuckThreshold(t *testing.T) {
	detector := NewDetector()

	// Set custom threshold of 30 seconds
	detector.SetStuckThreshold(30 * time.Second)

	lastOutputTime := time.Now().Add(-45 * time.Second)
	output := "Working..."

	result := detector.DetectState(output, lastOutputTime)

	if result.State != StateStuck {
		t.Errorf("Expected StateStuck with 30s threshold, got %v", result.State)
	}
}

func TestDetector_extractEvidence(t *testing.T) {
	detector := NewDetector()

	output := "This is a long line of text with y/N prompt in the middle and more text after"
	pattern := detector.blockedAuthPattern

	evidence := detector.extractEvidence(output, pattern, 20)

	if !strings.Contains(evidence, "y/N") {
		t.Errorf("Evidence should contain matched pattern, got: %s", evidence)
	}

	// Evidence should be truncated to context length
	if len(evidence) > 100 { // Pattern + 2*20 chars context + some buffer
		t.Errorf("Evidence too long: %d chars", len(evidence))
	}
}
