package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// cacheEntryName is a content-addressed cache filename of the shape Go and
// golangci-lint write inside each shard.
const cacheEntryName = "0123456789abcdef0123456789abcdef0123456789abcdef-d"

// mkBuildCache writes a directory that is structurally a Go build cache.
//
// Deliberately writes NO trim.txt or README: on the real host 42 of 43
// abandoned caches carried neither, so a fixture that always has them would
// hide exactly the caches the reaper exists to remove.
func mkBuildCache(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := range 256 {
		shard := filepath.Join(dir, fmt.Sprintf("%02x", i))
		if err := os.MkdirAll(shard, 0o755); err != nil {
			t.Fatal(err)
		}
		// A cached artifact in each shard, so size accounting has something
		// to count and the content check has something to inspect.
		if err := os.WriteFile(filepath.Join(shard, cacheEntryName), []byte("0123456789"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func aged(t *testing.T, dir string, age time.Duration) {
	t.Helper()
	when := time.Now().Add(-age)
	if err := os.Chtimes(dir, when, when); err != nil {
		t.Fatal(err)
	}
}

func idle(string) (bool, error) { return false, nil }

// TestIsGoBuildCacheRoot_ProofIsByContentNotName is the safety core: the
// reaper deletes directories, so it must accept only what it can prove and
// must never be steered by a name.
func TestIsGoBuildCacheRoot_ProofIsByContentNotName(t *testing.T) {
	base := t.TempDir()

	t.Run("a real cache under an unrelated name is recognised", func(t *testing.T) {
		if !isGoBuildCacheRoot(mkBuildCache(t, filepath.Join(base, "totally-unrelated-name"))) {
			t.Fatal("a structurally valid build cache must be recognised regardless of its name")
		}
	})

	t.Run("a directory named like a cache but holding real work is refused", func(t *testing.T) {
		decoy := mkBuildCache(t, filepath.Join(base, "my-gocache"))
		// One top-level file that is not build-cache furniture disqualifies it.
		if err := os.WriteFile(filepath.Join(decoy, "important.go"), []byte("package x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if isGoBuildCacheRoot(decoy) {
			t.Fatal("a directory holding non-furniture content must never be reapable, whatever it is called")
		}
	})

	t.Run("a cache with no marker file is still recognised", func(t *testing.T) {
		// The host's dominant shape: 256 shards, no trim.txt, no README.
		// Requiring a marker file would have missed 42 of 43 real caches.
		d := mkBuildCache(t, filepath.Join(base, "no-marker"))
		if _, err := os.Stat(filepath.Join(d, "trim.txt")); !os.IsNotExist(err) {
			t.Fatalf("fixture should have no trim.txt, stat err = %v", err)
		}
		if !isGoBuildCacheRoot(d) {
			t.Fatal("a marker-less cache must still be recognised")
		}
	})

	t.Run("an emptied cache skeleton is recognised", func(t *testing.T) {
		// What an external cleanup leaves behind: shards with nothing in them.
		d := mkBuildCache(t, filepath.Join(base, "skeleton"))
		for i := range 256 {
			shard := filepath.Join(d, fmt.Sprintf("%02x", i))
			if err := os.Remove(filepath.Join(shard, cacheEntryName)); err != nil {
				t.Fatal(err)
			}
		}
		if !isGoBuildCacheRoot(d) {
			t.Fatal("an emptied cache skeleton is still a cache and still reclaimable")
		}
	})

	t.Run("hex-named directories holding real work are refused", func(t *testing.T) {
		// The dangerous near-miss: the shape is right, the contents are not.
		d := mkBuildCache(t, filepath.Join(base, "sharded-but-real"))
		if err := os.WriteFile(filepath.Join(d, "00", "notes.md"), []byte("real work"), 0o600); err != nil {
			t.Fatal(err)
		}
		if isGoBuildCacheRoot(d) {
			t.Fatal("a shard holding a non-cache file must disqualify the whole directory")
		}
	})

	t.Run("too few shards is refused", func(t *testing.T) {
		d := filepath.Join(base, "coincidence")
		for _, n := range []string{"ab", "cd"} {
			if err := os.MkdirAll(filepath.Join(d, n), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if isGoBuildCacheRoot(d) {
			t.Fatal("two hex-named directories must not be mistaken for a sharded cache")
		}
	})

	t.Run("a symlink in the root disqualifies it", func(t *testing.T) {
		d := mkBuildCache(t, filepath.Join(base, "with-symlink"))
		if err := os.Symlink(base, filepath.Join(d, "escape")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if isGoBuildCacheRoot(d) {
			t.Fatal("a symlink in the cache root must disqualify it")
		}
	})

	t.Run("a real repository checkout is refused", func(t *testing.T) {
		repo := filepath.Join(base, "a-repo")
		if err := os.MkdirAll(filepath.Join(repo, "cmd"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if isGoBuildCacheRoot(repo) {
			t.Fatal("an ordinary source tree must never be reapable")
		}
	})
}

// TestFindBuildCacheRoots_NestedAndBounded covers the host's two real shapes:
// a cache directly under /tmp, and a cache one level down inside a per-task
// scratch directory.
func TestFindBuildCacheRoots_NestedAndBounded(t *testing.T) {
	base := t.TempDir()
	top := mkBuildCache(t, filepath.Join(base, "top-level-gocache"))
	nested := mkBuildCache(t, filepath.Join(base, "task-scratch", "cache-darwin"))
	// Deeper than maxDepth ⇒ must not be found.
	tooDeep := mkBuildCache(t, filepath.Join(base, "a", "b", "c", "deep-cache"))

	got, err := findBuildCacheRoots(base, 2)
	if err != nil {
		t.Fatalf("findBuildCacheRoots: %v", err)
	}
	found := map[string]bool{}
	for _, g := range got {
		resolved, _ := filepath.EvalSymlinks(g)
		found[resolved] = true
	}
	for _, want := range []string{top, nested} {
		resolved, _ := filepath.EvalSymlinks(want)
		if !found[resolved] {
			t.Fatalf("expected to find %q; got %v", want, got)
		}
	}
	resolvedDeep, _ := filepath.EvalSymlinks(tooDeep)
	if found[resolvedDeep] {
		t.Fatalf("depth bound not honoured: %q should be out of range", tooDeep)
	}
	// A cache's own shards must never be reported as separate candidates.
	for _, g := range got {
		if filepath.Base(g) == "ab" {
			t.Fatalf("descended into a proven cache's shards: %q", g)
		}
	}
}

// TestReapBuildCaches_GatesAreFailClosed asserts every reason a cache is kept.
func TestReapBuildCaches_GatesAreFailClosed(t *testing.T) {
	newPass := func(t *testing.T) (string, map[string]string) {
		base := t.TempDir()
		old := mkBuildCache(t, filepath.Join(base, "old-cache"))
		fresh := mkBuildCache(t, filepath.Join(base, "fresh-cache"))
		aged(t, old, 48*time.Hour)
		aged(t, fresh, 5*time.Minute)
		return base, map[string]string{"old": old, "fresh": fresh}
	}

	t.Run("old and idle is reaped; recent is kept", func(t *testing.T) {
		base, dirs := newPass(t)
		res := reapBuildCaches(buildCacheConfig{
			Roots: []string{base}, MaxDepth: 2, MinAge: 24 * time.Hour, Reap: true,
			inUse: idle,
		})
		if len(res.Reaped) != 1 {
			t.Fatalf("expected exactly the aged cache reaped, got %v (skipped %v)", res.Reaped, res.Skipped)
		}
		if _, err := os.Stat(dirs["old"]); !os.IsNotExist(err) {
			t.Fatalf("aged cache should be gone, stat err = %v", err)
		}
		if _, err := os.Stat(dirs["fresh"]); err != nil {
			t.Fatalf("recent cache must survive: %v", err)
		}
		if res.BytesReclaimed <= 0 {
			t.Fatalf("reclaimed bytes should be counted, got %d", res.BytesReclaimed)
		}
	})

	t.Run("a cache with a process inside is never reaped", func(t *testing.T) {
		base, dirs := newPass(t)
		res := reapBuildCaches(buildCacheConfig{
			Roots: []string{base}, MaxDepth: 2, MinAge: 24 * time.Hour, Reap: true,
			inUse: func(string) (bool, error) { return true, nil },
		})
		if len(res.Reaped) != 0 {
			t.Fatalf("a busy cache must never be reaped, got %v", res.Reaped)
		}
		if _, err := os.Stat(dirs["old"]); err != nil {
			t.Fatalf("busy cache must survive: %v", err)
		}
	})

	t.Run("an unprovable liveness probe keeps the cache", func(t *testing.T) {
		base, dirs := newPass(t)
		res := reapBuildCaches(buildCacheConfig{
			Roots: []string{base}, MaxDepth: 2, MinAge: 24 * time.Hour, Reap: true,
			inUse: func(string) (bool, error) { return false, errors.New("lsof timed out") },
		})
		if len(res.Reaped) != 0 {
			t.Fatalf("could-not-prove-idle must mean keep, got %v", res.Reaped)
		}
		if _, err := os.Stat(dirs["old"]); err != nil {
			t.Fatalf("cache must survive an unprovable probe: %v", err)
		}
	})

	t.Run("scan-only removes nothing but still reports what it would free", func(t *testing.T) {
		base, dirs := newPass(t)
		res := reapBuildCaches(buildCacheConfig{
			Roots: []string{base}, MaxDepth: 2, MinAge: 24 * time.Hour, Reap: false,
			inUse: idle,
		})
		if len(res.Reaped) != 0 {
			t.Fatalf("scan-only must remove nothing, got %v", res.Reaped)
		}
		if _, err := os.Stat(dirs["old"]); err != nil {
			t.Fatalf("scan-only must not delete: %v", err)
		}
		if res.BytesReclaimed <= 0 {
			t.Fatal("scan-only must still report the reclaimable bytes")
		}
	})

	t.Run("every kept cache says why", func(t *testing.T) {
		base, _ := newPass(t)
		res := reapBuildCaches(buildCacheConfig{
			Roots: []string{base}, MaxDepth: 2, MinAge: 24 * time.Hour, Reap: true,
			inUse: idle,
		})
		if len(res.Skipped) == 0 {
			t.Fatal("a kept cache must carry a reason; a silent zero-reclaim pass is indistinguishable from a clean host")
		}
	})
}

// TestReapBuildCaches_MissingRootIsNotAFailure keeps a watchdog tick healthy
// on a host where a configured scan root does not exist.
func TestReapBuildCaches_MissingRootIsNotAFailure(t *testing.T) {
	res := reapBuildCaches(buildCacheConfig{
		Roots: []string{filepath.Join(t.TempDir(), "nope")}, MaxDepth: 2,
		MinAge: time.Hour, Reap: true, inUse: idle,
	})
	if len(res.Errors) != 0 || res.Scanned != 0 || len(res.Reaped) != 0 {
		t.Fatalf("a missing scan root must be an empty result, got %+v", res)
	}
}

// TestReapBuildCaches_SymlinkCannotRedirectTheReaper guards the obvious
// escape: a link planted in a scanned directory must not make the reaper
// delete its target.
func TestReapBuildCaches_SymlinkCannotRedirectTheReaper(t *testing.T) {
	base := t.TempDir()
	outside := mkBuildCache(t, filepath.Join(t.TempDir(), "outside-cache"))
	aged(t, outside, 48*time.Hour)
	if err := os.Symlink(outside, filepath.Join(base, "link-to-cache")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	res := reapBuildCaches(buildCacheConfig{
		Roots: []string{base}, MaxDepth: 2, MinAge: 24 * time.Hour, Reap: true, inUse: idle,
	})
	if len(res.Reaped) != 0 {
		t.Fatalf("a symlinked cache must not be followed or reaped, got %v", res.Reaped)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("the symlink target must be untouched: %v", err)
	}
}

// TestRun_RejectsNonPositiveBuildCacheAge covers DW-37: a zero or negative
// age gate would make a cache an in-flight build is writing right now
// immediately eligible, so it is a usage error rather than a silent disable.
func TestRun_RejectsNonPositiveBuildCacheAge(t *testing.T) {
	for _, age := range []string{"0s", "-1h"} {
		var out bytes.Buffer
		code, err := run([]string{"--build-cache-min-age", age}, &out)
		if code != 2 || err == nil {
			t.Fatalf("--build-cache-min-age %s: got (code=%d, err=%v), want a usage error with exit 2", age, code, err)
		}
		if !strings.Contains(err.Error(), "build-cache-min-age") {
			t.Fatalf("error should name the offending flag, got %v", err)
		}
	}
}

// TestRun_EmptyRootsDisablesTheReaper covers the documented off switch: with
// no scan roots the age gate is not validated and no build-cache line is
// reported at all.
func TestRun_EmptyRootsDisablesTheReaper(t *testing.T) {
	var out bytes.Buffer
	code, err := run([]string{"--build-cache-roots", "", "--build-cache-min-age", "0s", "--gc-max-age", "0", "--dry-run"}, &out)
	if err != nil || code != 0 {
		t.Fatalf("empty roots should disable the reaper cleanly, got (code=%d, err=%v)\n%s", code, err, out.String())
	}
	if strings.Contains(out.String(), "build caches:") {
		t.Fatalf("a disabled reaper must stay silent in the report:\n%s", out.String())
	}
}
