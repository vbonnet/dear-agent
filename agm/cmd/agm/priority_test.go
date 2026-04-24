package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/messages"
)

// TestPriorityInstructionsMap_Complete verifies all 5 priority levels are defined
func TestPriorityInstructionsMap_Complete(t *testing.T) {
	expectedKeys := []string{"fyi", "background", "normal", "urgent", "critical"}

	assert.Len(t, priorityInstructions, 5, "should have exactly 5 priority levels")

	for _, key := range expectedKeys {
		_, ok := priorityInstructions[key]
		assert.True(t, ok, "priority %q should be in priorityInstructions", key)
	}

	// Normal priority should have empty instruction (no header injected)
	assert.Empty(t, priorityInstructions["normal"], "normal priority should have empty instruction")

	// Non-normal priorities should have non-empty instructions
	for _, key := range []string{"fyi", "background", "urgent", "critical"} {
		assert.NotEmpty(t, priorityInstructions[key], "priority %q should have a non-empty instruction", key)
	}
}

// TestPriorityToQueuePriority_Mapping verifies correct mapping to queue constants
func TestPriorityToQueuePriority_Mapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"critical", messages.PriorityCritical},
		{"urgent", messages.PriorityHigh},
		{"normal", messages.PriorityMedium},
		{"background", messages.PriorityLow},
		{"fyi", messages.PriorityLow},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			actual, ok := priorityToQueuePriority[tt.input]
			require.True(t, ok, "priority %q should have a queue mapping", tt.input)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

// TestPriorityToQueuePriority_AllCovered ensures every priority in instructions has a queue mapping
func TestPriorityToQueuePriority_AllCovered(t *testing.T) {
	for key := range priorityInstructions {
		_, ok := priorityToQueuePriority[key]
		assert.True(t, ok, "priority %q is in priorityInstructions but missing from priorityToQueuePriority", key)
	}
}

// TestPriorityValidation_ValidValues verifies all valid priority strings are accepted
func TestPriorityValidation_ValidValues(t *testing.T) {
	validPriorities := []string{"fyi", "background", "normal", "urgent", "critical"}

	for _, p := range validPriorities {
		t.Run(p, func(t *testing.T) {
			_, ok := priorityInstructions[p]
			assert.True(t, ok, "priority %q should be accepted as valid", p)
		})
	}
}

// TestPriorityValidation_InvalidValues verifies invalid priority strings are rejected
func TestPriorityValidation_InvalidValues(t *testing.T) {
	invalidPriorities := []string{
		"high",      // wrong case / not a valid level
		"low",       // wrong case / not a valid level
		"CRITICAL",  // uppercase not accepted
		"URGENT",    // uppercase not accepted
		"medium",    // internal queue constant, not user-facing
		"",          // empty string
		"emergency", // not a valid level
		"HIGH",      // internal queue constant
		"info",      // not a valid level
	}

	for _, p := range invalidPriorities {
		t.Run("reject_"+p, func(t *testing.T) {
			_, ok := priorityInstructions[p]
			assert.False(t, ok, "priority %q should be rejected as invalid", p)
		})
	}
}

// TestFormatMessageWithPriority_Normal verifies normal priority does not add priority header
func TestFormatMessageWithPriority_Normal(t *testing.T) {
	oldPriority := sessionSendPriority
	defer func() { sessionSendPriority = oldPriority }()
	sessionSendPriority = "normal"

	result := formatMessageWithMetadata("sender", "id-001", "", "test message")

	assert.NotContains(t, result, "[Priority:")
	assert.Contains(t, result, "test message")
	assert.Contains(t, result, "[From: sender")
}

// TestFormatMessageWithPriority_Critical verifies critical priority adds correct header
func TestFormatMessageWithPriority_Critical(t *testing.T) {
	oldPriority := sessionSendPriority
	defer func() { sessionSendPriority = oldPriority }()
	sessionSendPriority = "critical"

	result := formatMessageWithMetadata("sender", "id-001", "", "urgent task")

	assert.Contains(t, result, "[Priority: critical]")
	assert.Contains(t, result, "DROP everything")
	assert.Contains(t, result, "urgent task")
}

// TestFormatMessageWithPriority_AllNonNormal verifies all non-normal priorities inject instructions
func TestFormatMessageWithPriority_AllNonNormal(t *testing.T) {
	nonNormal := []string{"fyi", "background", "urgent", "critical"}

	for _, p := range nonNormal {
		t.Run(p, func(t *testing.T) {
			oldPriority := sessionSendPriority
			defer func() { sessionSendPriority = oldPriority }()
			sessionSendPriority = p

			result := formatMessageWithMetadata("sender", "id-001", "", "hello")

			assert.Contains(t, result, "[Priority: "+p+"]")

			instruction := priorityInstructions[p]
			assert.True(t, strings.Contains(result, instruction),
				"result should contain instruction %q for priority %q", instruction, p)
		})
	}
}
