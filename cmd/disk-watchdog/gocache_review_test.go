package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fatten writes size bytes into every shard entry of an existing cache, so a
// budget can be crossed with real bytes rather than a fabricated figure.
func fatten(t *testing.T, dir string, size int) {
	t.Helper()
	payload := []byte(strings.Repeat("x", size))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		shard := filepath.Join(dir, e.Name())
		files, rerr := os.ReadDir(shard)
		if rerr != nil {
			continue
		}
		for _, f := range files {
			if err := os.WriteFile(filepath.Join(shard, f.Name()), payload, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// The trim must empty each shard without removing the shard directory.
//
// Go creates the 256 shard directories once, when it opens the cache, and
// later writes entries into them without recreating the parent. Removing a
// shard directory under a concurrent build therefore turns a cache miss,
// which this trim is explicitly allowed to cause, into an ENOENT that some
// build paths propagate as a build failure.
func TestTrimKeepsShardDirectories(t *testing.T) {
	dir := mkBuildCache(t, filepath.Join(t.TempDir(), "go-build"))
	fatten(t, dir, 64)

	res := trimCanonicalCaches(canonicalCacheConfig{
		Dirs: []string{dir}, MaxBytes: 1, Trim: true,
	})

	if len(res.Trimmed) != 1 {
		t.Fatalf("cache was not trimmed: %+v", res)
	}
	shard := filepath.Join(dir, "00")
	info, err := os.Stat(shard)
	if err != nil {
		t.Fatalf("shard directory must survive the trim: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("shard is no longer a directory")
	}
	entries, err := os.ReadDir(shard)
	if err != nil {
		t.Fatalf("read shard: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("shard still holds %d entries, want it emptied", len(entries))
	}
	if res.BytesReclaimed <= 0 {
		t.Errorf("BytesReclaimed = %d, want the emptied entries counted", res.BytesReclaimed)
	}
}

// The budget must be sized on the bytes the trim can actually reclaim.
//
// fuzz/ is deliberately preserved, so counting it toward the budget lets a
// corpus push an in-budget cache over the line. Every breached tick then
// wipes newly warmed shards without ever bringing the measured size under
// budget, and the dry run reports preserved bytes as reclaimable.
func TestBudgetCountsOnlyRemovableBytes(t *testing.T) {
	dir := mkBuildCache(t, filepath.Join(t.TempDir(), "go-build"))
	fatten(t, dir, 4)
	corpus := filepath.Join(dir, "fuzz", "FuzzParse")
	if err := os.MkdirAll(corpus, 0o755); err != nil {
		t.Fatalf("create corpus: %v", err)
	}
	if err := os.WriteFile(filepath.Join(corpus, "seed"), []byte(strings.Repeat("c", 8192)), 0o600); err != nil {
		t.Fatalf("write corpus: %v", err)
	}

	// The shards hold 256*4 bytes; the corpus alone is 8192. A budget above
	// the shard total but below the whole-root total must read as in budget.
	res := trimCanonicalCaches(canonicalCacheConfig{
		Dirs: []string{dir}, MaxBytes: 4096, Trim: true,
	})

	if len(res.Trimmed) != 0 {
		t.Errorf("cache was trimmed although its removable bytes are within budget: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(corpus, "seed")); err != nil {
		t.Errorf("fuzz corpus must survive: %v", err)
	}

	dry := trimCanonicalCaches(canonicalCacheConfig{
		Dirs: []string{dir}, MaxBytes: 4096, Trim: false,
	})
	if dry.BytesReclaimable != 0 {
		t.Errorf("BytesReclaimable = %d, want 0: preserved bytes are not reclaimable", dry.BytesReclaimable)
	}
}

// A cache budget that is not a finite, representable byte count must be
// rejected at parse time. NaN and +Inf both slip past a "< 0" check, and the
// float-to-int64 conversion then yields an implementation-defined value,
// commonly MinInt64 — under which every proven cache compares as over budget
// and is emptied on the next breached tick.
func TestGoCacheBudgetRejectsNonFiniteAndOverflow(t *testing.T) {
	for _, tc := range []struct {
		name string
		gb   float64
	}{
		{"NaN", math.NaN()},
		{"positive infinity", math.Inf(1)},
		{"negative infinity", math.Inf(-1)},
		{"beyond int64 bytes", math.MaxFloat64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := goCacheMaxBytes(tc.gb); err == nil {
				t.Errorf("goCacheMaxBytes(%v) accepted an unusable budget", tc.gb)
			}
		})
	}

	got, err := goCacheMaxBytes(20)
	if err != nil {
		t.Fatalf("goCacheMaxBytes(20): %v", err)
	}
	if want := int64(20) << 30; got != want {
		t.Errorf("goCacheMaxBytes(20) = %d, want %d", got, want)
	}
}

// Whole-directory reaping of an abandoned cache must not take a fuzz corpus
// with it. Accepting fuzz/ as cache furniture made these roots eligible for
// the abandoned-cache path, which removes the entire root rather than only
// its shards, destroying coverage-increasing inputs that cost compute to find.
func TestAbandonedReapPreservesFuzzCorpus(t *testing.T) {
	base := t.TempDir()
	dir := mkBuildCache(t, filepath.Join(base, "go-build"))
	plain := mkBuildCache(t, filepath.Join(base, "plain-cache"))
	corpus := filepath.Join(dir, "fuzz", "FuzzParse")
	if err := os.MkdirAll(corpus, 0o755); err != nil {
		t.Fatalf("create corpus: %v", err)
	}
	seed := filepath.Join(corpus, "seed")
	if err := os.WriteFile(seed, []byte("corpus"), 0o600); err != nil {
		t.Fatalf("write corpus: %v", err)
	}
	aged(t, dir, 48*time.Hour)
	aged(t, plain, 48*time.Hour)

	res := reapBuildCaches(buildCacheConfig{
		Roots: []string{base}, MaxDepth: 2, MinAge: 24 * time.Hour, Reap: true,
		inUse: idle,
	})

	if _, err := os.Stat(plain); !os.IsNotExist(err) {
		t.Fatalf("a corpus-free abandoned cache must still be reaped, stat err = %v", err)
	}

	if _, err := os.Stat(seed); err != nil {
		t.Fatalf("fuzz corpus was destroyed by the abandoned-cache reaper: %v", err)
	}
	// findBuildCacheRoots reports resolved paths, so match on the reason
	// rather than on the caller's spelling of the directory.
	var reason string
	for path, why := range res.Skipped {
		if strings.HasSuffix(path, "go-build") {
			reason = why
		}
	}
	if !strings.Contains(reason, "fuzz") {
		t.Errorf("skip reason for the corpus-bearing cache = %q, want it to name the corpus", reason)
	}
}

// The lint cache preflight actually grows lives one level below the path the
// watchdog can name, because scripts/preflight.sh keys it on the checkout:
// ${XDG_CACHE_HOME:-$HOME/.cache}/dear-agent/golangci-lint/<cksum>. The
// launchd job sets no GOLANGCI_LINT_CACHE, so the parent is the only thing
// the watchdog can be configured with, and a parent of cache roots is not
// itself a proven root.
func TestCacheRootsAreDiscoveredOneLevelDown(t *testing.T) {
	parent := t.TempDir()
	first := mkBuildCache(t, filepath.Join(parent, "1234567890"))
	second := mkBuildCache(t, filepath.Join(parent, "9876543210"))
	fatten(t, first, 64)
	fatten(t, second, 64)

	res := trimCanonicalCaches(canonicalCacheConfig{
		Dirs: []string{parent}, MaxBytes: 1, Trim: true,
	})

	if len(res.Trimmed) != 2 {
		t.Fatalf("Trimmed = %v, want both per-checkout caches; skipped=%v", res.Trimmed, res.Skipped)
	}
	for _, dir := range []string{first, second} {
		if n := nonEmptyShards(t, dir); n != 0 {
			t.Errorf("%s: %d shard(s) still hold entries", dir, n)
		}
	}
}

// Expansion must not loosen the structural proof. A parent whose children are
// ordinary directories is still refused, and reported as such, rather than
// silently expanded into real work.
func TestExpansionStillRefusesNonCacheChildren(t *testing.T) {
	parent := t.TempDir()
	src := filepath.Join(parent, "some-checkout")
	if err := os.MkdirAll(filepath.Join(src, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "internal", "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	res := trimCanonicalCaches(canonicalCacheConfig{
		Dirs: []string{parent}, MaxBytes: 1, Trim: true,
	})

	if len(res.Trimmed) != 0 {
		t.Fatalf("Trimmed = %v, want nothing", res.Trimmed)
	}
	if _, err := os.Stat(filepath.Join(src, "internal", "main.go")); err != nil {
		t.Fatalf("source tree must survive: %v", err)
	}
	if !strings.Contains(res.Skipped[parent], "not a proven") {
		t.Errorf("Skipped[%s] = %q, want a not-a-cache reason", parent, res.Skipped[parent])
	}
}
