//go:build integration

package helpers

import (
	"os"
)

// ClaudeInterface abstracts Claude CLI operations for testing
type ClaudeInterface interface {
	// Start initiates Claude in the specified tmux session
	Start(sessionName string) error

	// IsReady checks if Claude is ready (process running, prompt visible)
	IsReady(sessionName string) bool

	// Stop terminates Claude in the session
	Stop(sessionName string) error
}

// ClaudeTestMode represents the test mode for Claude
type ClaudeTestMode string

const (
	ModeMock ClaudeTestMode = "mock"
	ModeReal ClaudeTestMode = "real"
)

// GetTestMode returns the current test mode from environment variable
func GetTestMode() ClaudeTestMode {
	mode := os.Getenv("AGM_TEST_CLAUDE_MODE")
	if mode == "real" {
		return ModeReal
	}
	return ModeMock // default to mock
}

// NewClaudeForTest returns appropriate Claude implementation based on AGM_TEST_CLAUDE_MODE
func NewClaudeForTest() ClaudeInterface {
	if GetTestMode() == ModeReal {
		// For now, real mode is not implemented - return mock
		// TODO: Implement RealClaude when needed
		return &MockClaude{}
	}
	return &MockClaude{}
}
