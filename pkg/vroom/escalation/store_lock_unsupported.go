//go:build !darwin && !linux && !freebsd && !windows

package escalation

import (
	"context"
	"fmt"
	"runtime"
)

// Writes fail closed before publication on unsupported platforms, so there is
// no cooperating FileStore writer for Get to serialize with.
const storeReadsRequireLock = false

func acquireStoreFileLock(context.Context, string) (storeFileLock, error) {
	return nil, fmt.Errorf("escalation: cross-process store locking is unsupported on %s", runtime.GOOS)
}
