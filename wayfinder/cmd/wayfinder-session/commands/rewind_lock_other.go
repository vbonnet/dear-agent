//go:build !unix && !windows

package commands

import (
	"fmt"
	"runtime"
)

func acquireRewindTransitionLock(string) (rewindTransitionLock, error) {
	return nil, fmt.Errorf("rewind transition locking is not supported on %s", runtime.GOOS)
}

func rewindLockFilePath(string) (string, error) {
	return "", fmt.Errorf("rewind transition locking is not supported on %s", runtime.GOOS)
}
