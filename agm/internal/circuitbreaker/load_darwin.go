//go:build darwin

package circuitbreaker

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// SysctlLoadReader reads the 5-minute load average on macOS via
// `sysctl -n vm.loadavg`. macOS has no /proc/loadavg, so the Linux
// ProcLoadReader errors there and the CPU-load gate silently fails open;
// this reader makes the gate actually fire on macOS.
type SysctlLoadReader struct{}

// Load5 returns the 5-minute load average from `sysctl -n vm.loadavg`, whose
// output looks like "{ 1.23 4.56 7.89 }".
func (SysctlLoadReader) Load5() (float64, error) {
	out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output()
	if err != nil {
		return 0, fmt.Errorf("sysctl vm.loadavg: %w", err)
	}
	s := strings.Trim(strings.TrimSpace(string(out)), "{}")
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return 0, fmt.Errorf("unexpected sysctl vm.loadavg output: %q", string(out))
	}
	return strconv.ParseFloat(fields[1], 64)
}

// DefaultLoadReader returns the platform-native load reader (sysctl on macOS).
func DefaultLoadReader() LoadReader {
	return SysctlLoadReader{}
}
