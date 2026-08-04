//go:build darwin || linux

package main

import (
	"os"
	"syscall"
)

func openReportFile(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
}
