//go:build windows

package main

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func syncContinuationReceiptKeyDirectory(path string) error {
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("encode continuation receipt key directory for sync: %w", err)
	}
	handle, err := windows.CreateFile(
		pathPointer,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return fmt.Errorf("open continuation receipt key directory for sync: %w", err)
	}
	if err := windows.FlushFileBuffers(handle); err != nil {
		_ = windows.CloseHandle(handle)
		return fmt.Errorf("sync continuation receipt key directory: %w", err)
	}
	if err := windows.CloseHandle(handle); err != nil {
		return fmt.Errorf("close continuation receipt key directory: %w", err)
	}
	return nil
}
