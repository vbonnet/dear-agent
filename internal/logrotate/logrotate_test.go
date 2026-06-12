package logrotate

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixedClock returns a Rotator whose timestamps advance by one second per call,
// so successive rotations in a test get distinct, ordered backup names without
// relying on wall-clock timing.
func newTestRotator(p Policy, start time.Time) *Rotator {
	r := New(p)
	cur := start
	r.now = func() time.Time {
		t := cur
		cur = cur.Add(time.Second)
		return t
	}
	return r
}

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestRotateFile_OverSize(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "app.log")
	writeFile(t, log, 2048)

	r := newTestRotator(Policy{MaxSizeBytes: 1024}, time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC))
	res, err := r.RotateFile(log, false)
	if err != nil {
		t.Fatalf("RotateFile: %v", err)
	}

	if got := countKind(res, "rotate"); got != 1 {
		t.Fatalf("want 1 rotate action, got %d (%+v)", got, res.Actions)
	}
	// Live log recreated and empty.
	fi, err := os.Stat(log)
	if err != nil {
		t.Fatalf("live log missing after rotate: %v", err)
	}
	if fi.Size() != 0 {
		t.Fatalf("recreated live log not empty: %d bytes", fi.Size())
	}
	// Exactly one backup of 2048 bytes exists.
	backups, _ := backupsFor(log)
	if len(backups) != 1 {
		t.Fatalf("want 1 backup, got %d", len(backups))
	}
	if bi, _ := os.Stat(backups[0].path); bi.Size() != 2048 {
		t.Fatalf("backup lost data: %d bytes", bi.Size())
	}
}

func TestRotateFile_UnderSizeNoop(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "app.log")
	writeFile(t, log, 100)

	r := newTestRotator(Policy{MaxSizeBytes: 1024}, time.Now().UTC())
	res, err := r.RotateFile(log, false)
	if err != nil {
		t.Fatalf("RotateFile: %v", err)
	}
	if len(res.Actions) != 0 {
		t.Fatalf("expected no actions under threshold, got %+v", res.Actions)
	}
}

func TestRotate_PreservesMode(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "app.log")
	writeFile(t, log, 2048)
	if err := os.Chmod(log, 0o600); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	r := newTestRotator(Policy{MaxSizeBytes: 1024}, time.Now().UTC())
	if _, err := r.RotateFile(log, false); err != nil {
		t.Fatalf("RotateFile: %v", err)
	}
	fi, err := os.Stat(log)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("recreated log mode = %v, want 0600", fi.Mode().Perm())
	}
}

func TestCompress_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "app.log")
	const payload = "queryable log content\nsecond line\n"
	if err := os.WriteFile(log, []byte(payload), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	r := newTestRotator(Policy{MaxSizeBytes: 1, Compress: true}, time.Now().UTC())
	res, err := r.RotateFile(log, false)
	if err != nil {
		t.Fatalf("RotateFile: %v", err)
	}
	if countKind(res, "compress") != 1 {
		t.Fatalf("expected a compress action, got %+v", res.Actions)
	}

	// Find the .gz backup and confirm it decompresses to the original payload.
	entries, _ := os.ReadDir(dir)
	var gzPath string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".gz") {
			gzPath = filepath.Join(dir, e.Name())
		}
		if !strings.HasSuffix(e.Name(), ".gz") && e.Name() != "app.log" {
			t.Fatalf("uncompressed backup left behind: %s", e.Name())
		}
	}
	if gzPath == "" {
		t.Fatal("no .gz backup produced")
	}
	got := gunzip(t, gzPath)
	if got != payload {
		t.Fatalf("gz round-trip mismatch: got %q want %q", got, payload)
	}
}

func TestPrune_MaxBackups(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "app.log")

	r := newTestRotator(Policy{MaxSizeBytes: 1, MaxBackups: 2}, time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC))
	// Rotate five times; each rotation creates one backup, then prune keeps 2.
	for i := range 5 {
		writeFile(t, log, 10)
		if _, err := r.RotateFile(log, false); err != nil {
			t.Fatalf("rotate %d: %v", i, err)
		}
	}
	backups, _ := backupsFor(log)
	if len(backups) != 2 {
		t.Fatalf("MaxBackups=2 not enforced: %d backups remain", len(backups))
	}
}

