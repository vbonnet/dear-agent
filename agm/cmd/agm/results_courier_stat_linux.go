//go:build linux

package main

import (
	"os"
	"syscall"
	"time"
)

func courierFileChangeTime(info os.FileInfo) int64 {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0
	}
	return time.Unix(stat.Ctim.Sec, stat.Ctim.Nsec).UnixNano()
}
