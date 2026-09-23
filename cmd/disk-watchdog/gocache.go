package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/pkg/vroom/supervisor"
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

	for _, dir := range expandCacheRoots(cfg.Dirs) {
		res.Scanned++
		if !isGoBuildCacheRoot(dir) {
			// Covers a missing directory, an unreadable one, and a real
			// directory holding real work. All three mean keep.
			res.Skipped[dir] = "not a proven Go build cache root"
			continue
		}
		m, ok := measureRemovable(dir, g)
		if !ok {
			res.Skipped[dir] = "cache root became unreadable while measuring it"
			continue
		}
		if m.total <= cfg.MaxBytes {
			res.Skipped[dir] = fmt.Sprintf("within budget (%.1f GiB removable <= %.1f GiB)",
				gib(m.total), gib(cfg.MaxBytes))
			continue
		}
		if !g.Trim {
			res.Skipped[dir] = fmt.Sprintf("dry run (%.1f GiB over budget)", gib(m.total-cfg.MaxBytes))
			res.BytesReclaimable += m.total
			continue
		}
		res.BytesReclaimed += trimCacheShards(m, g, &res)
		res.Trimmed = append(res.Trimmed, dir)
	}
	sort.Strings(res.Trimmed)
	return res
}

// cacheMeasurement is one root measured once.
//
// The budget decision, the dry-run reclaimable figure, and the post-deletion
// accounting all read from this single pass. The trim used to walk the whole
// root for the budget and then walk every shard again to total the deletion,
// so a cache of several hundred thousand files paid for two full metadata
// passes before the first byte came back, during the emergency the trim
// exists to end.
type cacheMeasurement struct {
	root   string
	shards []string
	sizes  map[string]int64
	// total counts only bytes this trim can actually remove. Preserved
	// furniture, including a fuzz corpus, is not reclaimable, so counting it
	// toward the budget would let a corpus push an in-budget cache over the
	// line and wipe newly warmed shards on every breached tick without ever
	// bringing the measured size down.
	total int64
}

// measureRemovable walks each shard of dir exactly once.
func measureRemovable(dir string, g canonicalCacheConfig) (cacheMeasurement, bool) {
	shards, _, ok := cacheShards(dir)
	if !ok {
		return cacheMeasurement{}, false
	}
	m := cacheMeasurement{root: dir, shards: shards, sizes: make(map[string]int64, len(shards))}
	for _, shard := range shards {
		size := g.sizeOf(shard)
		m.sizes[shard] = size
		m.total += size
	}
	return m, true
}

// trimCacheShards empties each measured shard and returns the bytes deletion
// actually reclaimed. A shard that fails to empty is recorded as an error and
// contributes only what the filesystem gave back: counting an undeleted shard
// as reclaimed is how a still-full disk gets reported as remediated.
//
// The shard directories themselves survive. Go creates all 256 once, when it
// opens the cache, and later writes entries into them without recreating the
// parent, so removing a shard under a concurrent build turns a cache miss,
// which this trim is allowed to cause, into an ENOENT that some build paths
// propagate as a failure. Go's own cache trimming removes entries and keeps
// the shards.
func trimCacheShards(m cacheMeasurement, g canonicalCacheConfig, res *canonicalCacheTrimResult) int64 {
	var reclaimed int64
	for _, shard := range m.shards {
		before := m.sizes[shard]
		if err := emptyShard(shard, g); err != nil {
			res.Errors[shard] = err.Error()
			reclaimed += before - g.sizeOf(shard)
			continue
		}
		reclaimed += before
	}
	return reclaimed
}

