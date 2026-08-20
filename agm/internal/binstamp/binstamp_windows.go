//go:build windows

package binstamp

import "os"

// Windows exposes no equivalent to a POSIX inode through os.FileInfo, so a
// build here relies on Size and ModTime alone to detect a replaced binary.
func getInode(os.FileInfo) uint64 {
	return 0
}
