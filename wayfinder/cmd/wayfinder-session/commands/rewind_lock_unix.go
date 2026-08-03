//go:build unix

package commands

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

type unixRewindTransitionLock struct {
	file *os.File
}

func acquireRewindTransitionLock(projectDir string) (rewindTransitionLock, error) {
	lockPath, err := rewindLockFilePath(projectDir)
	if err != nil {
		return nil, err
	}
	file, err := openRewindLockFile(lockPath)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		closeErr := file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, errors.Join(errRewindTransitionInProgress, closeErr)
		}
		return nil, fmt.Errorf("acquire rewind lock: %w", errors.Join(err, closeErr))
	}
	return &unixRewindTransitionLock{file: file}, nil
}

func openRewindLockFile(path string) (*os.File, error) {
	// #nosec G703 -- path is a fixed filename beneath the validated project metadata directories; O_NOFOLLOW rejects replacement links.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open rewind lock: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect open rewind lock: %w", errors.Join(err, file.Close()))
	}
	// #nosec G703 -- compare the opened object with the fixed project-local path to reject replacement races.
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect rewind lock path: %w", errors.Join(err, file.Close()))
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	// #nosec G115 -- effective Unix user IDs are non-negative and Stat_t.Uid is uint32.
	if !info.Mode().IsRegular() || pathInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, pathInfo) || !ok || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 || info.Mode().Perm()&0o077 != 0 {
		validationErr := fmt.Errorf("rewind lock is not a private project-owned regular file: %s", path)
		return nil, errors.Join(validationErr, file.Close())
	}
	return file, nil
}

func validatePrivateRewindLockDirectories(wayfinderDir, lockDir string) error {
	if err := validateRewindMetadataDirectory(wayfinderDir); err != nil {
		return err
	}
	return validateOwnedRewindDirectory(lockDir, 0o077)
}

func validateRewindMetadataDirectory(path string) error {
	return validateOwnedRewindDirectory(path, 0o022)
}

func validateOwnedRewindDirectory(path string, prohibitedPermissions os.FileMode) error {
	// #nosec G703 -- revalidate the fixed project metadata component after creation to reject link replacement.
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect rewind lock directory: %w", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	// #nosec G115 -- effective Unix user IDs are non-negative and Stat_t.Uid is uint32.
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !ok || stat.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("rewind lock directory is not owned by the current user: %s", path)
	}
	if info.Mode().Perm()&prohibitedPermissions != 0 {
		return fmt.Errorf("rewind lock directory permissions are too broad: %s", path)
	}
	return nil
}

func (l *unixRewindTransitionLock) Close() error {
	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	return errors.Join(unlockErr, closeErr)
}
