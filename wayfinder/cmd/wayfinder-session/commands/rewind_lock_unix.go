//go:build unix

package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	// #nosec G703 -- lockPath is a SHA-256 filename inside the verified private per-user lock directory.
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open rewind lock: %w", err)
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

func rewindLockFilePath(projectDir string) (string, error) {
	// #nosec G703 -- projectDir is the explicit Wayfinder project target; Stat reads only its stable filesystem identity.
	info, err := os.Stat(projectDir)
	if err != nil {
		return "", fmt.Errorf("inspect rewind project directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("rewind project path is not a directory: %s", projectDir)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("rewind project directory has unsupported identity metadata: %s", projectDir)
	}
	identity := fmt.Sprintf("unix:%d:%d", stat.Dev, stat.Ino)
	filename := rewindLockFilenameForIdentity(identity)
	lockDir, err := rewindLockDirectory()
	if err != nil {
		return "", err
	}
	if err := ensurePrivateRewindLockDirectory(lockDir); err != nil {
		return "", err
	}
	return filepath.Join(lockDir, filename), nil
}

func ensurePrivateRewindLockDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create rewind lock directory: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect rewind lock directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("rewind lock path is not an owned directory: %s", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	// #nosec G115 -- effective Unix user IDs are non-negative and Stat_t.Uid is uint32.
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("rewind lock directory is not owned by the current user: %s", path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("rewind lock directory permissions are too broad: %s", path)
	}
	return nil
}

func (l *unixRewindTransitionLock) Close() error {
	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	return errors.Join(unlockErr, closeErr)
}
