package ops

import (
	"testing"
)

func TestResourceHealth_IsHealthy(t *testing.T) {
	tests := []struct {
		name     string
		health   ResourceHealth
		wantSafe bool
	}{
		{
			name:     "no warnings is healthy",
			health:   ResourceHealth{DiskUsagePercent: 50, LoadAverage: 1.0},
			wantSafe: true,
		},
		{
			name:     "disk warning makes unhealthy",
			health:   ResourceHealth{Warnings: []string{"DISK: 85% used"}},
			wantSafe: false,
		},
		{
			name:     "load warning makes unhealthy",
			health:   ResourceHealth{Warnings: []string{"LOAD: 12.0"}},
			wantSafe: false,
		},
		{
			name:     "multiple warnings makes unhealthy",
			health:   ResourceHealth{Warnings: []string{"DISK: 90% used", "LOAD: 15.0"}},
			wantSafe: false,
		},
		{
			name:     "zero value is healthy",
			health:   ResourceHealth{},
			wantSafe: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.health.IsHealthy(); got != tt.wantSafe {
				t.Errorf("IsHealthy() = %v, want %v", got, tt.wantSafe)
			}
		})
	}
}

func TestCheckResourceHealth(t *testing.T) {
	// Integration test: runs against the real system.
	// Verifies that CheckResourceHealth does not panic and returns
	// sensible values on any Linux host.
	h := CheckResourceHealth()

	if h.DiskUsagePercent < 0 || h.DiskUsagePercent > 100 {
		t.Errorf("DiskUsagePercent out of range: %d", h.DiskUsagePercent)
	}

	if h.LoadAverage < 0 {
		t.Errorf("LoadAverage negative: %f", h.LoadAverage)
	}

	// Verify warnings are consistent with thresholds.
	diskWarned := false
	loadWarned := false
	for _, w := range h.Warnings {
		if len(w) >= 4 && w[:4] == "DISK" {
			diskWarned = true
		}
		if len(w) >= 4 && w[:4] == "LOAD" {
			loadWarned = true
		}
	}

	if h.DiskUsagePercent > 80 && !diskWarned {
		t.Error("expected disk warning when usage > 80%")
	}
	if h.DiskUsagePercent <= 80 && diskWarned {
		t.Error("unexpected disk warning when usage <= 80%")
	}
	if h.LoadAverage > 10 && !loadWarned {
		t.Error("expected load warning when load > 10")
	}
	if h.LoadAverage <= 10 && loadWarned {
		t.Error("unexpected load warning when load <= 10")
	}
}
