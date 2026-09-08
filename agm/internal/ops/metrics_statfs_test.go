package ops

import (
	"math"
	"testing"
)

func TestNonNegativeStatfsBlockCount(t *testing.T) {
	t.Parallel()

	if got := nonNegativeStatfsBlockCount(int64(-1)); got != 0 {
		t.Fatalf("negative signed count normalized to %d, want 0", got)
	}
	if got := nonNegativeStatfsBlockCount(int64(7)); got != 7 {
		t.Fatalf("positive signed count normalized to %d, want 7", got)
	}
	if got := nonNegativeStatfsBlockCount(uint64(7)); got != 7 {
		t.Fatalf("positive unsigned count normalized to %d, want 7", got)
	}
}

func TestDiskMetricsFromBlockCounts(t *testing.T) {
	t.Parallel()

	const gibibyte = uint64(1024 * 1024 * 1024)
	tests := []struct {
		name      string
		blockSize uint64
		total     uint64
		free      uint64
		available uint64
		want      DiskMetrics
		wantErr   bool
	}{
		{
			name:      "normal",
			blockSize: gibibyte,
			total:     10,
			free:      4,
			available: 3,
			want: DiskMetrics{
				Mount:       "/data",
				TotalGB:     10,
				UsedGB:      6,
				AvailGB:     3,
				UsedPercent: 60,
			},
		},
		{
			name:      "kernel counters are capped at total",
			blockSize: gibibyte,
			total:     10,
			free:      20,
			available: 20,
			want: DiskMetrics{
				Mount:   "/data",
				TotalGB: 10,
				AvailGB: 10,
			},
		},
		{
			name:  "zero block size",
			total: 10,
			want:  DiskMetrics{Mount: "/data"},
		},
		{
			name:      "zero total blocks",
			blockSize: gibibyte,
			want:      DiskMetrics{Mount: "/data"},
		},
		{
			name:      "byte count overflow",
			blockSize: 2,
			total:     math.MaxUint64,
			want:      DiskMetrics{Mount: "/data"},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := diskMetricsFromBlockCounts("/data", tt.blockSize, tt.total, tt.free, tt.available)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("diskMetricsFromBlockCounts() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("diskMetricsFromBlockCounts() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("diskMetricsFromBlockCounts() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// Two overflowed mounts both report zeroed counters. That is absence of
// evidence, not evidence they are the same filesystem, so deduplication must
// not collapse them and discard the second mount's OverflowErr.
func TestDedupeSameFilesystemKeepsEveryOverflowingMount(t *testing.T) {
	results := []DiskMetrics{
		{Mount: "/", OverflowErr: "block arithmetic overflowed uint64"},
		{Mount: "/home", OverflowErr: "block arithmetic overflowed uint64"},
	}

	got := dedupeSameFilesystem(results)

	if len(got) != 2 {
		t.Fatalf("dedupeSameFilesystem kept %d overflowing mount(s), want 2: %+v", len(got), got)
	}
	for _, want := range []string{"/", "/home"} {
		found := false
		for _, dm := range got {
			if dm.Mount == want {
				found = true
				if dm.OverflowErr == "" {
					t.Errorf("mount %s lost its OverflowErr", want)
				}
			}
		}
		if !found {
			t.Errorf("mount %s was dropped by deduplication", want)
		}
	}
}

// A mount that overflowed and one that measured cleanly are not the same
// filesystem either, even when the clean one happens to report zero.
func TestDedupeSameFilesystemKeepsMixedOverflowAndMeasurement(t *testing.T) {
	results := []DiskMetrics{
		{Mount: "/", OverflowErr: "block arithmetic overflowed uint64"},
		{Mount: "/home"},
	}

	if got := dedupeSameFilesystem(results); len(got) != 2 {
		t.Fatalf("dedupeSameFilesystem kept %d mount(s), want 2: %+v", len(got), got)
	}
}

// Real, equal measurements still collapse: that is the case the rule exists for.
func TestDedupeSameFilesystemCollapsesEqualMeasurements(t *testing.T) {
	results := []DiskMetrics{
		{Mount: "/", TotalGB: 100, UsedGB: 40},
		{Mount: "/home", TotalGB: 100, UsedGB: 40},
	}

	got := dedupeSameFilesystem(results)

	if len(got) != 1 {
		t.Fatalf("dedupeSameFilesystem kept %d mount(s) for one filesystem, want 1: %+v", len(got), got)
	}
	if got[0].Mount != "/" {
		t.Errorf("retained mount = %q, want /", got[0].Mount)
	}
}

// Distinct real filesystems are preserved.
func TestDedupeSameFilesystemKeepsDistinctMeasurements(t *testing.T) {
	results := []DiskMetrics{
		{Mount: "/", TotalGB: 100, UsedGB: 40},
		{Mount: "/home", TotalGB: 500, UsedGB: 40},
	}

	if got := dedupeSameFilesystem(results); len(got) != 2 {
		t.Fatalf("dedupeSameFilesystem kept %d mount(s), want 2: %+v", len(got), got)
	}
}
