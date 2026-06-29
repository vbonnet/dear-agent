//go:build !linux && !darwin

package supervisor

import "context"

// sysMemoryUsedFraction returns 0 on platforms other than Linux and Darwin.
func sysMemoryUsedFraction() float64 { return 0 }

// sysFreeMemoryBytes returns 0 on platforms other than Linux and Darwin
// (treated as "unknown" by the monitor, never as "0 bytes free").
func sysFreeMemoryBytes() uint64 { return 0 }

// sysSwapUsedFraction returns 0 on platforms other than Linux and Darwin.
func sysSwapUsedFraction() float64 { return 0 }

// sysFDUsedFraction returns 0 on platforms other than Linux and Darwin.
func sysFDUsedFraction() float64 { return 0 }

// sysVnodeUsedFraction returns 0 on platforms other than Darwin.
func sysVnodeUsedFraction() float64 { return 0 }

// sysGoplsCount returns 0 on platforms other than Linux and Darwin.
func sysGoplsCount(_ context.Context) int { return 0 }

// sysMemorystatusLevel returns 0 on platforms other than Darwin.
func sysMemorystatusLevel() int { return 0 }

// sysCorrectMemoryMetrics is a no-op on platforms other than Darwin.
func sysCorrectMemoryMetrics(_ context.Context, snap ResourceSnapshot) (float64, uint64) {
	return snap.MemoryUsedFraction, snap.FreePhysicalMemoryBytes
}
