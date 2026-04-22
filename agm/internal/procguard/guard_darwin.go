//go:build darwin

package procguard

import "fmt"

// ApplyNprocLimit is a no-op on macOS: prlimit(2) is Linux-only.
// Process-level nproc constraints are not supported on Darwin.
func ApplyNprocLimit(pid int, limit uint64) error {
	return fmt.Errorf("ApplyNprocLimit: prlimit not supported on macOS (pid %d)", pid)
}