func TestPrune_MaxAge(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "app.log")
	writeFile(t, log, 10)

	// Pre-create an old backup well beyond the age cutoff.
	old := log + ".20200101T000000Z"
	if err := os.WriteFile(old, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old: %v", err)
	}
	oldTime := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(old, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	r := New(Policy{MaxAge: 24 * time.Hour})
	res, err := r.RotateFile(log, false)
	if err != nil {
		t.Fatalf("RotateFile: %v", err)
	}
	if countKind(res, "prune") != 1 {
		t.Fatalf("expected old backup pruned, got %+v", res.Actions)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old backup still present")
	}
}

func TestDryRun_NoWrites(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "app.log")
	writeFile(t, log, 4096)

	r := newTestRotator(Policy{MaxSizeBytes: 1024, Compress: true}, time.Now().UTC())
	res, err := r.RotateFile(log, true)
	if err != nil {
		t.Fatalf("RotateFile dry-run: %v", err)
	}
	if !res.DryRun || len(res.Actions) == 0 {
		t.Fatalf("dry-run should report intended actions, got %+v", res)
	}
	// Nothing changed on disk: original file intact, no backups.
	fi, _ := os.Stat(log)
	if fi.Size() != 4096 {
		t.Fatalf("dry-run mutated live log: %d bytes", fi.Size())
	}
	if backups, _ := backupsFor(log); len(backups) != 0 {
		t.Fatalf("dry-run created %d backups", len(backups))
	}
}

func TestRotateDir_SkipsBackupsAndRecurses(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.log"), 2048)
	writeFile(t, filepath.Join(dir, "sub", "b.log"), 2048)
	// An already-rotated file must not be treated as a live log.
	writeFile(t, filepath.Join(dir, "a.log.20260101T000000Z"), 2048)

	r := newTestRotator(Policy{MaxSizeBytes: 1024}, time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC))

	// Non-recursive: only top-level a.log rotates.
	res, err := r.RotateDir(dir, false, false)
	if err != nil {
		t.Fatalf("RotateDir: %v", err)
	}
	if got := countKind(res, "rotate"); got != 1 {
		t.Fatalf("non-recursive want 1 rotate, got %d (%+v)", got, res.Actions)
	}

	// Recursive: sub/b.log now rotates too.
	res, err = r.RotateDir(dir, true, false)
	if err != nil {
		t.Fatalf("RotateDir recursive: %v", err)
	}
	if got := countKind(res, "rotate"); got != 1 {
		t.Fatalf("recursive want 1 rotate for sub/b.log, got %d (%+v)", got, res.Actions)
	}
}

func TestRotateDir_ErrorsOnNonDir(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file")
	writeFile(t, f, 10)
	r := New(Policy{MaxSizeBytes: 1})
	if _, err := r.RotateDir(f, false, false); err == nil {
		t.Fatal("expected error rotating a non-directory")
	}
}

func TestIsRotated(t *testing.T) {
	cases := map[string]bool{
		"app.log":                     false,
		"app.log.20260612T100000Z":    true,
		"app.log.20260612T100000Z.gz": true,
		"app.log.20260612T100000Z-1":  true,
		"messages.jsonl":              false,
		"audit.jsonl.notatimestamp":   false,
	}
	for name, want := range cases {
		if got := isRotated(name); got != want {
			t.Errorf("isRotated(%q) = %v, want %v", name, got, want)
		}
	}
}

func countKind(res Result, kind string) int {
	n := 0
	for _, a := range res.Actions {
		if a.Kind == kind {
			n++
		}
	}
	return n
}

func gunzip(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open gz: %v", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gz.Close()
	b, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("read gz: %v", err)
	}
	return string(b)
}
