package diskledger

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestMeasureCountsRegularFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.bin"), 8192)
	writeFile(t, filepath.Join(dir, "sub", "b.bin"), 8192)

	got, err := Measure(dir)
	if err != nil {
		t.Fatalf("Measure: %v", err)
	}
	if got.Files != 2 {
		t.Errorf("Files = %d, want 2", got.Files)
	}
	if got.ApparentBytes != 16384 {
		t.Errorf("ApparentBytes = %d, want 16384", got.ApparentBytes)
	}
	// Allocated is block-rounded, so it must be at least apparent for
	// non-sparse files, and within a sane factor.
	if got.AllocatedBytes < got.ApparentBytes {
		t.Errorf("AllocatedBytes %d < ApparentBytes %d for non-sparse files",
			got.AllocatedBytes, got.ApparentBytes)
	}
}

func TestMeasureMissingPathIsZeroNotError(t *testing.T) {
	got, err := Measure(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("Measure on missing path should not error, got %v", err)
	}
	if got.AllocatedBytes != 0 || got.Files != 0 {
		t.Errorf("missing path = %+v, want zero usage", got)
	}
	if !got.Missing {
		t.Error("Missing = false, want true for absent path")
	}
}

// A hardlink is one set of blocks with two names. Counting it twice is the
// classic double-count bug that makes a naive du-based ledger overstate what
// deleting a tree would actually return.
func TestMeasureDeduplicatesHardlinks(t *testing.T) {
	dir := t.TempDir()
	orig := filepath.Join(dir, "orig.bin")
	writeFile(t, orig, 65536)
	if err := os.Link(orig, filepath.Join(dir, "link.bin")); err != nil {
		t.Skipf("hardlink unsupported: %v", err)
	}

	got, err := Measure(dir)
	if err != nil {
		t.Fatalf("Measure: %v", err)
	}
	if got.Files != 1 {
		t.Errorf("Files = %d, want 1 (hardlink must not double-count)", got.Files)
	}
	if got.AllocatedBytes > 2*65536 {
		t.Errorf("AllocatedBytes = %d, want ~65536 (hardlink double-counted)", got.AllocatedBytes)
	}
}

// APFS clonefile (reflink) is how sandboxes are provisioned. The clone shares
// blocks with its source until written. Apparent size therefore massively
// overstates what reaping the clone returns to the filesystem; this is exactly
// why the existing gclog.DirSize numbers are inflated.
func TestMeasureReportsCloneSharing(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("clonefile is APFS/darwin-specific")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFile(t, filepath.Join(src, "big.bin"), 1<<20)

	dst := filepath.Join(dir, "clone")
	if err := exec.Command("cp", "-c", "-R", src, dst).Run(); err != nil {
		t.Skipf("cp -c (clonefile) unavailable: %v", err)
	}

	cloneUsage, err := Measure(dst)
	if err != nil {
		t.Fatalf("Measure clone: %v", err)
	}
	// The clone must be measurable at all.
	if cloneUsage.Files != 1 {
		t.Fatalf("clone Files = %d, want 1", cloneUsage.Files)
	}
	// We do not assert clone blocks are zero: APFS reports st_blocks for the
	// clone as if it owned them. What we DO assert is that we surface the
	// apparent size so a caller can tell the difference rather than silently
	// trusting one number.
	if cloneUsage.ApparentBytes != 1<<20 {
		t.Errorf("clone ApparentBytes = %d, want %d", cloneUsage.ApparentBytes, 1<<20)
	}
}

func TestMeasureIgnoresSymlinkTargets(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.bin")
	writeFile(t, outside, 1<<20)
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	got, err := Measure(dir)
	if err != nil {
		t.Fatalf("Measure: %v", err)
	}
	if got.ApparentBytes >= 1<<20 {
		t.Errorf("ApparentBytes = %d, symlink target must not be followed", got.ApparentBytes)
	}
}
