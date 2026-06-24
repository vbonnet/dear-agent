//go:build linux

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// readLoad5 reads the 5-minute load average from /proc/loadavg on Linux.
func readLoad5() (float64, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, fmt.Errorf("reading /proc/loadavg: %w", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0, fmt.Errorf("unexpected /proc/loadavg format: %q", string(data))
	}
	return strconv.ParseFloat(fields[1], 64)
}

// readFreeMemPct returns the percentage of free RAM on Linux from /proc/meminfo.
// It computes (MemAvailable / MemTotal) × 100.
func readFreeMemPct() (float64, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, fmt.Errorf("reading /proc/meminfo: %w", err)
	}
	var total, available int64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total, err = strconv.ParseInt(fields[1], 10, 64)
			if err != nil {
				return 0, fmt.Errorf("parse MemTotal: %w", err)
			}
		case "MemAvailable:":
			available, err = strconv.ParseInt(fields[1], 10, 64)
			if err != nil {
				return 0, fmt.Errorf("parse MemAvailable: %w", err)
			}
		}
	}
	if total <= 0 {
		return 0, fmt.Errorf("MemTotal not found in /proc/meminfo")
	}
	return float64(available) / float64(total) * 100, nil
}
