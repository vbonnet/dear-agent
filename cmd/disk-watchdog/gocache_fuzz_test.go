package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A real GOCACHE has a fuzz/ directory, and the structural proof rejected it.
//
// Found by running this watchdog against the actual cache on the host it was
// written for. It reported:
//
//	go cache  : 2 scanned, 0 trimmed, 0 B reclaimed
//	skipped: ~/Library/Caches/go-build: "not a proven Go build cache root"
//
// while that cache held 47 GB. The cause is that `go test -fuzz` creates a
// fuzz/ DIRECTORY in the cache root. cacheShards accepts only hex shard
// directories and a fixed set of regular-file furniture (README, trim.txt,
// lock, testexpire.txt), and returns not-a-cache for anything else. One
// directory therefore disqualified the entire cache.
//
// This is the failure mode that matters most for a reaper: not a crash, but a
// silent refusal that reports success. The cache this feature exists to
// reclaim was the one cache it could not touch.
func TestCacheShardsAcceptsFuzzDirectory(t *testing.T) {
	dir := mkBuildCache(t, filepath.Join(t.TempDir(), "go-build"))

	// Exactly what `go test -fuzz` leaves behind.
	fuzzCorpus := filepath.Join(dir, "fuzz", "FuzzParse")
	if err := os.MkdirAll(fuzzCorpus, 0o755); err != nil {
		t.Fatalf("create fuzz corpus: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fuzzCorpus, "seed"), []byte("corpus"), 0o644); err != nil {
		t.Fatalf("write corpus entry: %v", err)
	}

	if !isGoBuildCacheRoot(dir) {
		t.Fatal("a cache root holding fuzz/ must still be a proven Go build cache root; " +
			"rejecting it is why the 47 GB canonical cache on this host was never trimmed")
	}
}

// The trim must leave fuzz/ alone. Shards are regenerable build output; a fuzz
// corpus is an input that took CPU time to find and is not reproducible on
// demand. Deleting it to reclaim disk would be a data loss, not a reclaim.
func TestTrimCanonicalCachesPreservesFuzzCorpus(t *testing.T) {
	dir := mkBuildCache(t, filepath.Join(t.TempDir(), "go-build"))
	fuzzCorpus := filepath.Join(dir, "fuzz", "FuzzParse")
	if err := os.MkdirAll(fuzzCorpus, 0o755); err != nil {
		t.Fatalf("create fuzz corpus: %v", err)
	}
	seed := filepath.Join(fuzzCorpus, "seed")
	if err := os.WriteFile(seed, []byte("corpus"), 0o644); err != nil {
		t.Fatalf("write corpus entry: %v", err)
	}

	res := trimCanonicalCaches(canonicalCacheConfig{
		Dirs:     []string{dir},
		MaxBytes: 0, // over budget, so the trim definitely runs
		Trim:     true,
	})

	if len(res.Trimmed) != 1 {
		t.Fatalf("Trimmed = %v, want the cache trimmed; skipped=%v errors=%v",
			res.Trimmed, res.Skipped, res.Errors)
	}
	if n := countShards(t, dir); n != 0 {
		t.Fatalf("%d shard(s) survived the trim, want 0", n)
	}
	if _, err := os.Stat(seed); err != nil {
		t.Fatalf("fuzz corpus must survive the trim: %v", err)
	}
}

// A directory in the cache root that is NOT fuzz/ must still disqualify the
// root. The allowance is for one known Go-created directory, not a general
// relaxation: the whole point of the structural proof is that a GOCACHE
// mis-set to a source tree reclaims nothing.
func TestCacheShardsStillRejectsForeignDirectory(t *testing.T) {
	dir := mkBuildCache(t, filepath.Join(t.TempDir(), "go-build"))
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatalf("create foreign dir: %v", err)
	}

	if isGoBuildCacheRoot(dir) {
		t.Fatal("a cache root holding an unexpected directory must not be proven")
	}
}