// emptyShard removes every entry in shard, leaving the directory itself.
func emptyShard(shard string, g canonicalCacheConfig) error {
	entries, err := os.ReadDir(shard)
	if err != nil {
		return err
	}
	var firstErr error
	for _, e := range entries {
		if rerr := g.remove(filepath.Join(shard, e.Name())); rerr != nil && firstErr == nil {
			firstErr = rerr
		}
	}
	return firstErr
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
	} else {
		// os.Getenv cannot see a GOCACHE persisted with `go env -w`, and the
		// launchd job passes only PATH, HOME and DOLT_PORT, so it cannot see a
		// shell-only override either. `go env GOCACHE` reports the effective
		// setting, which is the cache Go actually writes. The conventional
		// location stays as the fallback for a host with no usable toolchain.
		if effective := effectiveGoEnv("GOCACHE"); effective != "" {
			add(effective)
		} else if err == nil {
			add(filepath.Join(cacheHome, "go-build"))
		}
	}
	if lintCache := os.Getenv("GOLANGCI_LINT_CACHE"); lintCache != "" {
		add(lintCache)
	} else if err == nil {
		add(filepath.Join(cacheHome, "golangci-lint"))
		// scripts/preflight.sh keys the lint cache on the checkout:
		// ${XDG_CACHE_HOME:-$HOME/.cache}/dear-agent/golangci-lint/<cksum>.
		// The launchd job sets no GOLANGCI_LINT_CACHE, so without this the
		// watchdog fell back to ~/.cache/golangci-lint, a path that does not
		// normally exist on this host, and the caches preflight actually
		// grows stayed unreachable during disk pressure. This names the
		// parent; expandCacheRoots finds the per-worktree roots under it.
		add(filepath.Join(xdgCacheHome(cacheHome), "dear-agent", "golangci-lint"))
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

// goCacheMaxBytes converts a -go-cache-max-gb value into a byte budget.
//
// A bare "< 0" check lets NaN and both infinities through, and the
// float-to-int64 conversion of any of them, or of a finite value larger than
// math.MaxInt64 bytes, is implementation-defined. On this platform it yields
// MinInt64, under which every proven cache compares as over budget and is
// emptied on the next breached tick despite the caller having asked for a
// nonnegative or effectively unlimited budget.
func goCacheMaxBytes(gb float64) (int64, error) {
	switch {
	case math.IsNaN(gb):
		return 0, fmt.Errorf("invalid -go-cache-max-gb %v: the cache budget must be a number", gb)
	case math.IsInf(gb, 0):
		return 0, fmt.Errorf("invalid -go-cache-max-gb %v: the cache budget must be finite (pass an empty -go-cache-dirs to disable the trim)", gb)
	case gb < 0:
		return 0, fmt.Errorf("invalid -go-cache-max-gb %v: the cache budget cannot be negative (pass an empty -go-cache-dirs to disable the trim)", gb)
	}
	// Validate the SCALED value, not the input against a scaled limit.
	// float64(math.MaxInt64) rounds up to 2^63, so comparing gb against
	// float64(math.MaxInt64)/GiB admits exactly 8589934592 GiB, whose product
	// is 2^63: one past int64, which converts to MinInt64 here. Every proven
	// cache would then read as over a negative budget and be emptied on the
	// next breached tick, which is the failure the check exists to prevent.
	scaled := gb * supervisor.GiB
	if scaled >= float64(math.MaxInt64) {
		return 0, fmt.Errorf("invalid -go-cache-max-gb %v: the cache budget does not fit in a byte count", gb)
	}
	return int64(scaled), nil
}

// xdgCacheHome returns the base directory preflight resolves its lint cache
// against. os.UserCacheDir already honours XDG_CACHE_HOME on Linux, but on
// macOS it returns ~/Library/Caches while the shell expression falls back to
// ~/.cache, so the two disagree exactly where the cache is written.
func xdgCacheHome(fallback string) string {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); xdg != "" {
		return xdg
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".cache")
	}
	return fallback
}

// expandCacheRoots resolves each configured path to the cache roots under it.
//
// A configured path is usually a cache root itself. It can also be a parent of
// several, which is how preflight keys its lint cache per checkout, and a
// parent of cache roots is not itself a proven root: its entries are named
// directories rather than hex shards, so the structural proof rejects it and
// every cache beneath it stays unreachable. One level of expansion covers that
// layout without loosening the proof each root still has to pass.
func expandCacheRoots(dirs []string) []string {
	var out []string
	for _, dir := range dirs {
		if isGoBuildCacheRoot(dir) {
			out = append(out, dir)
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			out = append(out, dir)
			continue
		}
		var children []string
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			child := filepath.Join(dir, e.Name())
			if isGoBuildCacheRoot(child) {
				children = append(children, child)
			}
		}
		if len(children) == 0 {
			out = append(out, dir)
			continue
		}
		out = append(out, children...)
	}
	sort.Strings(out)
	return out
}

// goEnvTimeout bounds the toolchain query. The watchdog runs on a breached
// tick, so a hung `go` must not hold the reclaim it is about to perform.
const goEnvTimeout = 5 * time.Second

// effectiveGoEnv returns `go env <name>`, or "" when the toolchain is absent,
// slow, or reports nothing. Every failure path is a silent fallback: this is
// discovery, and a host without a usable `go` on PATH is an ordinary case
// rather than an error worth failing a disk-pressure tick over.
func effectiveGoEnv(name string) string {
	ctx, cancel := context.WithTimeout(context.Background(), goEnvTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "go", "env", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
