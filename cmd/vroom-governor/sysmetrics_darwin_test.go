//go:build darwin

package main

import "testing"

func TestParseMemoryPressure(t *testing.T) {
	const sample = `The system has 25769803776 (1572864 pages with a page size of 16384).
System-wide memory free percentage: 69%
`
	pct, err := parseMemoryPressure(sample)
	if err != nil {
		t.Fatalf("parseMemoryPressure: %v", err)
	}
	if pct != 69 {
		t.Errorf("pct = %v, want 69", pct)
	}
}

// TestParseMemoryPressure_LowFreeFalsePositive documents the bug this fix
// replaces: the old vm_stat-derived formula (free+speculative only) reported
// single-digit "free" percentages on a healthy idle Mac because macOS parks
// reclaimable file-backed content in the inactive queue instead of counting
// it as free. Deferring to `memory_pressure -Q` avoids reimplementing an
// approximation of Apple's own accounting, so a genuinely healthy machine
// is parsed as such instead of tripping the min-free-mem-pct spawn pause.
func TestParseMemoryPressure_LowFreeFalsePositive(t *testing.T) {
	const sample = `The system has 25769803776 (1572864 pages with a page size of 16384).
System-wide memory free percentage: 65%
`
	pct, err := parseMemoryPressure(sample)
	if err != nil {
		t.Fatalf("parseMemoryPressure: %v", err)
	}
	if pct < 10 {
		t.Errorf("pct = %v, want >= 10 (should not trip the default min-free-mem-pct threshold)", pct)
	}
}

func TestParseMemoryPressure_MissingLine(t *testing.T) {
	_, err := parseMemoryPressure("some unexpected output\n")
	if err == nil {
		t.Fatal("expected an error when the free percentage line is missing")
	}
}

func TestParseMemoryPressure_MalformedPercentage(t *testing.T) {
	const sample = `System-wide memory free percentage: not-a-number%`
	_, err := parseMemoryPressure(sample)
	if err == nil {
		t.Fatal("expected an error for a non-numeric percentage")
	}
}

func TestReadFreeMemPct_Live(t *testing.T) {
	// Integration test: actually calls memory_pressure -Q.
	pct, err := readFreeMemPct()
	if err != nil {
		t.Fatalf("readFreeMemPct: %v", err)
	}
	if pct < 0 || pct > 100 {
		t.Errorf("readFreeMemPct = %.2f%%, want value in [0, 100]", pct)
	}
	t.Logf("live free RAM: %.2f%%", pct)
}
