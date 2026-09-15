//go:build linux

package marketplaceparity

import "golang.org/x/sys/unix"

func anchoredChangeTime(stat *unix.Stat_t) anchoredTimestamp {
	return anchoredTimestamp{
		seconds:     int64(stat.Ctim.Sec),
		nanoseconds: int64(stat.Ctim.Nsec),
	}
}
