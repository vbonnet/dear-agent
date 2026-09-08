//go:build linux

package steps

import (
	"io/fs"
	"syscall"

	"golang.org/x/sys/unix"
)

const buildAuthorityNoFollowOpenFlag = unix.O_NOFOLLOW

func buildAuthorityWaitOwnershipSameChangeTime(first, second fs.FileInfo) bool {
	firstStat, firstOK := first.Sys().(*syscall.Stat_t)
	secondStat, secondOK := second.Sys().(*syscall.Stat_t)
	return firstOK && secondOK && firstStat.Ctim == secondStat.Ctim
}
