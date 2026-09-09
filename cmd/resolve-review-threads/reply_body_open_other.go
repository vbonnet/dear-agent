//go:build !unix

package main

import "os"

func openReplyBodyFile(path string) (*os.File, error) {
	return os.Open(path) //nolint:gosec // G304/G703: operator-selected data file; descriptor validation precedes reading
}
