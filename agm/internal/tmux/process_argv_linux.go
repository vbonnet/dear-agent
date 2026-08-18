//go:build linux

package tmux

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

func readProcessArgv(pid int) ([]string, error) {
	path := filepath.Join("/proc", strconv.Itoa(pid), "cmdline")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(raw) == 0 || raw[len(raw)-1] != 0 {
		return nil, fmt.Errorf("read %s: missing argv terminator", path)
	}
	fields := bytes.Split(raw[:len(raw)-1], []byte{0})
	if len(fields) == 0 || len(fields[0]) == 0 {
		return nil, fmt.Errorf("read %s: empty argv", path)
	}
	args := make([]string, len(fields))
	for index, field := range fields {
		args[index] = string(field)
	}
	return args, nil
}
