//go:build darwin

package circuitbreaker

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// VMStatMemReader reads free memory percentage on macOS using vm_stat and sysctl.
type VMStatMemReader struct{}

// FreeMemPct returns the percentage of free RAM (0–100) by reading vm_stat
// output. It computes: (free_pages + speculative_pages) × page_size / hw.memsize × 100.
//
// Speculative pages are included because macOS eagerly pre-faults data but
// can reclaim those pages instantly — they behave like free memory from the
// OS scheduler's perspective.
func (VMStatMemReader) FreeMemPct() (float64, error) {
	vmOut, err := exec.Command("vm_stat").Output()
	if err != nil {
		return 0, fmt.Errorf("vm_stat: %w", err)
	}

	pageSize, freePages, specPages, err := parseVMStat(string(vmOut))
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

// parseVMStat extracts page size, free pages, and speculative pages from
// vm_stat output. The first line has the form:
//
//	Mach Virtual Memory Statistics: (page size of 16384 bytes)
func parseVMStat(output string) (pageSize, freePages, specPages int64, err error) {
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.Contains(line, "page size of "):
			// "Mach Virtual Memory Statistics: (page size of 16384 bytes)"
			_, rest, ok := strings.Cut(line, "page size of ")
			if !ok {
				continue
			}
			rest = strings.TrimSuffix(rest, " bytes)")
			rest = strings.TrimSuffix(rest, " bytes")
			pageSize, err = strconv.ParseInt(strings.TrimSpace(rest), 10, 64)
			if err != nil {
				return 0, 0, 0, fmt.Errorf("parsing page size from %q: %w", line, err)
			}
		case strings.HasPrefix(line, "Pages free:"):
			freePages, err = parseVMStatLine(line)
			if err != nil {
				return 0, 0, 0, fmt.Errorf("parsing free pages: %w", err)
			}
		case strings.HasPrefix(line, "Pages speculative:"):
			specPages, err = parseVMStatLine(line)
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

// parseVMStatLine parses a vm_stat line of the form "Pages free: 12345."
func parseVMStatLine(line string) (int64, error) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("unexpected vm_stat line: %q", line)
	}
	val := strings.TrimSpace(parts[1])
	val = strings.TrimSuffix(val, ".")
	return strconv.ParseInt(val, 10, 64)
}

// DefaultMemReader returns the platform-native memory reader (VMStatMemReader on macOS).
func DefaultMemReader() MemReader {
	return VMStatMemReader{}
}
