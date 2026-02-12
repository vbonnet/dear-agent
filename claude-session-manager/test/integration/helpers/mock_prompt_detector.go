//go:build integration

package helpers

import (
	"fmt"
	"time"
)

// MockPromptDetector simulates prompt detection for testing
type MockPromptDetector struct {
	PromptReady  bool
	WaitCalled   bool
	WaitTime     time.Duration
	TimeoutError error
}

// WaitForPromptSimple simulates waiting for Claude prompt
func (m *MockPromptDetector) WaitForPromptSimple(sessionName string, timeout time.Duration) error {
	m.WaitCalled = true

	// Simulate wait time
	if m.WaitTime > 0 {
		time.Sleep(m.WaitTime)
	}

	if !m.PromptReady {
		if m.TimeoutError != nil {
			return m.TimeoutError
		}
		return fmt.Errorf("Timeout waiting for prompt")
	}

	return nil
}

// Reset clears mock state
func (m *MockPromptDetector) Reset() {
	m.PromptReady = false
	m.WaitCalled = false
	m.WaitTime = 0
	m.TimeoutError = nil
}
