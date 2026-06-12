//go:build !linux && !darwin

package supervisor

// sysMemoryUsedFraction returns 0 on platforms other than Linux and Darwin.
func sysMemoryUsedFraction() float64 { return 0 }
