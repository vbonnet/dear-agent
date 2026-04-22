//go:build integration

package helpers

import (
	"fmt"
	"time"
)

// MockClaudeReadyFile simulates ready-file wait for testing
type MockClaudeReadyFile struct {
	Timeout     time.Duration
	WillTimeout bool
	ReadyFile   string
}

// WaitForReady simulates waiting for Claude ready signal
func (m *MockClaudeReadyFile) WaitForReady(timeout time.Duration, progressFunc func()) error {
	if m.WillTimeout {
		// Simulate timeout
		time.Sleep(m.Timeout)
		return fmt.Errorf(`Failed to detect Claude ready signal

SessionStart hook may not be configured.
Please see docs/HOOKS-SETUP.md for setup instructions.

Quick setup:
  1. Copy hook: cp docs/hooks/session-start-agm.sh ~/.config/claude/hooks/
  2. Make executable: chmod +x ~/.config/claude/hooks/session-start-agm.sh
  3. Add to ~/.config/claude/config.yaml:
     hooks:
       SessionStart:
         - name: agm-ready-signal
           command: ~/.config/claude/hooks/session-start-agm.sh`)
	}

	// Simulate ready-file appearing
	return nil
}

// Cleanup removes test ready-files
func (m *MockClaudeReadyFile) Cleanup() error {
	return nil
}

// PendingPath returns mock pending-file path
func (m *MockClaudeReadyFile) PendingPath() string {
	return fmt.Sprintf("/tmp/test-pending-%s", m.ReadyFile)
}

// ReadyPath returns mock ready-file path
func (m *MockClaudeReadyFile) ReadyPath() string {
	return fmt.Sprintf("/tmp/test-ready-%s", m.ReadyFile)
}
