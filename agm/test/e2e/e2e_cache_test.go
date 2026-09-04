package e2e

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func createTestFixtureDir(t *testing.T, baseDir, name string, mtime time.Time) string {
	t.Helper()
	dir := filepath.Join(baseDir, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
	bin := filepath.Join(dir, "agm")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write agm in %s: %v", name, err)
	}
	lock := filepath.Join(dir, "agm.lock")
	if err := os.WriteFile(lock, []byte{}, 0o600); err != nil {
		t.Fatalf("write agm.lock in %s: %v", name, err)
	}
	if !mtime.IsZero() {
		if err := os.Chtimes(dir, mtime, mtime); err != nil {
			t.Fatalf("chtimes %s: %v", name, err)
		}
	}
	return dir
}

func TestPruneE2EBuildCache_EnforcesMaxEntries(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Now()

	// Create 5 fixture directories with decreasing mtimes (01 is newest, 05 is oldest).
	createTestFixtureDir(t, baseDir, "agm-01", now.Add(-1*time.Minute))
	createTestFixtureDir(t, baseDir, "agm-02", now.Add(-2*time.Minute))
	createTestFixtureDir(t, baseDir, "agm-03", now.Add(-3*time.Minute))
	createTestFixtureDir(t, baseDir, "agm-04", now.Add(-4*time.Minute))
	createTestFixtureDir(t, baseDir, "agm-05", now.Add(-5*time.Minute))

	// Keep at most 3 entries.
	if err := pruneE2EBuildCache(baseDir, "", 3, 24*time.Hour); err != nil {
		t.Fatalf("prune failed: %v", err)
	}

	for _, kept := range []string{"agm-01", "agm-02", "agm-03"} {
		if _, err := os.Stat(filepath.Join(baseDir, kept)); err != nil {
			t.Errorf("expected %s to be kept, but got error: %v", kept, err)
		}
	}
	for _, evicted := range []string{"agm-04", "agm-05"} {
		if _, err := os.Stat(filepath.Join(baseDir, evicted)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be evicted, but still exists", evicted)
		}
	}
}

func TestPruneE2EBuildCache_EnforcesMaxAge(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Now()

	// Create 2 fresh entries and 2 expired entries (>24h).
	createTestFixtureDir(t, baseDir, "agm-fresh1", now.Add(-1*time.Hour))
	createTestFixtureDir(t, baseDir, "agm-fresh2", now.Add(-2*time.Hour))
	createTestFixtureDir(t, baseDir, "agm-stale1", now.Add(-25*time.Hour))
	createTestFixtureDir(t, baseDir, "agm-stale2", now.Add(-48*time.Hour))

	// Bound max entries high (10), so only age drives eviction.
	if err := pruneE2EBuildCache(baseDir, "", 10, 24*time.Hour); err != nil {
		t.Fatalf("prune failed: %v", err)
	}

	for _, kept := range []string{"agm-fresh1", "agm-fresh2"} {
		if _, err := os.Stat(filepath.Join(baseDir, kept)); err != nil {
			t.Errorf("expected %s to be kept, but got error: %v", kept, err)
		}
	}
	for _, evicted := range []string{"agm-stale1", "agm-stale2"} {
		if _, err := os.Stat(filepath.Join(baseDir, evicted)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be evicted, but still exists", evicted)
		}
	}
}

func TestPruneE2EBuildCache_PreservesCurrentDir(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Now()

	// agm-current is older and would otherwise be evicted when maxEntries=1.
	createTestFixtureDir(t, baseDir, "agm-newer", now.Add(-1*time.Minute))
	createTestFixtureDir(t, baseDir, "agm-current", now.Add(-60*time.Minute))

	if err := pruneE2EBuildCache(baseDir, "agm-current", 1, 24*time.Hour); err != nil {
		t.Fatalf("prune failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(baseDir, "agm-current")); err != nil {
		t.Errorf("current active dir must not be evicted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "agm-newer")); !os.IsNotExist(err) {
		t.Errorf("expected agm-newer to be evicted since agm-current counts toward the 1-entry bound")
	}
}

func TestPruneE2EBuildCache_EnforcesZeroMaxEntries(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Now()

	createTestFixtureDir(t, baseDir, "agm-01", now.Add(-1*time.Minute))
	createTestFixtureDir(t, baseDir, "agm-02", now.Add(-2*time.Minute))

	if err := pruneE2EBuildCache(baseDir, "", 0, 24*time.Hour); err != nil {
		t.Fatalf("prune failed: %v", err)
	}

	for _, evicted := range []string{"agm-01", "agm-02"} {
		if _, err := os.Stat(filepath.Join(baseDir, evicted)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be evicted, but still exists", evicted)
		}
	}
}

func TestPruneE2EBuildCache_SkipsLockedDirectory(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Now()

	createTestFixtureDir(t, baseDir, "agm-locked", now.Add(-10*time.Hour))
	lockFile, err := os.OpenFile(filepath.Join(baseDir, "agm-locked", "agm.lock"), os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("open lock file: %v", err)
	}
	defer lockFile.Close()

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("flock: %v", err)
	}

	// Should be evicted by maxEntries=0, but flock protects it.
	if err := pruneE2EBuildCache(baseDir, "", 0, 1*time.Hour); err != nil {
		t.Fatalf("prune failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(baseDir, "agm-locked")); err != nil {
		t.Errorf("locked dir must not be evicted: %v", err)
	}
}

func TestPruneE2EBuildCache_SkipsForeignDirectoryOrFiles(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Now()

	// 1. Non-agm directory prefix.
	foreignDir := filepath.Join(baseDir, "other-cache")
	if err := os.MkdirAll(foreignDir, 0o700); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(foreignDir, now.Add(-48*time.Hour), now.Add(-48*time.Hour))

	// 2. agm directory containing foreign files.
	unsafeDir := filepath.Join(baseDir, "agm-unsafe")
	if err := os.MkdirAll(unsafeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unsafeDir, "notes.txt"), []byte("foreign file"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(unsafeDir, now.Add(-48*time.Hour), now.Add(-48*time.Hour))

	if err := pruneE2EBuildCache(baseDir, "", 0, 1*time.Hour); err != nil {
		t.Fatalf("prune failed: %v", err)
	}

	if _, err := os.Stat(foreignDir); err != nil {
		t.Errorf("foreign directory must not be touched: %v", err)
	}
	if _, err := os.Stat(unsafeDir); err != nil {
		t.Errorf("directory with foreign file must not be touched: %v", err)
	}
}

func TestTouchE2ECacheDir(t *testing.T) {
	baseDir := t.TempDir()
	past := time.Now().Add(-10 * time.Hour)
	dir := createTestFixtureDir(t, baseDir, "agm-touch", past)
	dest := filepath.Join(dir, "agm")

	touchE2ECacheDir(dir, dest)

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(info.ModTime()) > 5*time.Second {
		t.Errorf("expected dir modTime to be refreshed, got %v", info.ModTime())
	}

	destInfo, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(destInfo.ModTime()) > 5*time.Second {
		t.Errorf("expected dest modTime to be refreshed, got %v", destInfo.ModTime())
	}
}
