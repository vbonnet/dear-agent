package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func createE2ETestFixture(t *testing.T, baseDir, name string, mtime time.Time) string {
	t.Helper()
	dir := filepath.Join(baseDir, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
	bin := filepath.Join(dir, "agm")
	if err := os.WriteFile(bin, []byte("fake-agm-binary"), 0o700); err != nil {
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

func TestReapE2ECaches_ReapsExceedingMaxEntries(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Now()

	createE2ETestFixture(t, baseDir, "agm-01", now.Add(-1*time.Minute))
	createE2ETestFixture(t, baseDir, "agm-02", now.Add(-2*time.Minute))
	createE2ETestFixture(t, baseDir, "agm-03", now.Add(-3*time.Minute))
	createE2ETestFixture(t, baseDir, "agm-04", now.Add(-4*time.Minute))

	res := reapE2ECaches(e2eCacheConfig{
		Dir:        baseDir,
		MaxEntries: 2,
		MinAge:     24 * time.Hour,
		Reap:       true,
	})

	if res.Scanned != 4 {
		t.Fatalf("expected 4 scanned, got %d", res.Scanned)
	}
	if len(res.Reaped) != 2 {
		t.Fatalf("expected 2 reaped, got %d: %v", len(res.Reaped), res.Reaped)
	}

	for _, kept := range []string{"agm-01", "agm-02"} {
		if _, err := os.Stat(filepath.Join(baseDir, kept)); err != nil {
			t.Errorf("expected %s to be kept, got err: %v", kept, err)
		}
	}
	for _, evicted := range []string{"agm-03", "agm-04"} {
		if _, err := os.Stat(filepath.Join(baseDir, evicted)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be evicted, still exists", evicted)
		}
	}
}

func TestReapE2ECaches_ReapsExceedingMinAge(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Now()

	createE2ETestFixture(t, baseDir, "agm-fresh", now.Add(-1*time.Hour))
	createE2ETestFixture(t, baseDir, "agm-stale", now.Add(-30*time.Hour))

	res := reapE2ECaches(e2eCacheConfig{
		Dir:        baseDir,
		MaxEntries: 10,
		MinAge:     24 * time.Hour,
		Reap:       true,
	})

	if res.Scanned != 2 {
		t.Fatalf("expected 2 scanned, got %d", res.Scanned)
	}
	if len(res.Reaped) != 1 || !strings.HasSuffix(res.Reaped[0], "agm-stale") {
		t.Fatalf("expected agm-stale to be reaped, got: %v", res.Reaped)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "agm-fresh")); err != nil {
		t.Errorf("expected agm-fresh to be kept: %v", err)
	}
}

func TestReapE2ECaches_DryRunDoesNotDelete(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Now()

	createE2ETestFixture(t, baseDir, "agm-old", now.Add(-48*time.Hour))

	res := reapE2ECaches(e2eCacheConfig{
		Dir:        baseDir,
		MaxEntries: 10,
		MinAge:     24 * time.Hour,
		Reap:       false,
	})

	if len(res.Reaped) != 0 {
		t.Fatalf("dry-run must not reap, got %v", res.Reaped)
	}
	if res.BytesReclaimed == 0 {
		t.Fatalf("dry-run must report reclaimable bytes, got 0")
	}
	if _, err := os.Stat(filepath.Join(baseDir, "agm-old")); err != nil {
		t.Errorf("dry-run must leave fixture in place: %v", err)
	}
}

func TestReapE2ECaches_SkipsInUseFixture(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Now()

	createE2ETestFixture(t, baseDir, "agm-inuse", now.Add(-48*time.Hour))
	lockFile, err := os.OpenFile(filepath.Join(baseDir, "agm-inuse", "agm.lock"), os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer lockFile.Close()

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}

	res := reapE2ECaches(e2eCacheConfig{
		Dir:        baseDir,
		MaxEntries: 0,
		MinAge:     1 * time.Hour,
		Reap:       true,
	})

	if len(res.Reaped) != 0 {
		t.Fatalf("in-use fixture must not be reaped, got %v", res.Reaped)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "agm-inuse")); err != nil {
		t.Errorf("in-use fixture must still exist: %v", err)
	}
}

func TestReapE2ECaches_SkipsForeignContent(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Now()

	// 1. Non-agm prefix
	otherDir := filepath.Join(baseDir, "other-dir")
	if err := os.MkdirAll(otherDir, 0o700); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(otherDir, now.Add(-48*time.Hour), now.Add(-48*time.Hour))

	// 2. agm prefix with unexpected file
	unsafeDir := filepath.Join(baseDir, "agm-unsafe")
	if err := os.MkdirAll(unsafeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unsafeDir, "notes.txt"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(unsafeDir, now.Add(-48*time.Hour), now.Add(-48*time.Hour))

	res := reapE2ECaches(e2eCacheConfig{
		Dir:        baseDir,
		MaxEntries: 0,
		MinAge:     1 * time.Hour,
		Reap:       true,
	})

	if res.Scanned != 0 {
		t.Fatalf("expected 0 scanned candidates, got %d", res.Scanned)
	}
	if _, err := os.Stat(otherDir); err != nil {
		t.Errorf("foreign dir must be kept: %v", err)
	}
	if _, err := os.Stat(unsafeDir); err != nil {
		t.Errorf("unsafe dir must be kept: %v", err)
	}
}

func TestReapE2ECaches_MissingDirReturnsEmpty(t *testing.T) {
	res := reapE2ECaches(e2eCacheConfig{
		Dir:        filepath.Join(t.TempDir(), "nonexistent"),
		MaxEntries: 5,
		MinAge:     24 * time.Hour,
		Reap:       true,
	})
	if res.Scanned != 0 || len(res.Errors) != 0 {
		t.Fatalf("missing directory should produce 0 scanned and 0 errors, got: %+v", res)
	}
}

func TestRun_E2ECacheFlagsValidation(t *testing.T) {
	var out bytes.Buffer
	for _, tc := range []struct {
		flags []string
		want  string
	}{
		{
			flags: []string{"--e2e-cache-min-age", "-1h"},
			want:  "e2e-cache-min-age",
		},
		{
			flags: []string{"--e2e-cache-max-entries", "-1"},
			want:  "e2e-cache-max-entries",
		},
	} {
		out.Reset()
		code, err := run(tc.flags, &out)
		if code != 2 || err == nil {
			t.Fatalf("expected usage error (exit 2) for flags %v, got (code=%d, err=%v)", tc.flags, code, err)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("expected error message to contain %q, got: %v", tc.want, err)
		}
	}
}

func TestRun_EmptyE2ECacheDirDisablesReaper(t *testing.T) {
	var out bytes.Buffer
	code, err := run([]string{
		"--e2e-cache-dir", "",
		"--e2e-cache-min-age", "0s",
		"--gc-max-age", "0",
		"--free-warn-gb", "0.0001",
		"--dry-run",
	}, &out)
	if err != nil || code != 0 {
		t.Fatalf("empty e2e-cache-dir should disable reaper cleanly, got (code=%d, err=%v)\n%s", code, err, out.String())
	}
	if strings.Contains(out.String(), "e2e caches:") {
		t.Fatalf("disabled e2e reaper must stay silent in report:\n%s", out.String())
	}
}
