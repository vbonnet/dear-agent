//go:build !darwin && !linux

package tmux

import "fmt"

func readProcessArgv(pid int) ([]string, error) {
	return nil, fmt.Errorf("lossless process argv is unsupported for pid %d on this platform", pid)
}
