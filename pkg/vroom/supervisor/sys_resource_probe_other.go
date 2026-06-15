//go:build !linux && !darwin

package supervisor

import "context"

// sysMemoryUsedFraction returns 0 on platforms other than Linux and Darwin.
func sysMemoryUsedFraction() float64 { return 0 }

// sysSwapUsedFraction returns 0 on platforms other than Linux and Darwin.
func sysSwapUsedFraction() float64 { return 0 }

// sysFDUsedFraction returns 0 on platforms other than Linux and Darwin.
func sysFDUsedFraction() float64 { return 0 }

// sysVnodeUsedFraction returns 0 on platforms other than Darwin.
func sysVnodeUsedFraction() float64 { return 0 }

// sysGoplsCount returns 0 on platforms other than Linux and Darwin.
func sysGoplsCount(_ context.Context) int { return 0 }
