//go:build !darwin && !linux

package main

import (
	"fmt"
	"os"
)

func persistentReminderLockSupported() bool {
	return false
}

func tryReminderFileLock(_ *os.File) (bool, error) {
	return false, fmt.Errorf("operating-system reminder locks are unsupported")
}
