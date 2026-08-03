package commands

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

var errRewindTransitionInProgress = errors.New("another rewind transition is already in progress")

type rewindTransitionLock interface {
	Close() error
}

func rewindLockFilenameForIdentity(identity string) string {
	digest := sha256.Sum256([]byte(identity))
	return fmt.Sprintf("rewind-%x.lock", digest[:16])
}

func rewindLockDirectory() (string, error) {
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("resolve current OS user: %w", err)
	}
	if strings.TrimSpace(currentUser.HomeDir) == "" {
		return "", fmt.Errorf("current OS user has no home directory")
	}
	cacheRoot := filepath.Join(currentUser.HomeDir, ".cache")
	if runtime.GOOS == "darwin" {
		cacheRoot = filepath.Join(currentUser.HomeDir, "Library", "Caches")
	}
	return filepath.Join(cacheRoot, "wayfinder-session", "locks"), nil
}
