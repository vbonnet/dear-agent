package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The canonical Go build cache is a different failure from an abandoned one,
// and the watchdog only had a fix for the abandoned kind.
//
// On 2026-09-04 the host reached 97% used with 12 GiB free and writes failing.
// The dominant consumer was the canonical GOCACHE at 48 GB, grown by a Codex
// desktop re-run of `make test-bdd` (`go test ./test/bdd/...`). A human ran
// `go clean -cache` and reclaimed ~44 GiB. The watchdog reclaimed nothing,
// because reapAbandonedBuildCaches scans /tmp and $TMPDIR for caches nobody
// owns any more, and the canonical cache is neither: it lives under
// os.UserCacheDir, and every build touches it, so an age gate can never fire.
//
// Two consequences shape this file.
//
// The gate is size, not age. A cache is not stale, it is oversized, and the
// budget is the only thing that distinguishes "warm" from "the reason writes
// fail".
//
// The trim runs only under disk pressure. Everything here is regenerable at
// the cost of rebuild time, so spending that cost on a healthy host is a pure
// loss; measuring alone means walking hundreds of thousands of files. Under
// pressure the trade inverts completely.
//
// Deletion safety is inherited wholesale from isGoBuildCacheRoot: a directory
// is emptied only when every top-level entry is a hex shard or known cache
// furniture and every sampled shard holds nothing but content-addressed
// entries. That proof is what keeps a mis-set GOCACHE pointing at a worktree
// from costing anyone their source.
//
// Unlike the abandoned-cache reaper, there is deliberately no liveness gate.
// A concurrent build almost always holds this cache open, so requiring an idle
// directory would mean never firing on the host that needs it. Removing an
// entry under a running build is a cache miss, not a corruption. The cache is
// content-addressed and Go recreates shards on demand, which is exactly what
// `go clean -cache` relies on.

// canonicalCacheConfig parameterises one trim pass. The seams exist so the
// policy is testable without real deletions.
type canonicalCacheConfig struct {
	// Dirs are canonical cache roots (GOCACHE, GOLANGCI_LINT_CACHE).
	Dirs []string
	// MaxBytes is the size budget. A cache at or under it is left alone.
	MaxBytes int64
	// Trim distinguishes a real pass from a --dry-run measurement.
	Trim bool

	sizeOf func(path string) int64
	remove func(path string) error
}

func (c canonicalCacheConfig) withDefaults() canonicalCacheConfig {
	if c.sizeOf == nil {
		c.sizeOf = dirBytes
	}
	if c.remove == nil {
		c.remove = os.RemoveAll
	}
	return c
}

// canonicalCacheTrimResult is one pass's outcome, shaped for the watchdog's
// JSON report and its decision-trail record.
//
// Skipped is keyed by path so a pass that reclaims nothing still says exactly
// why. A silent zero is indistinguishable from a host with nothing to reclaim,
// and that ambiguity is what let the disk fill repeatedly while every tick
// printed "Status: OK".
type canonicalCacheTrimResult struct {
	Scanned int      `json:"scanned"`
	Trimmed []string `json:"trimmed,omitempty"`
	// BytesReclaimed counts only bytes that were actually deleted.
	BytesReclaimed int64 `json:"bytes_reclaimed"`
	// BytesReclaimable is what a non-dry-run pass would have deleted.
	BytesReclaimable int64             `json:"bytes_reclaimable,omitempty"`
	Skipped          map[string]string `json:"skipped,omitempty"`
	Errors           map[string]string `json:"errors,omitempty"`
}

// trimCanonicalCaches empties every configured cache root that is both
// provably a Go-style build cache and over its size budget.
//
// Shard directories are removed; the root and its furniture (README,
// trim.txt) survive, so a concurrent build finds the directory it expects.
func trimCanonicalCaches(cfg canonicalCacheConfig) canonicalCacheTrimResult {
	g := cfg.withDefaults()
	res := canonicalCacheTrimResult{Skipped: map[string]string{}, Errors: map[string]string{}}

	for _, dir := range cfg.Dirs {
		res.Scanned++
		if !isGoBuildCacheRoot(dir) {
			// Covers a missing directory, an unreadable one, and a real
			// directory holding real work. All three mean keep.
			res.Skipped[dir] = "not a proven Go build cache root"
			continue
		}
		size := g.sizeOf(dir)
		if size <= cfg.MaxBytes {
			res.Skipped[dir] = fmt.Sprintf("within budget (%.1f GiB <= %.1f GiB)",
				gib(size), gib(cfg.MaxBytes))
			continue
		}
		if !g.Trim {
			res.Skipped[dir] = fmt.Sprintf("dry run (%.1f GiB over budget)", gib(size-cfg.MaxBytes))
			res.BytesReclaimable += size
			continue
		}
		res.BytesReclaimed += trimCacheShards(dir, g, &res)
		res.Trimmed = append(res.Trimmed, dir)
	}
	sort.Strings(res.Trimmed)
	return res
}

