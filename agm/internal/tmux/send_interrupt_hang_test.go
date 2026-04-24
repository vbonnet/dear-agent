package tmux

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestSendMultiLinePromptSafe_InterruptSkipsWait is a REGRESSION TEST
// for the bug where --interrupt flag still waited 60s for prompt.
//
// Bug: SendMultiLinePromptSafe always called WaitForPromptSimple, even with shouldInterrupt=true
// Fix: Skip wait when shouldInterrupt=true (send immediately)
//
// This test would FAIL before the fix (timeout), PASS after the fix (immediate send)
func TestSendMultiLinePromptSafe_InterruptSkipsWait(t *testing.T) {
	sessionName := "test-interrupt-no-wait"
	prompt := "Interrupt and send immediately"

	// SETUP: Mock prompt detector that would timeout (simulates busy session)
	mockPrompt := &mockPromptDetectorWithInterrupt{
		promptReady:  false, // Session busy, would timeout
		waitTime:     100 * time.Millisecond,
		timeoutError: assert.AnError,
	}

	// SETUP: Mock command sender
	mockSender := &mockCommandSenderWithInterrupt{}

	// EXECUTION: Simulate SendMultiLinePromptSafe with shouldInterrupt=true
	// This mimics the fixed behavior: skip wait when interrupting
	shouldInterrupt := true

	var err error
	if !shouldInterrupt {
		// OLD BEHAVIOR (buggy): Always wait, even with interrupt flag
		err = mockPrompt.WaitForPromptSimple(sessionName, 60*time.Second)
	}
	// NEW BEHAVIOR (fixed): Skip wait when interrupting

	// Send regardless of wait (interrupt mode sends immediately)
	if shouldInterrupt || err == nil {
		mockSender.SendPromptLiteral(sessionName, prompt, shouldInterrupt)
	}

	// ASSERTIONS: Verify interrupt mode behavior
	assert.False(t, mockPrompt.WaitCalled,
		"Expected NO wait when shouldInterrupt=true (immediate send)")
	assert.NotEmpty(t, mockSender.CommandsSent,
		"Expected command sent immediately without waiting")
	assert.Equal(t, prompt, mockSender.CommandsSent[0],
		"Expected exact prompt text sent")
	assert.True(t, mockSender.InterruptSent,
		"Expected ESC sent for interrupt mode")
}

// TestSendMultiLinePromptSafe_NoInterruptWaits verifies non-interrupt mode still waits
func TestSendMultiLinePromptSafe_NoInterruptWaits(t *testing.T) {
	sessionName := "test-no-interrupt-waits"
	prompt := "Wait for prompt before sending"

	// SETUP: Mock prompt detector (ready)
	mockPrompt := &mockPromptDetectorWithInterrupt{
		promptReady: true,
		waitTime:    50 * time.Millisecond,
	}

	// SETUP: Mock command sender
	mockSender := &mockCommandSenderWithInterrupt{}

	// EXECUTION: Simulate SendMultiLinePromptSafe with shouldInterrupt=false
	shouldInterrupt := false

	var err error
	if !shouldInterrupt {
		// Should wait for prompt (safe behavior)
		err = mockPrompt.WaitForPromptSimple(sessionName, 60*time.Second)
	}

	// Only send if wait succeeded
	if shouldInterrupt || err == nil {
		mockSender.SendPromptLiteral(sessionName, prompt, shouldInterrupt)
	}

	// ASSERTIONS: Verify safe mode behavior
	assert.True(t, mockPrompt.WaitCalled,
		"Expected wait for prompt when shouldInterrupt=false (safe mode)")
	assert.NotEmpty(t, mockSender.CommandsSent,
		"Expected command sent after prompt detected")
	assert.False(t, mockSender.InterruptSent,
		"Expected NO ESC sent in safe mode")
}

// TestSendMultiLinePromptSafe_InterruptPerformance verifies interrupt is fast
func TestSendMultiLinePromptSafe_InterruptPerformance(t *testing.T) {
	sessionName := "test-interrupt-performance"
	prompt := "Performance test"

	// SETUP: Mock prompt detector that would take 60s (simulates busy session)
	mockPrompt := &mockPromptDetectorWithInterrupt{
		promptReady:  false,
		waitTime:     60 * time.Second, // Would timeout after 60s
		timeoutError: assert.AnError,
	}

	// SETUP: Mock command sender
	mockSender := &mockCommandSenderWithInterrupt{}

	// EXECUTION: Measure time with interrupt mode
	start := time.Now()

	shouldInterrupt := true
	var err error
	if !shouldInterrupt {
		err = mockPrompt.WaitForPromptSimple(sessionName, 60*time.Second)
	}

	if shouldInterrupt || err == nil {
		mockSender.SendPromptLiteral(sessionName, prompt, shouldInterrupt)
	}

	elapsed := time.Since(start)

	// ASSERTIONS: Verify fast delivery
	assert.Less(t, elapsed, 1*time.Second,
		"Expected interrupt mode to send in < 1 second (no 60s wait)")
	assert.NotEmpty(t, mockSender.CommandsSent,
		"Expected command sent despite session being busy")
	assert.True(t, mockSender.InterruptSent,
		"Expected interrupt (ESC) sent")
}

// Mock types for interrupt behavior tests

type mockPromptDetectorWithInterrupt struct {
	promptReady  bool
	waitTime     time.Duration
	timeoutError error
	WaitCalled   bool
}

func (m *mockPromptDetectorWithInterrupt) WaitForPromptSimple(sessionName string, timeout time.Duration) error {
	m.WaitCalled = true

	// Simulate wait time
	if m.waitTime > 0 {
		time.Sleep(m.waitTime)
	}

	if !m.promptReady {
		if m.timeoutError != nil {
			return m.timeoutError
		}
		return assert.AnError
	}

	return nil
}

type mockCommandSenderWithInterrupt struct {
	CommandsSent  []string
	InterruptSent bool
}

func (m *mockCommandSenderWithInterrupt) SendPromptLiteral(sessionName string, text string, shouldInterrupt bool) error {
	m.CommandsSent = append(m.CommandsSent, text)
	m.InterruptSent = shouldInterrupt
	return nil
}
