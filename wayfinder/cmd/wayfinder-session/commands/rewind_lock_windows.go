//go:build windows

package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

type windowsRewindTransitionLock struct {
	file       *os.File
	overlapped windows.Overlapped
}

func acquireRewindTransitionLock(projectDir string) (rewindTransitionLock, error) {
	lockPath, err := rewindLockFilePath(projectDir)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open rewind lock: %w", err)
	}
	lock := &windowsRewindTransitionLock{file: file}
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)
	if err := windows.LockFileEx(windows.Handle(file.Fd()), flags, 0, 1, 0, &lock.overlapped); err != nil {
		closeErr := file.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, errors.Join(errRewindTransitionInProgress, closeErr)
		}
		return nil, fmt.Errorf("acquire rewind lock: %w", errors.Join(err, closeErr))
	}
	return lock, nil
}

func rewindLockFilePath(projectDir string) (lockPath string, returnErr error) {
	project, err := os.Open(projectDir)
	if err != nil {
		return "", fmt.Errorf("open rewind project directory: %w", err)
	}
	defer func() {
		if closeErr := project.Close(); closeErr != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("close rewind project directory: %w", closeErr))
		}
	}()
	info, err := project.Stat()
	if err != nil {
		return "", fmt.Errorf("inspect rewind project directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("rewind project path is not a directory: %s", projectDir)
	}
	var fileInfo windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(project.Fd()), &fileInfo); err != nil {
		return "", fmt.Errorf("read rewind project identity: %w", err)
	}
	identity := fmt.Sprintf("windows:%d:%d:%d", fileInfo.VolumeSerialNumber, fileInfo.FileIndexHigh, fileInfo.FileIndexLow)
	filename := rewindLockFilenameForIdentity(identity)
	lockDir, err := rewindLockDirectory()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		return "", fmt.Errorf("create rewind lock directory: %w", err)
	}
	return filepath.Join(lockDir, filename), nil
}

func (l *windowsRewindTransitionLock) Close() error {
	unlockErr := windows.UnlockFileEx(windows.Handle(l.file.Fd()), 0, 1, 0, &l.overlapped)
	closeErr := l.file.Close()
	return errors.Join(unlockErr, closeErr)
}
