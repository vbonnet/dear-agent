//go:build darwin

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// readLoad5 reads the 5-minute load average on macOS via `sysctl -n vm.loadavg`.
func readLoad5() (float64, error) {
	out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output()
	if err != nil {
		return 0, fmt.Errorf("sysctl vm.loadavg: %w", err)
	}
	// Output format: "{ 1.23 4.56 7.89 }"
	s := strings.Trim(strings.TrimSpace(string(out)), "{}")
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return 0, fmt.Errorf("unexpected sysctl vm.loadavg output: %q", string(out))
	}
	return strconv.ParseFloat(fields[1], 64)
}

// memoryPressureTimeout bounds the memory_pressure -Q subprocess call so a
// hung or slow probe fails the read instead of hanging the governor loop.
const memoryPressureTimeout = 5 * time.Second

// readFreeMemPct returns the percentage of free RAM on macOS using
// `memory_pressure -Q`, the same tool that backs Activity Monitor's memory
// pressure gauge and the OS-level low-memory notifications apps respond to.
//
// This used to be computed from vm_stat as (free_pages + speculative_pages)
// / total, which reports single-digit percentages on a healthy idle Mac:
// macOS deliberately keeps raw "free" pages near zero, parking
// recently-used file-backed content in the inactive queue (reclaimable
// instantly, at zero cost) instead of actually freeing it. That formula
// reported free memory as low as 1.8% on a machine memory_pressure -Q
// measured at 57-65% free. Deferring to macOS's own pressure calculation
// (which does account for the inactive queue and compressor headroom)
// matches what a human checking Activity Monitor would see, instead of
// reimplementing an approximation of Apple's internal accounting.
func readFreeMemPct() (float64, error) {
	bgCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(bgCtx, memoryPressureTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "memory_pressure", "-Q").Output()
	if ctx.Err() != nil {
		return 0, fmt.Errorf("memory_pressure -Q: %w", ctx.Err())
	}
	if err != nil {
		return 0, fmt.Errorf("memory_pressure -Q: %w", err)
	}
	return parseMemoryPressure(string(out))
}

// parseMemoryPressure extracts the free-memory percentage from
// `memory_pressure -Q` output, e.g.:
//
//	The system has 25769803776 (1572864 pages with a page size of 16384).
//	System-wide memory free percentage: 69%
func parseMemoryPressure(output string) (float64, error) {
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		_, rest, ok := strings.Cut(line, "System-wide memory free percentage:")
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)
		rest = strings.TrimSuffix(rest, "%")
		pct, err := strconv.ParseFloat(rest, 64)
		if err != nil {
			return 0, fmt.Errorf("parsing free percentage from %q: %w", line, err)
		}
		return pct, nil
	}
	return 0, fmt.Errorf("memory_pressure -Q output missing free percentage line: %q", output)
}
