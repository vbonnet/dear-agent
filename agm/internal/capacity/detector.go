package capacity

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// SystemInfo holds detected hardware and process information.
type SystemInfo struct {
	TotalRAMBytes     uint64 // Total system RAM in bytes
	AvailableRAMBytes uint64 // Available RAM in bytes (MemAvailable from /proc/meminfo)
	NumCPUs           int    // Number of logical CPU cores
	ClaudeProcesses   int    // Number of running Claude processes
}

// TotalRAMGB returns total RAM in gigabytes.
func (s SystemInfo) TotalRAMGB() float64 {
	return float64(s.TotalRAMBytes) / (1024 * 1024 * 1024)
}

// AvailableRAMGB returns available RAM in gigabytes.
func (s SystemInfo) AvailableRAMGB() float64 {
	return float64(s.AvailableRAMBytes) / (1024 * 1024 * 1024)
}

// UsedRAMBytes returns the amount of RAM currently in use.
func (s SystemInfo) UsedRAMBytes() uint64 {
	if s.TotalRAMBytes > s.AvailableRAMBytes {
		return s.TotalRAMBytes - s.AvailableRAMBytes
	}
	return 0
}

// RAMUsagePercent returns the percentage of RAM in use.
func (s SystemInfo) RAMUsagePercent() float64 {
	if s.TotalRAMBytes == 0 {
		return 0
	}
	return float64(s.UsedRAMBytes()) / float64(s.TotalRAMBytes) * 100
}

// Detector reads system resource information.
type Detector struct {
	procMeminfo     string // path to /proc/meminfo (overridable for testing)
	cpuCountFunc    func() int
	claudeCountFunc func() (int, error)
}

// NewDetector creates a Detector using real system sources.
func NewDetector() *Detector {
	return &Detector{
		procMeminfo:     "/proc/meminfo",
		cpuCountFunc:    runtime.NumCPU,
		claudeCountFunc: countClaudeProcesses,
	}
}

// Detect gathers current system information.
func (d *Detector) Detect() (SystemInfo, error) {
	total, available, err := d.readMeminfo()
	if err != nil {
		return SystemInfo{}, fmt.Errorf("reading memory info: %w", err)
	}

	claudeCount, err := d.claudeCountFunc()
	if err != nil {
		// Non-fatal: default to 0 if we can't count processes
		claudeCount = 0
	}

	return SystemInfo{
		TotalRAMBytes:     total,
		AvailableRAMBytes: available,
		NumCPUs:           d.cpuCountFunc(),
		ClaudeProcesses:   claudeCount,
	}, nil
}

// readMeminfo parses /proc/meminfo for MemTotal and MemAvailable (in bytes).
func (d *Detector) readMeminfo() (total, available uint64, err error) {
	f, err := os.Open(d.procMeminfo)
	if err != nil {
		return 0, 0, fmt.Errorf("opening %s: %w", d.procMeminfo, err)
	}
	defer f.Close()

	var foundTotal, foundAvailable bool
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			total, err = parseMeminfoLine(line)
			if err != nil {
				return 0, 0, err
			}
			foundTotal = true
		} else if strings.HasPrefix(line, "MemAvailable:") {
			available, err = parseMeminfoLine(line)
			if err != nil {
				return 0, 0, err
			}
			foundAvailable = true
		}
		if foundTotal && foundAvailable {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, fmt.Errorf("scanning %s: %w", d.procMeminfo, err)
	}
	if !foundTotal {
		return 0, 0, fmt.Errorf("MemTotal not found in %s", d.procMeminfo)
	}
	if !foundAvailable {
		return 0, 0, fmt.Errorf("MemAvailable not found in %s", d.procMeminfo)
	}
	return total, available, nil
}

// parseMeminfoLine extracts the value in bytes from a line like "MemTotal:    32456789 kB".
func parseMeminfoLine(line string) (uint64, error) {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return 0, fmt.Errorf("malformed meminfo line: %s", line)
	}
	kb, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing value from '%s': %w", line, err)
	}
	// /proc/meminfo reports in kB (1024 bytes)
	return kb * 1024, nil
}

// countClaudeProcesses counts running Claude CLI processes via pgrep.
func countClaudeProcesses() (int, error) {
	// Look for claude processes (the Claude Code CLI)
	out, err := exec.Command("pgrep", "-f", "claude").Output()
	if err != nil {
		// pgrep exits 1 when no processes found — that's fine
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			return 0, nil
		}
		return 0, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	count := 0
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			count++
		}
	}
	return count, nil
}
