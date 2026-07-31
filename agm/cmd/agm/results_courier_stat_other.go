//go:build !darwin && !linux

package main

import "os"

func courierFileChangeTime(os.FileInfo) int64 {
	return 0
}
