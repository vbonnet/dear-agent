//go:build darwin || linux || freebsd

package escalation

import (
	"context"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

const storeReadsRequireLock = false

type unixStoreFileLock struct {
	file *os.File
}

func acquireStoreFileLock(ctx context.Context, path string) (storeFileLock, error) {
	// #nosec G703 -- path is the fixed sidecar name beneath the configured store
	// directory; O_NOFOLLOW rejects a replacement symlink.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("escalation: open store lock: %w", err)
	}
	if err := validateUnixStoreLock(file, path); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	lock := &unixStoreFileLock{file: file}
	if err := waitForStoreFileLock(ctx, func() (bool, error) {
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, unix.EWOULDBLOCK), errors.Is(err, unix.EAGAIN), errors.Is(err, unix.EINTR):
			return false, nil
		default:
			return false, fmt.Errorf("escalation: acquire store lock: %w", err)
		}
	}); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return lock, nil
}

func validateUnixStoreLock(file *os.File, path string) error {
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("escalation: inspect open store lock: %w", err)
	}
	// #nosec G703 -- path is the fixed sidecar beneath the configured Store
	// directory, and the already-open descriptor is compared with this entry.
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("escalation: inspect store lock path: %w", err)
	}
	if !info.Mode().IsRegular() || pathInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, pathInfo) {
		return fmt.Errorf("escalation: store lock is not a stable regular file: %s", path)
	}
	return nil
}

func (l *unixStoreFileLock) Close() error {
	unlockErr := unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
	return errors.Join(unlockErr, l.file.Close())
}
