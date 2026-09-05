//go:build windows

package escalation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// Go does not guarantee os.Rename atomicity on Windows, so Store readers use
// the same cross-process lock as writers around record replacement.
const storeReadsRequireLock = true

type windowsStoreFileLock struct {
	file       *os.File
	overlapped windows.Overlapped
}

func acquireStoreFileLock(ctx context.Context, path string) (storeFileLock, error) {
	pathUTF16, err := storeLockWindowsPath(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pathUTF16,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("escalation: open store lock: %w", err)
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		return nil, errors.Join(errors.New("escalation: wrap store lock handle"), windows.CloseHandle(handle))
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return nil, fmt.Errorf("escalation: inspect open store lock: %w", errors.Join(err, file.Close()))
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 || info.NumberOfLinks != 1 {
		return nil, errors.Join(fmt.Errorf("escalation: store lock is not a stable regular file: %s", path), file.Close())
	}

	lock := &windowsStoreFileLock{file: file}
	if err := waitForStoreFileLock(ctx, func() (bool, error) {
		flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)
		err := windows.LockFileEx(handle, flags, 0, 1, 0, &lock.overlapped)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, windows.ERROR_LOCK_VIOLATION):
			return false, nil
		default:
			return false, fmt.Errorf("escalation: acquire store lock: %w", err)
		}
	}); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return lock, nil
}

func storeLockWindowsPath(path string) (*uint16, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("escalation: resolve store lock path: %w", err)
	}
	cleanPath := strings.ReplaceAll(filepath.Clean(absPath), "/", `\`)
	switch {
	case strings.HasPrefix(cleanPath, `\\?\`), strings.HasPrefix(cleanPath, `\\.\`):
	case strings.HasPrefix(cleanPath, `\\`):
		cleanPath = `\\?\UNC\` + strings.TrimPrefix(cleanPath, `\\`)
	default:
		cleanPath = `\\?\` + cleanPath
	}
	pathUTF16, err := windows.UTF16PtrFromString(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("escalation: encode store lock path: %w", err)
	}
	return pathUTF16, nil
}

func (l *windowsStoreFileLock) Close() error {
	unlockErr := windows.UnlockFileEx(windows.Handle(l.file.Fd()), 0, 1, 0, &l.overlapped)
	return errors.Join(unlockErr, l.file.Close())
}
