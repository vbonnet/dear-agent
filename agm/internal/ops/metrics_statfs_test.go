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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := diskMetricsFromBlockCounts("/data", tt.blockSize, tt.total, tt.free, tt.available)
			if got != tt.want {
				t.Fatalf("diskMetricsFromBlockCounts() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
