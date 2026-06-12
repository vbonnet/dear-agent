package commands_test

import (
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-features/commands"
)

func TestCommandsExported(t *testing.T) {
	// Verify that the cobra commands are registered as exported variables.
	if commands.StatusCmd == nil {
		t.Error("StatusCmd should not be nil")
	}
	if commands.StartCmd == nil {
		t.Error("StartCmd should not be nil")
	}
	if commands.VerifyCmd == nil {
		t.Error("VerifyCmd should not be nil")
	}
}
