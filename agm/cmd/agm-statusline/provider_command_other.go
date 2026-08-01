//go:build !unix

package main

import (
	"os/exec"
	"time"
)

const providerCommandWaitDelay = 100 * time.Millisecond

// configureProviderCommand preserves portable builds outside the Unix release
// targets. CommandContext terminates the direct child, and WaitDelay bounds the
// compositor even if a descendant keeps inherited I/O pipes open. Unlike the
// Unix implementation, this fallback does not claim process-tree termination.
func configureProviderCommand(cmd *exec.Cmd) {
	cmd.WaitDelay = providerCommandWaitDelay
}
