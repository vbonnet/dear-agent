//go:build linux

package specpackage

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func sameStableRegularStat(left, right *unix.Stat_t) bool {
	return identityFromUnixStat(left) == identityFromUnixStat(right) &&
		stableStatToken(left) == stableStatToken(right)
}

func stableStatToken(stat *unix.Stat_t) string {
	return fmt.Sprintf(
		"%d:%d:%d:%d:%d:%d:%d",
		stat.Mode,
		stat.Nlink,
		stat.Size,
		stat.Mtim.Sec,
		stat.Mtim.Nsec,
		stat.Ctim.Sec,
		stat.Ctim.Nsec,
	)
}
