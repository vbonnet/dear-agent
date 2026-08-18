//go:build darwin

package capacity

import (
	"fmt"
	"math"

	"golang.org/x/sys/unix"

	"github.com/vbonnet/dear-agent/agm/internal/circuitbreaker"
)

func readPlatformMemoryInfo() (total, available uint64, err error) {
	return readDarwinMemoryInfo(
		func() (uint64, error) { return unix.SysctlUint64("hw.memsize") },
		circuitbreaker.DefaultMemReader().FreeMemPct,
	)
}

func readDarwinMemoryInfo(
	totalReader func() (uint64, error),
	freePercentReader func() (float64, error),
) (total, available uint64, err error) {
	total, err = totalReader()
	if err != nil {
		return 0, 0, fmt.Errorf("reading hw.memsize: %w", err)
	}
	if total == 0 {
		return 0, 0, fmt.Errorf("reading hw.memsize: returned zero bytes")
	}

	freePercent, err := freePercentReader()
	if err != nil {
		return 0, 0, fmt.Errorf("reading memory pressure: %w", err)
	}
	if math.IsNaN(freePercent) || math.IsInf(freePercent, 0) || freePercent < 0 || freePercent > 100 {
		return 0, 0, fmt.Errorf("reading memory pressure: free percentage %.2f is outside [0, 100]", freePercent)
	}

	available = uint64(float64(total) * freePercent / 100)
	available = min(available, total)
	return total, available, nil
}
