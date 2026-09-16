//go:build unix

package main

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// openReplyBodyFile prevents a substituted FIFO from blocking before the
// caller can validate the opened descriptor. O_NONBLOCK has no effect on reads
// from a regular file.
func openReplyBodyFile(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		return nil, errors.Join(errors.New("wrap reply body file descriptor"), unix.Close(fd))
	}
	return file, nil
}
