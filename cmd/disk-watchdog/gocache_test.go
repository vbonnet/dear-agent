package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The canonical Go build cache is a different problem from an abandoned one.
//
// On 2026-09-04 the host hit 97% used with 12 GiB free and writes failing. The
// dominant consumer was ~/Library/Caches/go-build at 48 GB, grown by a Codex
// desktop re-run of `make test-bdd`. The watchdog's existing reaper never saw
// it: that reaper scans /tmp and $TMPDIR for *abandoned* caches, and the
// canonical cache is neither under those roots nor abandoned: every build
// touches it, so a 24h age gate can never fire on it. A human ran
// `go clean -cache` and reclaimed ~44 GiB.
//
// So the gate here is size, not age, and the trim runs only under disk
// pressure: a warm cache is worth keeping until it is the reason writes fail.

// cacheBytes sums the regular files under dir, the same accounting the trim
// reports as reclaimed.
func cacheBytes(t *testing.T, dir string) int64 {
	t.Helper()
	var total int64
	if err := filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, ierr := d.Info()
		if ierr == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return total
}

func countShards(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() && hexShard.MatchString(e.Name()) {
			n++
		}
	}
	return n
}

// TestTrimCanonicalCaches_ReclaimsOverBudgetCache is the headline receipt for
// defect 1: an over-budget canonical cache is emptied and the reclaimed bytes
// are reported, which is what makes disk pressure resolve without a human.
func TestTrimCanonicalCaches_ReclaimsOverBudgetCache(t *testing.T) {
	dir := mkBuildCache(t, filepath.Join(t.TempDir(), "go-build"))
	before := cacheBytes(t, dir)
	if before == 0 {
		t.Fatal("fixture wrote no bytes")
	}

	res := trimCanonicalCaches(canonicalCacheConfig{
		Dirs:     []string{dir},
		MaxBytes: before - 1, // over budget by one byte
		Trim:     true,
	})

	if len(res.Trimmed) != 1 || res.Trimmed[0] != dir {
		t.Fatalf("Trimmed = %v, want [%s]; skipped=%v errors=%v", res.Trimmed, dir, res.Skipped, res.Errors)
	}
	if res.BytesReclaimed != before {
		t.Fatalf("BytesReclaimed = %d, want %d", res.BytesReclaimed, before)
	}
	if n := countShards(t, dir); n != 0 {
		t.Fatalf("%d shard(s) survived the trim, want 0", n)
	}
	// The root itself must survive: Go recreates shards on demand but an
	// absent GOCACHE root is a different failure for a concurrent build.
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("cache root must survive the trim: %v", err)
	}
}

// TestTrimCanonicalCaches_KeepsCacheWithinBudget: a warm cache under budget is
// worth its disk. Trimming it on every breach would throw away build time for
// no reclaim.
func TestTrimCanonicalCaches_KeepsCacheWithinBudget(t *testing.T) {
	dir := mkBuildCache(t, filepath.Join(t.TempDir(), "go-build"))
	before := cacheBytes(t, dir)

	res := trimCanonicalCaches(canonicalCacheConfig{
		Dirs:     []string{dir},
		MaxBytes: before + 1,
		Trim:     true,
	})

	if len(res.Trimmed) != 0 || res.BytesReclaimed != 0 {
		t.Fatalf("within-budget cache was trimmed: %+v", res)
	}
	if !strings.Contains(res.Skipped[dir], "within budget") {
		t.Fatalf("Skipped[%s] = %q, want a within-budget reason", dir, res.Skipped[dir])
	}
	if n := countShards(t, dir); n != 256 {
		t.Fatalf("%d shards survived, want all 256 untouched", n)
	}
}

