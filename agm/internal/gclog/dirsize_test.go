package gclog

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDirSizeCountsFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), 4096)
	write(t, filepath.Join(dir, "sub", "b"), 4096)

	if got := DirSize(dir); got < 8192 {
		t.Errorf("DirSize = %d, want >= 8192", got)
	}
}

// BytesReclaimed feeds the reclaim-health check, which decides whether the
// collector is doing anything. A hard-linked tree counted twice reports more
// bytes reclaimed than the filesystem actually returned, which is precisely
// the direction that makes a broken collector look like it is working.
func TestDirSizeDoesNotDoubleCountHardlinks(t *testing.T) {
	dir := t.TempDir()
	orig := filepath.Join(dir, "orig")
	write(t, orig, 1<<20)
	if err := os.Link(orig, filepath.Join(dir, "link")); err != nil {
		t.Skipf("hardlink unsupported: %v", err)
	}

	got := DirSize(dir)
	// One inode, two names. Anything approaching 2 MiB means the second name
	// was charged again.
	if got > 3*(1<<20)/2 {
		t.Errorf("DirSize = %d, want ~%d (hardlink double-counted)", got, 1<<20)
	}
}

func TestDirSizeMissingPathIsZero(t *testing.T) {
	if got := DirSize(filepath.Join(t.TempDir(), "absent")); got != 0 {
		t.Errorf("DirSize(absent) = %d, want 0", got)
	}
}

// A symlink pointing outside the tree belongs to another owner. Following it
// would charge this sandbox for bytes that deleting it would not return.
func TestDirSizeDoesNotFollowSymlinks(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "big")
	write(t, outside, 1<<20)
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	if got := DirSize(dir); got >= 1<<20 {
		t.Errorf("DirSize = %d, symlink target must not be counted", got)
	}
}
