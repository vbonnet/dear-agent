//go:build darwin

package circuitbreaker

import (
	"testing"
)

func TestParseVmStat(t *testing.T) {
	// Realistic vm_stat output from macOS Sequoia (16 KiB pages).
	const sample = `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                               27542.
Pages active:                            435088.
Pages inactive:                          151553.
Pages speculative:                         6199.
Pages throttled:                              0.
Pages wired down:                        140534.
Pages purgeable:                          16082.
"File-backed" pages:                     147048.
Pages copied on write:                   158826.
Pages zero filled:                      4985897.
Pages reactivated:                       198049.
Pages purged:                            162898.
Swapins:                                      0.
Swapouts:                                     0.
Object cache: 18 hits of 1103960 lookups (0% hit rate)
`

	pageSize, freePages, specPages, err := parseVMStat(sample)
	if err != nil {
		t.Fatalf("parseVMStat: %v", err)
	}

	if pageSize != 16384 {
		t.Errorf("pageSize = %d, want 16384", pageSize)
	}
	if freePages != 27542 {
		t.Errorf("freePages = %d, want 27542", freePages)
	}
	if specPages != 6199 {
		t.Errorf("specPages = %d, want 6199", specPages)
	}
}

func TestParseVmStatLine(t *testing.T) {
	tests := []struct {
		line string
		want int64
	}{
		{"Pages free:                               27542.", 27542},
		{"Pages speculative:                         6199.", 6199},
		{"Pages throttled:                              0.", 0},
	}

	for _, tt := range tests {
		got, err := parseVMStatLine(tt.line)
		if err != nil {
			t.Errorf("parseVMStatLine(%q): %v", tt.line, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseVMStatLine(%q) = %d, want %d", tt.line, got, tt.want)
		}
	}
}

func TestVMStatMemReader_Live(t *testing.T) {
	// Integration test: actually call vm_stat and sysctl.
	// Only runs on Darwin; verifies the result is in a sane range (1–99%).
	mr := VMStatMemReader{}
	pct, err := mr.FreeMemPct()
	if err != nil {
		t.Fatalf("FreeMemPct: %v", err)
	}
	if pct < 0 || pct > 100 {
		t.Errorf("FreeMemPct = %.2f%%, want value in [0, 100]", pct)
	}
	t.Logf("live free RAM: %.2f%%", pct)
}

func TestDefaultMemReader_IsDarwin(t *testing.T) {
	mr := DefaultMemReader()
	if mr == nil {
		t.Fatal("expected non-nil MemReader on Darwin")
	}
	if _, ok := mr.(VMStatMemReader); !ok {
		t.Errorf("expected VMStatMemReader, got %T", mr)
	}
}
