//go:build darwin

package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
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

// readFreeMemPct returns the percentage of free RAM on macOS using vm_stat and sysctl.
func readFreeMemPct() (float64, error) {
	vmOut, err := exec.Command("vm_stat").Output()
	if err != nil {
		return 0, fmt.Errorf("vm_stat: %w", err)
	}
	pageSize, freePages, specPages, err := parseVMStatOutput(string(vmOut))
	if err != nil {
		return 0, err
	}
	sysctlOut, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, fmt.Errorf("sysctl hw.memsize: %w", err)
	}
	totalBytes, err := strconv.ParseInt(strings.TrimSpace(string(sysctlOut)), 10, 64)
	if err != nil || totalBytes <= 0 {
		return 0, fmt.Errorf("unexpected hw.memsize output: %q", string(sysctlOut))
	}
	freeBytes := float64(freePages+specPages) * float64(pageSize)
	return freeBytes / float64(totalBytes) * 100, nil
}

func parseVMStatOutput(output string) (pageSize, freePages, specPages int64, err error) {
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.Contains(line, "page size of "):
			_, rest, ok := strings.Cut(line, "page size of ")
			if !ok {
				continue
			}
			rest = strings.TrimSuffix(rest, " bytes)")
			rest = strings.TrimSuffix(rest, " bytes")
			pageSize, err = strconv.ParseInt(strings.TrimSpace(rest), 10, 64)
			if err != nil {
				return 0, 0, 0, fmt.Errorf("parsing page size: %w", err)
			}
		case strings.HasPrefix(line, "Pages free:"):
			freePages, err = parseStatLine(line)
			if err != nil {
				return 0, 0, 0, fmt.Errorf("parsing free pages: %w", err)
			}
		case strings.HasPrefix(line, "Pages speculative:"):
			specPages, err = parseStatLine(line)
			if err != nil {
				return 0, 0, 0, fmt.Errorf("parsing speculative pages: %w", err)
			}
		}
	}
	if pageSize <= 0 {
		return 0, 0, 0, fmt.Errorf("page size not found in vm_stat output")
	}
	return pageSize, freePages, specPages, nil
}

func parseStatLine(line string) (int64, error) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("unexpected vm_stat line: %q", line)
	}
	val := strings.TrimSpace(parts[1])
	val = strings.TrimSuffix(val, ".")
	return strconv.ParseInt(val, 10, 64)
}
