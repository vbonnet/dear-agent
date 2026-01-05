package main

import (
	"fmt"
	"os"

	testerrors "github.com/vbonnet/ai-tools/claude-session-manager/internal/testutil/errors"
)

var Version = "dev"

func main() {
	if err := Execute(); err != nil {
		// Exit with appropriate code based on error type
		exitCode := testerrors.ExitCode(err)
		os.Exit(exitCode)
	}
}

// formatErrorForDisplay formats an error for CLI display
// This handles the case where Cobra returns errors that we want to format nicely
func formatErrorForDisplay(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("Error: %v\n", err)
}
