//go:build windows

package specguard

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func configureProcessGroup(_ *exec.Cmd) {}

func killProcessGroup(_ *os.Process) error {
	return errors.New("descendant-process termination is unsupported on Windows")
}

func waitForGitCommandExitWithoutReaping(pid int) error {
	return fmt.Errorf("wait without reaping is unsupported for Git child pid %d on Windows", pid)
}

func gitProcessGroupEPERMComplete(_ int, _ bool) (bool, error) {
	return false, nil
}
