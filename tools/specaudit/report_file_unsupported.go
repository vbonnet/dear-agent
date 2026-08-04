//go:build !darwin && !linux

package main

import (
	"errors"
	"os"
)

func openReportFile(*os.Root, string) (*os.File, error) {
	return nil, errors.New("authenticated no-follow report opening is unsupported on this operating system")
}
