// Package main provides the devlog CLI entry point.
package main

import (
	"errors"
	"os"

	"github.com/vbonnet/dear-agent/pkg/cliframe"
	"github.com/vbonnet/dear-agent/tools/devlog/cmd/devlog"
)

func main() {
	if err := devlog.Execute(); err != nil {
		// Check if it's a cliframe CLIError
		var cliErr *cliframe.CLIError
		if errors.As(err, &cliErr) {
			// cliframe errors already have exit codes
			os.Exit(cliErr.ExitCode)
		}

		// For other errors, exit with general error code
		os.Exit(cliframe.ExitGeneralError)
	}
}
