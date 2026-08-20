//go:build !windows

package binstamp

import (
	"os"
	"syscall"
)

func getInode(info os.FileInfo) uint64 {
	if sys, ok := info.Sys().(*syscall.Stat_t); ok {
		return sys.Ino
	}
	return 0
}
