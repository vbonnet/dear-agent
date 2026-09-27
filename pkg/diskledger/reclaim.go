package diskledger

import (
	"fmt"
	"syscall"
)

// FreeBytes reports the space available on the filesystem holding path.
//
// This is Bavail (blocks available to an unprivileged caller), not Bfree,
// because the reserved blocks a privileged writer could still use are not
// space an agent run will ever get.
func FreeBytes(path string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, fmt.Errorf("statfs %s: %w", path, err)
	}
	//nolint:gosec // block counts and sizes are far below int64 overflow
	return int64(st.Bavail) * int64(st.Bsize), nil
}

// Reclaim runs a cleanup operation and reports how many bytes the filesystem
// actually returned, measured by statfs before and after.
//
// This exists because a directory walk cannot answer the question. Measure
// deduplicates by inode, which is right for hardlinks and useless for APFS
// clones: a clone is a distinct inode whose st_blocks reports the full
// allocation even though creating it consumed nothing. Sandboxes on this host
// are provisioned with clonefile (internal/sandbox/apfs/provider.go uses
// "cp -c"), so the dominant shape is exactly the one the walk gets wrong.
//
// Grading a collector on walked bytes therefore flatters it in the one
// direction that matters: a sweep that deletes a pile of clones and returns
// almost nothing to the disk reports a large reclaim and looks healthy. The
// filesystem's own free-space delta cannot be flattered that way.
//
// The number is honest but not exact. Anything else writing to the same
// filesystem during the operation moves free space too, so on a busy host the
// result carries that noise. It is floored at zero: a concurrent writer that
// consumes more than the operation released would otherwise produce a negative
// reclaim, which reads as a bug rather than as the measurement noise it is.
// Callers that need to reason about the noise should sample FreeBytes twice
// themselves and compare.
//
// An error from op is returned with a zero reclaim, so a cleanup that failed is
// never recorded as a successful reclaim of nothing.
func Reclaim(path string, op func() error) (int64, error) {
	before, err := FreeBytes(path)
	if err != nil {
		return 0, err
	}
	if err := op(); err != nil {
		return 0, fmt.Errorf("reclaim operation: %w", err)
	}
	after, err := FreeBytes(path)
	if err != nil {
		return 0, err
	}
	freed := after - before
	if freed < 0 {
		return 0, nil
	}
	return freed, nil
}
