package binstamp

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// A daemon whose binary nobody touched must keep running. This is the case
// that fires on every poll, so a false positive here would restart the
// daemon in a loop.
func TestWatcherReportsNoReplacementWhenBinaryIsUntouched(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "agm")
	write(t, bin, "build-one")

	stamp, err := Of(bin)
	if err != nil {
		t.Fatalf("Of: %v", err)
	}
	w := &Watcher{path: bin, baseline: stamp, ok: true}

	if w.Replaced() {
		t.Fatal("untouched binary reported as replaced")
	}
}

// The regression this package exists for: `go install` writes a temporary
// file and renames it over the target. The path still resolves, so a
// daemon that only checked for existence would keep serving the old image
// forever — which is exactly what watch-stalled did for eight days.
func TestWatcherDetectsRenameOverInstall(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "agm")
	write(t, bin, "build-one")

	stamp, err := Of(bin)
	if err != nil {
		t.Fatalf("Of: %v", err)
	}
	w := &Watcher{path: bin, baseline: stamp, ok: true}

	staged := filepath.Join(dir, "agm.tmp")
	write(t, staged, "build-two-is-longer")
	if err := os.Rename(staged, bin); err != nil {
		t.Fatalf("rename: %v", err)
	}

	if !w.Replaced() {
		t.Fatal("rename-over install not detected as a replacement")
	}
}

// An in-place rewrite that happens to preserve size keeps the inode, so
// only the modification time moves. Comparing inode alone would miss it.
func TestWatcherDetectsInPlaceRewriteOfSameSize(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "agm")
	write(t, bin, "build-one")

	stamp, err := Of(bin)
	if err != nil {
		t.Fatalf("Of: %v", err)
	}
	w := &Watcher{path: bin, baseline: stamp, ok: true}

	// Same byte length, different content and a later mtime.
	write(t, bin, "build-two")
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(bin, later, later); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	if !w.Replaced() {
		t.Fatal("same-size in-place rewrite not detected as a replacement")
	}
}

// Installs are not atomic from the observer's side. A binary that is
// momentarily absent must not be read as a replacement: restarting then
// would exec a path that does not exist yet.
func TestWatcherTreatsMissingBinaryAsNotReplaced(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "agm")
	write(t, bin, "build-one")

	stamp, err := Of(bin)
	if err != nil {
		t.Fatalf("Of: %v", err)
	}
	w := &Watcher{path: bin, baseline: stamp, ok: true}

	if err := os.Remove(bin); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if w.Replaced() {
		t.Fatal("momentarily absent binary reported as replaced")
	}

	// And once the new build lands, it is detected.
	write(t, bin, "build-two-is-longer")
	if !w.Replaced() {
		t.Fatal("replacement after a gap not detected")
	}
}

// A watcher that could not stamp its own executable must stay quiet rather
// than report a replacement on every poll.
func TestWatcherWithoutBaselineNeverReportsReplacement(t *testing.T) {
	w := &Watcher{path: "/nonexistent/agm", ok: false}
	if w.Replaced() {
		t.Fatal("watcher without a baseline reported a replacement")
	}
}

// The running process must be stampable, or the daemon can never notice
// its own redeploy.
func TestOfRunningStampsTheTestBinary(t *testing.T) {
	stamp, path, err := OfRunning()
	if err != nil {
		t.Fatalf("OfRunning: %v", err)
	}
	if path == "" {
		t.Fatal("empty executable path")
	}
	if stamp.Size == 0 {
		t.Fatal("zero-size stamp for the running test binary")
	}
}
