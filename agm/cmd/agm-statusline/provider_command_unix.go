//go:build unix

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const providerCommandWaitDelay = 100 * time.Millisecond

// configureProviderCommand isolates a provider process tree. A provider is an
// executable script, so killing only its direct shell can leave a descendant
// holding stdout open after the timeout. Cancel the dedicated process group
// instead and bound Wait in case the operating system cannot reap it promptly.
func configureProviderCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = providerCommandWaitDelay
}
