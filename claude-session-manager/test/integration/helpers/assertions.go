//go:build integration

package helpers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AssertCommandOrder verifies commands appear in expected order in output
func AssertCommandOrder(t *testing.T, output string, expectedOrder []string) {
	indices := make([]int, len(expectedOrder))

	for i, cmd := range expectedOrder {
		idx := strings.Index(output, cmd)
		require.Greater(t, idx, -1, "Expected command '%s' in output", cmd)
		indices[i] = idx
	}

	// Verify order
	for i := 1; i < len(indices); i++ {
		require.Less(t, indices[i-1], indices[i],
			"Expected '%s' before '%s'",
			expectedOrder[i-1], expectedOrder[i])
	}
}

// AssertErrorMessageQuality verifies error message contains required elements
func AssertErrorMessageQuality(t *testing.T, err error, requiredElements []string) {
	require.Error(t, err, "Expected error to be present")

	errorMsg := err.Error()

	for _, element := range requiredElements {
		assert.Contains(t, errorMsg, element,
			"Error message should contain '%s' for helpful guidance", element)
	}
}

// AssertSendSafetyOrder verifies wait-then-send order
func AssertSendSafetyOrder(t *testing.T, mockPrompt *MockPromptDetector, mockSender *MockCommandSender) {
	// Verify wait-then-send order
	assert.True(t, mockPrompt.WaitCalled,
		"Expected WaitForPromptSimple called before send")

	if mockPrompt.PromptReady {
		assert.NotEmpty(t, mockSender.CommandsSent,
			"Expected command sent after prompt detected")
	} else {
		assert.Empty(t, mockSender.CommandsSent,
			"Expected no command sent when prompt timeout")
	}
}

// AssertLiteralModeUsed verifies literal mode prevents special char interpretation
func AssertLiteralModeUsed(t *testing.T, mockSender *MockCommandSender, prompt string) {
	assert.True(t, mockSender.UsedLiteralMode,
		"Expected literal mode (send-keys -l) to prevent special char interpretation")

	// Verify no shell expansion
	if len(mockSender.CommandsSent) > 0 {
		assert.Equal(t, prompt, mockSender.CommandsSent[0],
			"Expected exact text sent without variable expansion")
	}
}
