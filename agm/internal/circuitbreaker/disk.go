package circuitbreaker

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// StatfsDiskReader reports the percentage of free disk space on the filesystem
// containing Path, via statfs(2). It works on macOS and Linux and is the
// volume where agent worktrees and sandboxes live, so it observes the same
// space an unbounded fill would consume.
type StatfsDiskReader struct {
	// Path is any path on the target volume. When empty it defaults to the
	// user home directory, which on this host resolves to the data volume
	// that holds ~/worktrees and ~/.agm/sandboxes.
	Path string
}

// FreeDiskPct returns free space as a percentage (0–100) of total space,
// using blocks available to unprivileged callers (Bavail) so it matches the
// space a spawn can actually consume.
func (d StatfsDiskReader) FreeDiskPct() (float64, error) {
	path := d.Path
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return 0, fmt.Errorf("resolving home dir for disk check: %w", err)
		}
		path = home
	}

	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, fmt.Errorf("statfs %s: %w", path, err)
	}
	if st.Blocks == 0 {
		return 0, fmt.Errorf("statfs %s: zero total blocks", path)
	}

	return float64(st.Bavail) / float64(st.Blocks) * 100, nil
}

// DefaultDiskReader returns a StatfsDiskReader rooted at the user home
// directory (the data volume that backs agent working directories).
func DefaultDiskReader() DiskReader {
	return StatfsDiskReader{}
}
