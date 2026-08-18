package steps

import (
	"context"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/capacity"
)

func TestCapacityDetectorAllowsZeroAvailableMemory(t *testing.T) {
	state := &agmCapacityPlatformState{
		info: capacity.SystemInfo{
			TotalRAMBytes:     32 << 30,
			AvailableRAMBytes: 0,
		},
	}
	ctx := context.WithValue(context.Background(), agmCapacityPlatformStateKey{}, state)

	if err := capacityDetectorShouldReportBoundedMemory(ctx); err != nil {
		t.Fatalf("zero available memory is a valid fail-safe reading: %v", err)
	}
}
