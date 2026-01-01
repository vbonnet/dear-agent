package main

import (
	"fmt"
	"os"

	"github.com/vbonnet/ai-tools/devlog/cmd/devlog"
	deverrors "github.com/vbonnet/ai-tools/devlog/internal/errors"
)

func main() {
	if err := devlog.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)

		// Use appropriate exit code based on error type
		var devErr *deverrors.DevlogError
		if deverrors.As(err, &devErr) {
			if deverrors.Is(err, deverrors.ErrConfigNotFound) ||
				deverrors.Is(err, deverrors.ErrConfigInvalid) {
				os.Exit(deverrors.ExitConfigError)
			}
			if deverrors.Is(err, deverrors.ErrGitFailed) {
				os.Exit(deverrors.ExitGitError)
			}
		}
		os.Exit(deverrors.ExitGeneralError)
	}
}
