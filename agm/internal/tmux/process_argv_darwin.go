//go:build darwin

package tmux

import (
	"encoding/binary"
	"fmt"

	"golang.org/x/sys/unix"
)

func readProcessArgv(pid int) ([]string, error) {
	raw, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return nil, fmt.Errorf("kern.procargs2 %d: %w", pid, err)
	}
	if len(raw) < 4 {
		return nil, fmt.Errorf("kern.procargs2 %d: short response", pid)
	}
	argc := int(binary.NativeEndian.Uint32(raw[:4]))
	if argc <= 0 || argc > 1<<20 {
		return nil, fmt.Errorf("kern.procargs2 %d: invalid argc %d", pid, argc)
	}
	position := 4
	for position < len(raw) && raw[position] != 0 {
		position++
	}
	for position < len(raw) && raw[position] == 0 {
		position++
	}
	args := make([]string, 0, argc)
	for len(args) < argc && position < len(raw) {
		end := position
		for end < len(raw) && raw[end] != 0 {
			end++
		}
		if end == len(raw) {
			return nil, fmt.Errorf("kern.procargs2 %d: unterminated argv entry", pid)
		}
		args = append(args, string(raw[position:end]))
		position = end + 1
	}
	if len(args) != argc {
		return nil, fmt.Errorf("kern.procargs2 %d: got %d argv entries, want %d", pid, len(args), argc)
	}
	return args, nil
}
