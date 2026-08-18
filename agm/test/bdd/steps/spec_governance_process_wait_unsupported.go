//go:build !darwin && !linux

package steps

import "fmt"

func waitForSpecAuditCommandExitWithoutReaping(pid int) error {
	return fmt.Errorf("wait without reaping is unsupported for pid %d on this platform", pid)
}

func specAuditProcessGroupEPERMComplete(_ int, _ bool) (bool, error) {
	return false, nil
}
