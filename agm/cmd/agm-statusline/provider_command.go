package main

import (
	"os/exec"
	"time"
)

const providerCommandWaitDelay = 100 * time.Millisecond

// configureProviderCommand bounds cancellation cleanup. CommandContext kills
// the direct provider process at its deadline; WaitDelay prevents descendants
// that retain inherited I/O pipes from keeping the compositor blocked. It does
// not claim to terminate detached or background descendants.
func configureProviderCommand(cmd *exec.Cmd) {
	cmd.WaitDelay = providerCommandWaitDelay
}
