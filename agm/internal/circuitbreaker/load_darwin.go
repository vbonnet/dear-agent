//go:build darwin

package circuitbreaker

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// sysctlTimeout bounds the `sysctl` subprocess. It matters more now that the
// CPU-load gate fails closed: without a deadline a wedged sysctl would hang the
// spawn indefinitely rather than refuse it, which is worse than either outcome
// the gate is supposed to produce. Matches the bound already used by
// memory_pressure and ps.
const sysctlTimeout = 5 * time.Second

// SysctlLoadReader reads the 5-minute load average on macOS via
// `sysctl -n vm.loadavg`. macOS has no /proc/loadavg, so the Linux
// ProcLoadReader errors there and the CPU-load gate could never fire;
// this reader makes the gate actually work on macOS.
type SysctlLoadReader struct{}

// Load5 returns the 5-minute load average from `sysctl -n vm.loadavg`, whose
// output looks like "{ 1.23 4.56 7.89 }".
func (SysctlLoadReader) Load5() (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sysctlTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "sysctl", "-n", "vm.loadavg").Output()
	if ctx.Err() != nil {
		return 0, fmt.Errorf("sysctl vm.loadavg: %w", ctx.Err())
	}
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
