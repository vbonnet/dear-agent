//go:build darwin

package testcontext

import "golang.org/x/sys/unix"

const symlinkOpenFlags = unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NONBLOCK | unix.O_SYMLINK

func renameNoReplace(oldDirectoryFD int, oldName string, newDirectoryFD int, newName string) error {
	return unix.RenameatxNp(
		oldDirectoryFD,
		oldName,
		newDirectoryFD,
		newName,
		unix.RENAME_EXCL,
	)
}