// TestTrimCanonicalCaches_RefusesUnprovenDirectory is the safety core. The
// trim deletes directories, so it must accept only what it can prove is a
// content-addressed cache, never a path because it was named as one. A
// mis-set GOCACHE pointing at a worktree must reclaim nothing.
func TestTrimCanonicalCaches_RefusesUnprovenDirectory(t *testing.T) {
	root := t.TempDir()

	// A source tree that someone pointed GOCACHE at by mistake.
	worktree := filepath.Join(root, "worktree")
	if err := os.MkdirAll(filepath.Join(worktree, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "pkg", "main.go"), []byte("package main"), 0o600); err != nil {
		t.Fatal(err)
	}

	// A cache-shaped directory that hides one foreign file among its shards.
	poisoned := mkBuildCache(t, filepath.Join(root, "poisoned"))
	if err := os.WriteFile(filepath.Join(poisoned, "00", "NOTES.md"), []byte("real work"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{worktree, poisoned} {
		res := trimCanonicalCaches(canonicalCacheConfig{
			Dirs:     []string{dir},
			MaxBytes: 0, // always over budget; only the structural proof can save it
			Trim:     true,
		})
		if len(res.Trimmed) != 0 || res.BytesReclaimed != 0 {
			t.Fatalf("%s was trimmed without proof: %+v", dir, res)
		}
		if !strings.Contains(res.Skipped[dir], "not a proven") {
			t.Fatalf("Skipped[%s] = %q, want an unproven-directory reason", dir, res.Skipped[dir])
		}
	}
	if _, err := os.Stat(filepath.Join(worktree, "pkg", "main.go")); err != nil {
		t.Fatalf("source file must be untouched: %v", err)
	}
	if _, err := os.Stat(filepath.Join(poisoned, "00", "NOTES.md")); err != nil {
		t.Fatalf("foreign file must be untouched: %v", err)
	}
}

// TestTrimCanonicalCaches_DryRunDeletesNothing: --dry-run must still report
// what a real tick would reclaim, so an operator can size the budget without
// spending the cache.
func TestTrimCanonicalCaches_DryRunDeletesNothing(t *testing.T) {
	dir := mkBuildCache(t, filepath.Join(t.TempDir(), "go-build"))
	before := cacheBytes(t, dir)

	res := trimCanonicalCaches(canonicalCacheConfig{
		Dirs:     []string{dir},
		MaxBytes: 0,
		Trim:     false,
	})

	if res.BytesReclaimed != 0 || len(res.Trimmed) != 0 {
		t.Fatalf("dry run deleted something: %+v", res)
	}
	if res.BytesReclaimable != before {
		t.Fatalf("BytesReclaimable = %d, want %d", res.BytesReclaimable, before)
	}
	if n := countShards(t, dir); n != 256 {
		t.Fatalf("dry run removed %d shard(s)", 256-n)
	}
}

// TestTrimCanonicalCaches_ShardRemovalFailureIsReported: a shard that cannot
// be removed must surface as an error rather than be counted as reclaimed.
// Over-reporting reclaim is how a full disk gets declared healthy.
func TestTrimCanonicalCaches_ShardRemovalFailureIsReported(t *testing.T) {
	dir := mkBuildCache(t, filepath.Join(t.TempDir(), "go-build"))

	res := trimCanonicalCaches(canonicalCacheConfig{
		Dirs:     []string{dir},
		MaxBytes: 0,
		Trim:     true,
		remove:   func(string) error { return errors.New("read-only file system") },
	})

	if res.BytesReclaimed != 0 {
		t.Fatalf("BytesReclaimed = %d after every removal failed, want 0", res.BytesReclaimed)
	}
	if len(res.Errors) != 256 {
		t.Fatalf("len(Errors) = %d, want one per failed shard (256)", len(res.Errors))
	}
}

// TestTrimOversizedCanonicalCaches_OnlyUnderDiskPressure: measuring a 48 GB
// cache means walking hundreds of thousands of files. That cost is justified
// when writes are failing and pointless every five minutes on a healthy host.
func TestTrimOversizedCanonicalCaches_OnlyUnderDiskPressure(t *testing.T) {
	dir := mkBuildCache(t, filepath.Join(t.TempDir(), "go-build"))
	cfg := config{goCacheDirs: dir, goCacheMaxBytes: 0}

	if got := trimOversizedCanonicalCaches(cfg, false); got != nil {
		t.Fatalf("unbreached tick must not walk the cache, got %+v", got)
	}
	if n := countShards(t, dir); n != 256 {
		t.Fatalf("unbreached tick removed %d shard(s)", 256-n)
	}

	got := trimOversizedCanonicalCaches(cfg, true)
	if got == nil || len(got.Trimmed) != 1 {
		t.Fatalf("breached tick must trim the over-budget cache, got %+v", got)
	}
}

// TestTrimOversizedCanonicalCaches_EmptyDirsDisables covers the documented off
// switch.
func TestTrimOversizedCanonicalCaches_EmptyDirsDisables(t *testing.T) {
	if got := trimOversizedCanonicalCaches(config{goCacheDirs: ""}, true); got != nil {
		t.Fatalf("empty --go-cache-dirs must disable the trim, got %+v", got)
	}
}

// TestRun_TrimsCanonicalCacheUnderDiskPressure is the end-to-end receipt: a
// real `run` tick, forced into breach the way the host was on 2026-09-04,
// empties an over-budget cache and says so in its report. This is the path
// that did not exist when the disk filled.
func TestRun_TrimsCanonicalCacheUnderDiskPressure(t *testing.T) {
	dir := mkBuildCache(t, filepath.Join(t.TempDir(), "go-build"))
	before := cacheBytes(t, dir)

	var out bytes.Buffer
	code, err := run([]string{
		"--go-cache-dirs", dir,
		"--go-cache-max-gb", "0",
		"--build-cache-roots", "",
		"--e2e-cache-dir", "",
		"--gc-max-age", "0",
		"--trail", filepath.Join(t.TempDir(), "trail.jsonl"),
		"--brake", filepath.Join(t.TempDir(), "brake.json"),
		"--escalation-journal", filepath.Join(t.TempDir(), "escalation.jsonl"),
		"--escalation-state", filepath.Join(t.TempDir(), "escalation-state.json"),
		"--agm", "/nonexistent-agm",
		"--free-warn-gb", "9999999",
		"--free-critical-gb", "0.0001",
	}, &out)
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	if code != 1 {
		t.Fatalf("forced breach should exit 1, got %d\n%s", code, out.String())
	}
	if n := countShards(t, dir); n != 0 {
		t.Fatalf("%d shard(s) survived a breached tick\n%s", n, out.String())
	}
	want := humanBytes(before)
	if !strings.Contains(out.String(), "go cache") || !strings.Contains(out.String(), want) {
		t.Fatalf("report must state the reclaim (%s):\n%s", want, out.String())
	}
}

// TestRun_RejectsNegativeGoCacheBudget: a typo must fail loudly rather than
// silently disabling the only lever that reclaims real space.
func TestRun_RejectsNegativeGoCacheBudget(t *testing.T) {
	var out bytes.Buffer
	code, err := run([]string{"--go-cache-max-gb", "-1"}, &out)
	if code != 2 || err == nil {
		t.Fatalf("got (code=%d, err=%v), want a usage error with exit 2", code, err)
	}
	if !strings.Contains(err.Error(), "go-cache-max-gb") {
		t.Fatalf("error should name the offending flag, got %v", err)
	}
}

// TestDefaultGoCacheDirs_IncludesTheCanonicalCache: the whole defect was that
// the canonical cache was outside every configured scan root. The default must
// name it, or the fix ships switched off.
func TestDefaultGoCacheDirs_IncludesTheCanonicalCache(t *testing.T) {
	t.Setenv("GOCACHE", "/fixture/gocache")
	t.Setenv("GOLANGCI_LINT_CACHE", "/fixture/lintcache")

	got := defaultGoCacheDirs()
	for _, want := range []string{"/fixture/gocache", "/fixture/lintcache"} {
		if !strings.Contains(got, want) {
			t.Fatalf("defaultGoCacheDirs() = %q, want it to include %q", got, want)
		}
	}
}
