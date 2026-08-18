//go:build linux

package capacity

func readPlatformMemoryInfo() (total, available uint64, err error) {
	return readMeminfo("/proc/meminfo")
}
