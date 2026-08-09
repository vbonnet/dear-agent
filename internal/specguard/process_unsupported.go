//go:build !darwin && !linux && !windows

package specguard

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func configureProcessGroup(_ *exec.Cmd) {}

func killProcessGroup(_ *os.Process) error {
	return errors.New("descendant-process termination is unsupported on this platform")
}

func waitForGitCommandExitWithoutReaping(pid int) error {
	return fmt.Errorf("wait without reaping is unsupported for Git child pid %d", pid)
}

func gitProcessGroupEPERMComplete(_ int, _, _ bool) (bool, error) {
	return false, nil
}
