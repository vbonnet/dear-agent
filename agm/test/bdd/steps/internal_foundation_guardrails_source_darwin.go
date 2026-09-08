//go:build darwin

package steps

import (
	"io/fs"
	"syscall"

	"golang.org/x/sys/unix"
)

const buildAuthorityNoFollowOpenFlag = unix.O_NOFOLLOW_ANY

func buildAuthorityWaitOwnershipSameChangeTime(first, second fs.FileInfo) bool {
	firstStat, firstOK := first.Sys().(*syscall.Stat_t)
	secondStat, secondOK := second.Sys().(*syscall.Stat_t)
	return firstOK && secondOK && firstStat.Ctimespec == secondStat.Ctimespec
}