// trimCacheShards removes dir's shard directories and returns the bytes that
// deletion actually reclaimed. A shard that fails to delete is recorded as an
// error and contributes nothing: counting an undeleted shard as reclaimed is
// how a still-full disk gets reported as remediated.
func trimCacheShards(dir string, g canonicalCacheConfig, res *canonicalCacheTrimResult) int64 {
	shards, _, ok := cacheShards(dir)
	if !ok {
		res.Errors[dir] = "cache root became unreadable during the trim"
		return 0
	}
	var reclaimed int64
	for _, shard := range shards {
		size := g.sizeOf(shard)
		if err := g.remove(shard); err != nil {
			res.Errors[shard] = err.Error()
			continue
		}
		reclaimed += size
	}
	return reclaimed
}

func gib(b int64) float64 { return float64(b) / (1 << 30) }

// humanBytes renders a reclaim figure in a unit that shows it. "0.0 GiB" for a
// real 900 MiB reclaim reads as "reclaimed nothing", which is the exact
// ambiguity this watchdog keeps getting caught by.
func humanBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(b)/(1<<20))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// defaultGoCacheDirs names the canonical caches this host actually writes.
//
// GOCACHE and GOLANGCI_LINT_CACHE are read from the environment when set, so a
// relocated cache is still covered, and fall back to the conventional
// locations under os.UserCacheDir, which is where the 48 GB that filled the
// disk was sitting, outside every scan root the watchdog had.
func defaultGoCacheDirs() string {
	var dirs []string
	add := func(dir string) {
		if dir = strings.TrimSpace(dir); dir != "" {
			dirs = append(dirs, dir)
		}
	}
	cacheHome, err := os.UserCacheDir()
	if goCache := os.Getenv("GOCACHE"); goCache != "" {
		add(goCache)
	} else if err == nil {
		add(filepath.Join(cacheHome, "go-build"))
	}
	if lintCache := os.Getenv("GOLANGCI_LINT_CACHE"); lintCache != "" {
		add(lintCache)
	} else if err == nil {
		add(filepath.Join(cacheHome, "golangci-lint"))
	}
	return strings.Join(dirs, ",")
}

// defaultGoCacheMaxGB is the canonical cache's size budget.
//
// Twenty GiB is comfortably more than a full build of every module here needs
// and well under the point where the cache alone can exhaust the disk. The
// cache that triggered this was 48 GB.
const defaultGoCacheMaxGB = 20

// trimOversizedCanonicalCaches runs one trim pass for a watchdog tick.
//
// It returns nil when the trim is disabled or the tick is not breached, so the
// report and the JSON stay silent about work that did not happen, and so a
// healthy host never pays for the walk.
func trimOversizedCanonicalCaches(cfg config, breached bool) *canonicalCacheTrimResult {
	if strings.TrimSpace(cfg.goCacheDirs) == "" || !breached {
		return nil
	}
	var dirs []string
	seen := map[string]bool{}
	for d := range strings.SplitSeq(cfg.goCacheDirs, ",") {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		// Dedup after resolving symlinks so a cache reachable by two spellings
		// is neither walked nor counted twice.
		key := d
		if resolved, err := filepath.EvalSymlinks(d); err == nil {
			key = resolved
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		dirs = append(dirs, d)
	}
	res := trimCanonicalCaches(canonicalCacheConfig{
		Dirs:     dirs,
		MaxBytes: cfg.goCacheMaxBytes,
		Trim:     !cfg.dryRun,
	})
	return &res
}

// summarizeCanonicalCacheTrim renders one line for the watchdog's report.
func summarizeCanonicalCacheTrim(res canonicalCacheTrimResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "go cache  : %d scanned, %d trimmed, %s reclaimed",
		res.Scanned, len(res.Trimmed), humanBytes(res.BytesReclaimed))
	if res.BytesReclaimable > 0 {
		fmt.Fprintf(&b, " (%s reclaimable)", humanBytes(res.BytesReclaimable))
	}
	if len(res.Errors) > 0 {
		fmt.Fprintf(&b, ", %d error(s)", len(res.Errors))
	}
	return b.String()
}
