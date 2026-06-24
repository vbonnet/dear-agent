//go:build !darwin && !linux

package main

import "fmt"

// readLoad5 is not implemented on this platform.
func readLoad5() (float64, error) {
	return 0, fmt.Errorf("readLoad5 not implemented on this platform")
}

// readFreeMemPct is not implemented on this platform.
func readFreeMemPct() (float64, error) {
	return 0, fmt.Errorf("readFreeMemPct not implemented on this platform")
}
