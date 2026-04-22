package ops

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// ResourceHealth contains system resource status.
type ResourceHealth struct {
	DiskUsagePercent int
	DiskFreeGB       float64
	LoadAverage      float64
	SessionCount     int
	Warnings         []string
}

// CheckResourceHealth returns current system resource status.
func CheckResourceHealth() ResourceHealth {
	h := ResourceHealth{}

	// Disk usage
	out, err := exec.Command("df", "-h", "/home").CombinedOutput()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) > 1 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 5 {
				pct := strings.TrimSuffix(fields[4], "%")
				h.DiskUsagePercent, _ = strconv.Atoi(pct)
			}
		}
	}

	// Load average
	loadBytes, err := os.ReadFile("/proc/loadavg")
	if err == nil {
		fields := strings.Fields(string(loadBytes))
		if len(fields) > 0 {
			h.LoadAverage, _ = strconv.ParseFloat(fields[0], 64)
		}
	}

	// Warnings
	if h.DiskUsagePercent > 80 {
		h.Warnings = append(h.Warnings, fmt.Sprintf("DISK: %d%% used", h.DiskUsagePercent))
	}
	if h.LoadAverage > 10 {
		h.Warnings = append(h.Warnings, fmt.Sprintf("LOAD: %.1f", h.LoadAverage))
	}

	return h
}

// IsHealthy returns true if no warnings.
func (h ResourceHealth) IsHealthy() bool {
	return len(h.Warnings) == 0
}
