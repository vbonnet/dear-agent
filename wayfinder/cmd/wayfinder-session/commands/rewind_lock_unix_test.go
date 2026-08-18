//go:build unix

package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRewindTransitionLockRejectsWritableMetadataParentBeforeCreatingLockDirectory(t *testing.T) {
	projectDir := t.TempDir()
	wayfinderDir := filepath.Join(projectDir, ".wayfinder")
	if err := os.Mkdir(wayfinderDir, 0o700); err != nil {
		t.Fatalf("create Wayfinder metadata directory: %v", err)
	}
	if err := os.Chmod(wayfinderDir, 0o777); err != nil {
		t.Fatalf("make Wayfinder metadata directory writable by other users: %v", err)
	}

	lock, err := acquireRewindTransitionLock(projectDir)
	if err == nil {
		if closeErr := lock.Close(); closeErr != nil {
			t.Errorf("release unexpectedly acquired lock: %v", closeErr)
		}
		t.Fatal("acquire rewind lock succeeded with replaceable metadata parent")
	}
	if _, statErr := os.Stat(filepath.Join(wayfinderDir, "locks")); !os.IsNotExist(statErr) {
		t.Fatalf("lock directory was created before metadata ownership validation: %v", statErr)
	}
}
