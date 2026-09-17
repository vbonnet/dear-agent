package sandbox

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOrphanCleanupPlatformApplicability(t *testing.T) {
	t.Parallel()

	for _, goos := range []string{"darwin", "linux"} {
		if err := orphanCleanupPlatformError(goos); err != nil {
			t.Errorf("orphanCleanupPlatformError(%q) = %v, want nil", goos, err)
		}
	}
	for _, goos := range []string{"freebsd", "openbsd", "windows"} {
		err := orphanCleanupPlatformError(goos)
		if err == nil {
			t.Errorf("orphanCleanupPlatformError(%q) = nil, want unsupported", goos)
			continue
		}
		assertUnsupportedCleanupError(t, err)
	}
}

func TestCleanupOrphanedUnsupportedPlatformPreservesDirectories(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "sandbox-old")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("create sandbox directory: %v", err)
	}
	sentinel := filepath.Join(dir, "sentinel")
	if err := os.WriteFile(sentinel, []byte("preserve"), 0o600); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(dir, old, old); err != nil {
		t.Fatalf("age sandbox directory: %v", err)
	}

	platformErr := orphanCleanupPlatformError("freebsd")
	stats, err := cleanupOrphaned(root, time.Hour, platformErr)
	assertUnsupportedCleanupError(t, err)
	if stats.MountsDetected != 0 || stats.MountsCleanedUp != 0 ||
		stats.DirsDetected != 0 || stats.DirsCleanedUp != 0 ||
		len(stats.Errors) != 0 || stats.TotalBytesFreed != 0 {
		t.Fatalf("stats = %+v, want zero value", stats)
	}
	if data, readErr := os.ReadFile(sentinel); readErr != nil {
		t.Fatalf("sentinel was removed: %v", readErr)
	} else if string(data) != "preserve" {
		t.Fatalf("sentinel = %q, want preserve", data)
	}
}

func TestCleanupHelpersUnsupportedPlatformDoNotMutate(t *testing.T) {
	root := t.TempDir()
	sentinel := filepath.Join(root, "sentinel")
	if err := os.WriteFile(sentinel, []byte("preserve"), 0o600); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	platformErr := orphanCleanupPlatformError("freebsd")

	mountStats := cleanupOrphanedMounts([]OrphanedResource{{
		Path: root,
		Type: "mount",
	}}, platformErr)
	if mountStats.MountsDetected != 1 || mountStats.MountsCleanedUp != 0 {
		t.Fatalf("mount stats = %+v, want one detected and zero cleaned", mountStats)
	}
	if len(mountStats.Errors) != 1 {
		t.Fatalf("mount errors = %d, want 1", len(mountStats.Errors))
	}
	assertUnsupportedCleanupError(t, mountStats.Errors[0])

	dirStats := cleanupOrphanedDirectories([]OrphanedResource{{
		Path: root,
		Type: "directory",
		Size: 99,
	}}, platformErr)
	if dirStats.DirsDetected != 1 || dirStats.DirsCleanedUp != 0 || dirStats.TotalBytesFreed != 0 {
		t.Fatalf("directory stats = %+v, want one detected and zero cleanup", dirStats)
	}
	if len(dirStats.Errors) != 1 {
		t.Fatalf("directory errors = %d, want 1", len(dirStats.Errors))
	}
	assertUnsupportedCleanupError(t, dirStats.Errors[0])
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("sentinel was mutated: %v", err)
	}
	if isNotMountedError(platformErr) {
		t.Fatal("unsupported-platform error classified as already unmounted")
	}
}

func assertUnsupportedCleanupError(t *testing.T, err error) {
	t.Helper()
	var sandboxErr *Error
	if !errors.As(err, &sandboxErr) {
		t.Fatalf("error = %T %v, want *Error", err, err)
	}
	if sandboxErr.Code != ErrCodeUnsupportedPlatform {
		t.Fatalf("error code = %v, want %v", sandboxErr.Code, ErrCodeUnsupportedPlatform)
	}
	if sandboxErr.Category != CategoryPlatform {
		t.Fatalf("error category = %q, want %q", sandboxErr.Category, CategoryPlatform)
	}
}
