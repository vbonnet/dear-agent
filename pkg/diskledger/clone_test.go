package diskledger

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// cloneFixtureBytes is large enough that a clone's phantom allocation is far
// above filesystem free-space noise, and small enough to stay a fast test.
const cloneFixtureBytes = 64 << 20 // 64 MiB

// writeFixture creates a file of n bytes of incompressible data.
//
// Incompressible matters: APFS may transparently compress a run of zeroes,
// which would make the fixture occupy far fewer blocks than requested and
// silently invalidate every assertion below.
func writeFixture(t *testing.T, path string, n int64) {
	t.Helper()
	src, err := os.Open("/dev/urandom")
	if err != nil {
		t.Fatalf("open urandom: %v", err)
	}
	defer func() { _ = src.Close() }()

	dst, err := os.Create(path)
	if err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	defer func() { _ = dst.Close() }()

	if _, err := io.CopyN(dst, src, n); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if err := dst.Sync(); err != nil {
		t.Fatalf("sync fixture: %v", err)
	}
}

// clone makes dst a copy-on-write clone of src, reporting whether the
// filesystem supported it. A non-APFS volume (or a CI container) simply cannot
// produce the shape these tests are about, so they skip rather than assert
// something untrue about the host they happen to run on.
func clone(t *testing.T, src, dst string) bool {
	t.Helper()
	// -c requests clonefile(2) and fails rather than falling back, so a
	// success here really is a clone and not a silent full copy.
	if err := exec.Command("cp", "-c", src, dst).Run(); err != nil {
		return false
	}
	return true
}

// freeSpaceNoise estimates how much the filesystem's free space moves on its
// own, independent of anything the test does. On a host running other agents
// this is not zero, and an assertion smaller than the noise floor is a coin
// flip.
//
// It reports the MINIMUM movement across several samples rather than one
// sample. A single sample catches whatever happened to be writing in that
// instant: measured on this host, typical movement is under 4 MB per 150 ms
// but individual samples spike past 17 MB, which is enough to skip a test
// whose threshold is 16 MB. The minimum finds a quiet moment and answers the
// question actually being asked, which is whether the host can hold still long
// enough to measure a reclaim at all.
func freeSpaceNoise(path string) (int64, error) {
	const samples = 5
	best := int64(-1)
	for range samples {
		a, err := FreeBytes(path)
		if err != nil {
			return 0, err
		}
		time.Sleep(50 * time.Millisecond)
		b, err := FreeBytes(path)
		if err != nil {
			return 0, err
		}
		moved := b - a
		if moved < 0 {
			moved = -moved
		}
		if best < 0 || moved < best {
			best = moved
		}
	}
	return best, nil
}

// TestMeasureDoesNotDeduplicateAPFSClones pins a real limitation of Measure.
//
// Measure deduplicates by inode, which is correct for hardlinks. An APFS clone
// is NOT a hardlink: it is a distinct inode whose extents are shared with the
// source until written. st_blocks on that distinct inode reports the FULL
// allocation, so Measure charges a clone its whole apparent size even though
// creating it consumed no blocks.
//
// This is asserted rather than merely commented because sandboxes on this host
// are provisioned with clonefile (internal/sandbox/apfs/provider.go uses
// "cp -c"). A ledger that treats Measure's number as "bytes this run consumed"
// therefore reports a large phantom leak for every healthy sandbox.
//
// If a future change teaches Measure to detect shared extents, this test fails
// loudly and should be rewritten, not deleted.
func TestMeasureDoesNotDeduplicateAPFSClones(t *testing.T) {
	dir := t.TempDir()
	orig := filepath.Join(dir, "orig.bin")
	writeFixture(t, orig, cloneFixtureBytes)

	if !clone(t, orig, filepath.Join(dir, "clone.bin")) {
		t.Skip("filesystem does not support clonefile; nothing to pin here")
	}

	u, err := Measure(dir)
	if err != nil {
		t.Fatalf("Measure: %v", err)
	}

	// Two distinct inodes, so no dedup: the tree measures roughly twice the
	// bytes that were actually consumed.
	if u.Files != 2 {
		t.Fatalf("Files = %d, want 2 (a clone is a distinct inode, not a hardlink)", u.Files)
	}
	if u.AllocatedBytes < 2*cloneFixtureBytes {
		t.Fatalf("AllocatedBytes = %d, want >= %d. "+
			"Measure appears to have gained clone awareness; this limitation test must be rewritten, not removed.",
			u.AllocatedBytes, 2*cloneFixtureBytes)
	}
}

