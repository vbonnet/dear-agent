//go:build linux

package testcontext

import "golang.org/x/sys/unix"

const symlinkOpenFlags = unix.O_PATH | unix.O_CLOEXEC | unix.O_NOFOLLOW

func renameNoReplace(oldDirectoryFD int, oldName string, newDirectoryFD int, newName string) error {
	return unix.Renameat2(
		oldDirectoryFD,
		oldName,
		newDirectoryFD,
		newName,
		unix.RENAME_NOREPLACE,
	)
}
