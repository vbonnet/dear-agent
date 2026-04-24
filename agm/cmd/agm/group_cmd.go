package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// groupRunE returns a RunE function for parent/group commands that have no
// action of their own. Without this, cobra silently prints help and exits 0
// when an unknown subcommand is provided (e.g. "agm session send" when "send"
// is not a subcommand of "session"). This causes callers to believe the
// command succeeded when it actually did nothing.
//
// Behavior:
//   - No args: prints help (same as before, but still returns nil)
//   - With args: returns an error with "unknown command" message and exit code 1
func groupRunE(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	_ = cmd.Help()
	return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
}