// TestReclaimReportsWhatTheFilesystemReturned is the point of the whole file.
//
// Deleting a clone returns almost nothing to the filesystem, because the
// extents are still referenced by the source. Measure says the clone was 64
// MiB; the disk disagrees. Reclaim asks the filesystem directly (statfs before
// and after) and therefore reports the truth that a directory walk cannot.
//
// This is the number Entry.BytesReclaimed must be built on. A collector graded
// on walked bytes looks like it is working while returning nothing.
func TestReclaimReportsWhatTheFilesystemReturned(t *testing.T) {
	dir := t.TempDir()
	orig := filepath.Join(dir, "orig.bin")
	writeFixture(t, orig, cloneFixtureBytes)

	clonePath := filepath.Join(dir, "clone.bin")
	if !clone(t, orig, clonePath) {
		t.Skip("filesystem does not support clonefile; nothing to pin here")
	}

	// A busy host moves free space on its own. Establish the noise floor
	// first and skip rather than emit a flaky failure.
	noise, err := freeSpaceNoise(dir)
	if err != nil {
		t.Fatalf("noise probe: %v", err)
	}
	if noise > cloneFixtureBytes/4 {
		t.Skipf("host free space is moving by %d bytes between samples; too noisy to assert on", noise)
	}

	walked, err := Measure(clonePath)
	if err != nil {
		t.Fatalf("Measure clone: %v", err)
	}

	freed, err := Reclaim(dir, func() error { return os.Remove(clonePath) })
	if err != nil {
		t.Fatalf("Reclaim: %v", err)
	}

	// The walk charges the clone its full size.
	if walked.AllocatedBytes < cloneFixtureBytes {
		t.Fatalf("walked clone = %d, want >= %d", walked.AllocatedBytes, cloneFixtureBytes)
	}
	// The filesystem returned essentially nothing, because orig still holds
	// the extents. Allow the noise floor; reject anything near the walked size.
	if freed > cloneFixtureBytes/2 {
		t.Fatalf("Reclaim = %d, want far below the walked %d: deleting a clone frees the shared extents only when the last reference goes",
			freed, walked.AllocatedBytes)
	}
}

// TestReclaimSeesRealDeletion is the control: when the bytes really are the
// last reference, Reclaim must report them. Without this, a Reclaim that
// always returned zero would pass the clone test above.
func TestReclaimSeesRealDeletion(t *testing.T) {
	dir := t.TempDir()
	orig := filepath.Join(dir, "orig.bin")
	writeFixture(t, orig, cloneFixtureBytes)

	noise, err := freeSpaceNoise(dir)
	if err != nil {
		t.Fatalf("noise probe: %v", err)
	}
	if noise > cloneFixtureBytes/4 {
		t.Skipf("host free space is moving by %d bytes between samples; too noisy to assert on", noise)
	}

	freed, err := Reclaim(dir, func() error { return os.Remove(orig) })
	if err != nil {
		t.Fatalf("Reclaim: %v", err)
	}
	if freed < cloneFixtureBytes/2 {
		t.Fatalf("Reclaim = %d, want roughly %d: the only reference was deleted", freed, int64(cloneFixtureBytes))
	}
}

// TestReclaimPropagatesOperationError makes sure a failed cleanup is never
// reported as a successful reclaim of zero bytes, which would let a broken
// collector look merely idle.
func TestReclaimPropagatesOperationError(t *testing.T) {
	dir := t.TempDir()
	wantErr := os.ErrPermission
	if _, err := Reclaim(dir, func() error { return wantErr }); err == nil {
		t.Fatal("Reclaim returned nil error for a failing operation")
	}
}
