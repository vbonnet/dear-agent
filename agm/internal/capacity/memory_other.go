//go:build !darwin && !linux

package capacity

import (
	"fmt"
	"runtime"
)

func readPlatformMemoryInfo() (total, available uint64, err error) {
	return 0, 0, fmt.Errorf("memory detection is unsupported on %s", runtime.GOOS)
}
