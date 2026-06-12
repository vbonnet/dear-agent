//go:build linux

package supervisor

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// sysMemoryUsedFraction returns the fraction of physical RAM currently in use
// on Linux by parsing /proc/meminfo.
func sysMemoryUsedFraction() float64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()

	vals := make(map[string]uint64, 4)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		key, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		// rest looks like "  1234567 kB"
		fields := strings.Fields(strings.TrimSpace(rest))
		if len(fields) == 0 {
			continue
		}
		v, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		vals[key] = v
		if len(vals) == 2 && vals["MemTotal"] > 0 && vals["MemAvailable"] > 0 {
			break
		}
	}

	total := vals["MemTotal"]
	avail := vals["MemAvailable"]
	if total == 0 || avail > total {
		return 0
	}
	return float64(total-avail) / float64(total)
}
