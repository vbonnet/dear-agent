package tmux

import (
	"testing"
)

// Note: WaitForPaneClose, SendKeysToPane, and IsPaneActive require a running
// tmux session to test properly. These would be tested in integration tests
// rather than unit tests. Here we just verify the functions exist and have
// correct signatures.

func TestPaneMonitorFunctionsExist(t *testing.T) {
	// This is a compile-time check - if these functions don't exist
	// or have the wrong signature, this won't compile

	var _ func(string, string) error = SendKeysToPane
	var _ func(string) (bool, error) = IsPaneActive
}
