//go:build !unix && !windows

package main

import (
	"errors"
	"os"
)

func openContinuationReceiptKey(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("continuation receipt key must not be a symlink")
	}
	file, err := os.Open(path) //nolint:gosec // G304/G703: fixed tool-owned state path; identity and descriptor validation bracket reading
	if err != nil {
		return nil, err
	}
	openedInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return nil, errors.New("continuation receipt key changed while it was opened")
	}
	return file, nil
}
