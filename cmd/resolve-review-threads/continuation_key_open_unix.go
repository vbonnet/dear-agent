//go:build unix

package main

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func openContinuationReceiptKey(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		return nil, errors.Join(errors.New("wrap continuation receipt key descriptor"), unix.Close(fd))
	}
	return file, nil
}
