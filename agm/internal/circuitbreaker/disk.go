package circuitbreaker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// StatfsDiskReader reports the free disk space on the filesystem containing
// Path, via statfs(2). It works on macOS and Linux and is the volume where
// agent worktrees and sandboxes live, so it observes the same space an
// unbounded fill would consume.
type StatfsDiskReader struct {
	// Path is the path whose target volume is measured. Readers built by
	// NewDiskReader retain the planned sandbox path here and re-resolve its
	// deepest existing ancestor at every probe. When empty it defaults to the
	// user home directory.
	Path string

	resolvePlannedPath bool
}

// bytesPerGiB is 2^30, used to convert statfs block counts to gigabytes.
const bytesPerGiB = 1024 * 1024 * 1024

// FreeDiskGB returns free space in gigabytes (GiB), using blocks available to
// unprivileged callers (Bavail) so it matches the space a spawn can actually
// consume.
func (d StatfsDiskReader) FreeDiskGB() (float64, error) {
	path := d.Path
	if d.resolvePlannedPath {
		var err error
		path, err = deepestExistingAncestor(path)
		if err != nil {
			return 0, err
		}
	}
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

	return float64(st.Bavail) * float64(st.Bsize) / bytesPerGiB, nil
}

// NewDiskReader returns a disk reader for the filesystem that will hold
// plannedSandboxRoot. The root need not exist yet: construction walks upward
// only through literal ENOENT failures to validate a probe location. The reader
// repeats that resolution on every probe so a newly provisioned root or mount
// is measured directly. Any other inspection error fails closed.
func NewDiskReader(plannedSandboxRoot string) (DiskReader, error) {
	if plannedSandboxRoot == "" {
		return nil, fmt.Errorf("planned sandbox root must not be empty")
	}
	if !filepath.IsAbs(plannedSandboxRoot) {
		return nil, fmt.Errorf("planned sandbox root must be absolute: %q", plannedSandboxRoot)
	}
	if clean := filepath.Clean(plannedSandboxRoot); clean != plannedSandboxRoot {
		return nil, fmt.Errorf("planned sandbox root must be clean: %q", plannedSandboxRoot)
	}
	if filepath.Dir(plannedSandboxRoot) == plannedSandboxRoot {
		return nil, fmt.Errorf("planned sandbox root must not be the filesystem root: %q", plannedSandboxRoot)
	}

	if _, err := deepestExistingAncestor(plannedSandboxRoot); err != nil {
		return nil, err
	}
	return StatfsDiskReader{Path: plannedSandboxRoot, resolvePlannedPath: true}, nil
}

func deepestExistingAncestor(path string) (string, error) {
	for candidate := path; ; candidate = filepath.Dir(candidate) {
		info, err := os.Lstat(candidate)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("disk probe path %q is a symlink", candidate)
			}
			if !info.IsDir() {
				return "", fmt.Errorf("disk probe path %q is not a directory", candidate)
			}
			resolved, resolveErr := filepath.EvalSymlinks(candidate)
			if resolveErr != nil {
				return "", fmt.Errorf("resolve disk probe path %q: %w", candidate, resolveErr)
			}
			if resolved != candidate {
				return "", fmt.Errorf("disk probe path %q contains a symlink and resolves as %q", candidate, resolved)
			}
			return candidate, nil
		}
		if !errors.Is(err, unix.ENOENT) {
			return "", fmt.Errorf("inspect disk probe path %q: %w", candidate, err)
		}
		if parent := filepath.Dir(candidate); parent == candidate {
			return "", fmt.Errorf("no existing ancestor for planned sandbox root %q", path)
		}
	}
}

// DefaultDiskReader returns a StatfsDiskReader rooted at the user home
// directory (the data volume that backs agent working directories).
func DefaultDiskReader() DiskReader {
	return StatfsDiskReader{}
}
