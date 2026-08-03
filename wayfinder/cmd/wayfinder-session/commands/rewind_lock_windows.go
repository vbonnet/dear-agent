//go:build windows

package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	file, err := openRewindLockFile(lockPath)
	if err != nil {
		return nil, err
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

func openRewindLockFile(path string) (*os.File, error) {
	pathUTF16, err := windowsExtendedPathPointer(path)
	if err != nil {
		return nil, fmt.Errorf("encode rewind lock path: %w", err)
	}
	shareMode := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE)
	handle, err := windows.CreateFile(
		pathUTF16,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		shareMode,
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("open rewind lock: %w", err)
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		closeErr := windows.CloseHandle(handle)
		return nil, errors.Join(errors.New("wrap rewind lock handle"), closeErr)
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return nil, fmt.Errorf("inspect open rewind lock: %w", errors.Join(err, file.Close()))
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 || info.NumberOfLinks != 1 {
		validationErr := fmt.Errorf("rewind lock is not a private project-owned regular file: %s", path)
		return nil, errors.Join(validationErr, file.Close())
	}
	return file, nil
}

func validatePrivateRewindLockDirectories(wayfinderDir, lockDir string) error {
	for _, path := range []string{wayfinderDir, lockDir} {
		if err := validateRewindMetadataDirectory(path); err != nil {
			return err
		}
	}
	return nil
}

func validateRewindMetadataDirectory(path string) error {
	pathUTF16, err := windowsExtendedPathPointer(path)
	if err != nil {
		return fmt.Errorf("encode rewind lock directory path: %w", err)
	}
	attributes, err := windows.GetFileAttributes(pathUTF16)
	if err != nil {
		return fmt.Errorf("inspect rewind lock directory attributes: %w", err)
	}
	if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || attributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return fmt.Errorf("rewind lock path is not an owned directory: %s", path)
	}
	return nil
}

func windowsExtendedPathPointer(path string) (*uint16, error) {
	cleanPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve Windows lock path: %w", err)
	}
	cleanPath = strings.ReplaceAll(filepath.Clean(cleanPath), "/", `\`)
	switch {
	case strings.HasPrefix(cleanPath, `\\?\`), strings.HasPrefix(cleanPath, `\\.\`):
		// The caller already supplied an extended-length or device path.
	case strings.HasPrefix(cleanPath, `\\`):
		cleanPath = `\\?\UNC\` + strings.TrimPrefix(cleanPath, `\\`)
	default:
		cleanPath = `\\?\` + cleanPath
	}
	pathUTF16, err := windows.UTF16PtrFromString(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("encode Windows lock path: %w", err)
	}
	return pathUTF16, nil
}

func (l *windowsRewindTransitionLock) Close() error {
	unlockErr := windows.UnlockFileEx(windows.Handle(l.file.Fd()), 0, 1, 0, &l.overlapped)
	closeErr := l.file.Close()
	return errors.Join(unlockErr, closeErr)
}
