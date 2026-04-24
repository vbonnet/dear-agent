//go:build integration

package tmux

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/test/integration/helpers"
)

func TestHookTimeoutErrorMessage(t *testing.T) {
	// SETUP
	sessionName := "test-hook-timeout"

	// Mock ClaudeReadyFile with short timeout
	claudeReady := &helpers.MockClaudeReadyFile{
		Timeout:     1 * time.Second, // Short timeout for test speed
		WillTimeout: true,
		ReadyFile:   sessionName,
	}

	// EXECUTION
	err := claudeReady.WaitForReady(1*time.Second, nil)

	// ASSERTIONS
	require.Error(t, err, "Expected timeout error when hook not configured")

	// Assert error contains helpful guidance
	helpers.AssertErrorMessageQuality(t, err, []string{
		"SessionStart hook may not be configured",
		"docs/HOOKS-SETUP.md",
		"chmod +x",
		"config.yaml",
		"session-start-agm.sh",
	})

	// TEARDOWN (no cleanup needed for mock)
}
