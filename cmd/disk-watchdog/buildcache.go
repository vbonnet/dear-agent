package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
)

// Abandoned Go build caches were the single largest source of disk growth on
// this host: 262 directories holding 37.1 GB, every one of them created
// within four days (~9 GB/day), and nothing anywhere owned their cleanup.
//
// They exist because agents were told to dodge golangci-lint's global lock by
// pointing GOCACHE/GOLANGCI_LINT_CACHE at a private directory under /tmp. The
// workaround does not even work — that lock is global and unrelated to the
// cache directory, so `parallel golangci-lint is running` still fires — but
// every attempt left another multi-GB cache behind.
//
// A cache older than the age gate has no value at all: the next run creates
// its own. So this reaps continuously rather than only under disk pressure.
// Waiting for a breach would mean absorbing ~9 GB/day between breaches, which
// is firefighting, not a bound.

// hexShard matches the two-hex-digit shard directories a Go build cache is
// made of ("00".."ff").
var hexShard = regexp.MustCompile(`^[0-9a-f]{2}$`)

// cacheEntryFile matches the content-addressed files a Go-style cache stores
// inside each shard: a 64-hex action/output ID with an "-a" or "-d" suffix.
var cacheEntryFile = regexp.MustCompile(`^[0-9a-f]{64}-[ad]$`)

// buildCacheFurniture is every entry name a cache root may contain besides
// its hex shards.
var buildCacheFurniture = map[string]bool{
	"README":         true,
	"trim.txt":       true,
	"lock":           true,
	"testexpire.txt": true,
}

// minHexShards is how many shard directories must be present before a
// directory is accepted. Go and golangci-lint both create all 256 up front,
// so the threshold only has to be high enough that no ordinary directory
// reaches it by coincidence.
const minHexShards = 64

// shardSampleSize bounds how many shards are inspected for foreign content.
// Reading all 256 on every five-minute tick would be wasteful, and a
// directory that is a cache in its first 32 shards and something else in the
// rest does not occur in practice.
const shardSampleSize = 32

// isGoBuildCacheRoot reports whether dir is provably a Go-style build cache
// root (Go's own GOCACHE, or a GOLANGCI_LINT_CACHE, which shares the layout).
//
// Proof is by content, never by name: the reaper must not be fooled by a
// directory called "gocache" that holds real work, and must still recognise a
// cache written under an unrelated name.
//
// Two conditions, both structural:
//
//  1. Every top-level entry is either a hex shard directory or known
//     furniture — nothing foreign at all — and there are at least
//     minHexShards shards.
//  2. Every sampled shard contains only content-addressed cache files (or is
//     empty, which is what a trimmed or externally-emptied cache leaves
//     behind).
//
// Together these are what makes deletion safe: a directory whose entire
// contents are hex shards holding nothing but hash-named cache files has no
// other content to lose. Requiring a marker file instead would be both weaker
// and wrong — on this host 42 of 43 real caches carried no trim.txt at all.
//
// Every failure path returns false, so an unreadable directory is kept.
func isGoBuildCacheRoot(dir string) bool {
	shards, ok := cacheShards(dir)
	if !ok || len(shards) < minHexShards {
		return false
	}
	sort.Strings(shards)
	if len(shards) > shardSampleSize {
		shards = shards[:shardSampleSize]
	}
	return shardsHoldOnlyCacheEntries(shards)
}

func sameDevice(a, b os.FileInfo) bool {
	sa, oka := a.Sys().(*syscall.Stat_t)
	sb, okb := b.Sys().(*syscall.Stat_t)
	if !oka || !okb {
		return false
	}
	return sa.Dev == sb.Dev
}

// cacheShards returns dir's hex shard directories, reporting ok=false as soon
// as it sees anything that is neither a shard nor known furniture. A symlink
// at this level also disqualifies: following one is how a reaper gets steered
// outside the tree it was pointed at.
func cacheShards(dir string) (shards []string, ok bool) {
	rootInfo, err := os.Stat(dir)
	if err != nil {
		return nil, false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}
	for _, e := range entries {
		name := e.Name()
		info, ierr := e.Info()
		if ierr != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil, false
		}
		if !sameDevice(info, rootInfo) {
			return nil, false
		}
		switch {
		case e.IsDir() && hexShard.MatchString(name):
			shards = append(shards, filepath.Join(dir, name))
		case buildCacheFurniture[name] && info.Mode().IsRegular():
			// furniture, fine
		default:
			return nil, false
		}
	}
	return shards, true
}

// shardsHoldOnlyCacheEntries reports whether every given shard contains only
// content-addressed cache files or Go executable cache directories — or nothing,
// which is what a trimmed or externally-emptied cache leaves behind. An unreadable shard is a false.
func shardsHoldOnlyCacheEntries(shards []string) bool {
	for _, shard := range shards {
		if !shardHoldsOnlyCacheEntries(shard) {
			return false
		}
	}
	return true
}

func shardHoldsOnlyCacheEntries(shard string) bool {
	shardBase := filepath.Base(shard)
	names, err := os.ReadDir(shard)
	if err != nil {
		return false
	}
	for _, n := range names {
		if !isValidShardEntry(shard, shardBase, n) {
			return false
		}
	}
	return true
}

func isValidShardEntry(shard, shardBase string, n os.DirEntry) bool {
	name := n.Name()
	if !cacheEntryFile.MatchString(name) || len(name) < 2 || name[:2] != shardBase {
		return false
	}
	info, ierr := n.Info()
	if ierr != nil || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	if !n.IsDir() {
		return info.Mode().IsRegular()
	}
	if !strings.HasSuffix(name, "-d") {
		return false
	}
	execEntries, err := os.ReadDir(filepath.Join(shard, name))
	if err != nil {
		return false
	}
	for _, ee := range execEntries {
		einfo, err := ee.Info()
		if err != nil || !einfo.Mode().IsRegular() {
			return false
		}
	}
	return true
}

// findBuildCacheRoots walks root to at most maxDepth levels and returns every
// proven Go build cache root beneath it.
//
// A proven cache root is never descended into: its shards are part of it, not
// separate candidates. Symlinks are never followed, so a link planted in the
// scanned directory cannot redirect the reaper outside root.
func findBuildCacheRoots(root string, maxDepth int) ([]string, error) {
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		// A missing scan root is an empty result, not a failure: the reaper
		// is best-effort remediation and must not fail a watchdog tick.
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("resolve scan root %q: %w", root, err)
	}

	var found []string
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > maxDepth {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			child := filepath.Join(dir, e.Name())
			// Lstat, not Stat: a symlinked directory must not be followed.
			info, err := os.Lstat(child)
			if err != nil || info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if isGoBuildCacheRoot(child) {
				found = append(found, child)
				continue // do not descend into a cache's own shards
			}
			walk(child, depth+1)
		}
	}
	walk(resolved, 1)
	sort.Strings(found)
	return found, nil
}

// buildCacheConfig parameterises one reap pass. The seams exist so the policy
// can be tested without a real clock, a real lsof, or real deletions.
type buildCacheConfig struct {
	Roots    []string
	MaxDepth int
	MinAge   time.Duration
	Reap     bool

	now    func() time.Time
	inUse  func(path string) (bool, error)
	remove func(path string) error
	sizeOf func(path string) int64
}

// buildCacheReapResult is one pass's outcome, shaped for the watchdog's JSONL
// record. Skipped is keyed by path so a run that reclaims nothing still says
// exactly why, rather than looking like a host with nothing to reclaim — the
// failure mode that hid the worktree-sweep truncation for months.
type buildCacheReapResult struct {
	Scanned        int               `json:"scanned"`
	Reaped         []string          `json:"reaped,omitempty"`
	Skipped        map[string]string `json:"skipped,omitempty"`
	BytesReclaimed int64             `json:"bytes_reclaimed"`
	Errors         map[string]string `json:"errors,omitempty"`
}

// reapBuildCaches finds and (when Reap is set) removes abandoned Go build
// caches under the configured roots.
//
// Every gate is fail-closed: an unreadable age, an unresolvable liveness
// probe, or an unproven directory all mean "keep". The only way to be deleted
// is to be a proven cache root, older than MinAge, with no process inside.
func reapBuildCaches(cfg buildCacheConfig) buildCacheReapResult {
	res := buildCacheReapResult{Skipped: map[string]string{}, Errors: map[string]string{}}
	g := cfg.withDefaults()

	for _, root := range cfg.Roots {
		caches, err := findBuildCacheRoots(root, cfg.MaxDepth)
		if err != nil {
			res.Errors[root] = err.Error()
			continue
		}
		for _, cache := range caches {
			res.Scanned++
			if reason := g.keepReason(cache, cfg.MinAge); reason != "" {
				res.Skipped[cache] = reason
				continue
			}
			size := g.sizeOf(cache)
			if !cfg.Reap {
				res.Skipped[cache] = "scan only"
				res.BytesReclaimed += size
				continue
			}
			if err := g.remove(cache); err != nil {
				res.Errors[cache] = err.Error()
				continue
			}
			res.Reaped = append(res.Reaped, cache)
			res.BytesReclaimed += size
		}
	}
	if len(res.Skipped) == 0 {
		res.Skipped = nil
	}
	if len(res.Errors) == 0 {
		res.Errors = nil
	}
	return res
}

// buildCacheGates is one pass's resolved seams.
type buildCacheGates struct {
	now    func() time.Time
	inUse  func(string) (bool, error)
	remove func(string) error
	sizeOf func(string) int64
}

// withDefaults resolves every nil seam to its real-host implementation.
func (cfg buildCacheConfig) withDefaults() buildCacheGates {
	g := buildCacheGates{now: cfg.now, inUse: cfg.inUse, remove: cfg.remove, sizeOf: cfg.sizeOf}
	if g.now == nil {
		g.now = time.Now
	}
	if g.inUse == nil {
		g.inUse = newLsofProber()
	}
	if g.remove == nil {
		g.remove = os.RemoveAll
	}
	if g.sizeOf == nil {
		g.sizeOf = dirBytes
	}
	return g
}

// keepReason returns why this cache must be kept, or "" when it is reapable.
//
// Every branch is fail-closed: an unreadable age or a liveness probe that
// cannot be evaluated both mean keep. The only path to deletion is a cache
// older than minAge with no process inside it.
func (g buildCacheGates) keepReason(cache string, minAge time.Duration) string {
	info, err := os.Lstat(cache)
	if err != nil {
		return "stat failed: " + err.Error()
	}
	if age := g.now().Sub(info.ModTime()); age < minAge {
		return fmt.Sprintf("too recent (%s < %s)", age.Round(time.Second), minAge)
	}
	busy, err := g.inUse(cache)
	if err != nil {
		return "liveness probe failed: " + err.Error()
	}
	if busy {
		return "a process holds a file open inside"
	}
	return ""
}

// dirBytes sums the apparent size of every regular file beneath dir.
func dirBytes(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // best-effort accounting; a partial sum is fine
		}
		if info, ierr := d.Info(); ierr == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total
}

// summarizeBuildCacheReap renders one line for the watchdog's report.
func summarizeBuildCacheReap(res buildCacheReapResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "build caches: %d found, %d reaped, %.1f GiB",
		res.Scanned, len(res.Reaped), float64(res.BytesReclaimed)/(1<<30))
	if len(res.Errors) > 0 {
		fmt.Fprintf(&b, ", %d error(s)", len(res.Errors))
	}
	return b.String()
}

// defaultBuildCacheMinAge is how long an abandoned cache must sit untouched
// before it is reapable. A day is far longer than any single build, review,
// or test run on this host, so nothing in flight can be caught, while the
// steady-state accumulation is still bounded to roughly one day's worth.
const defaultBuildCacheMinAge = 24 * time.Hour

// defaultBuildCacheDepth covers both shapes seen on the host: a cache created
// directly under the temp root (GOCACHE=/tmp/<name>) and one nested a single
// level inside a per-task scratch directory (GOCACHE=/tmp/<task>/gocache).
const defaultBuildCacheDepth = 2

// defaultBuildCacheRoots names the temp directories agents actually wrote
// their private caches into: the shared /tmp and the per-user TMPDIR macOS
// hands out under /var/folders. Both were carrying real caches.
func defaultBuildCacheRoots() string {
	roots := []string{"/tmp"}
	if tmp := os.TempDir(); tmp != "" && filepath.Clean(tmp) != "/tmp" {
		roots = append(roots, tmp)
	}
	return strings.Join(roots, ",")
}

// reapAbandonedBuildCaches runs one reap pass for a watchdog tick, honouring
// --dry-run. It returns nil when the reaper is disabled so the report and the
// JSON stay silent about a feature the operator turned off.
func reapAbandonedBuildCaches(cfg config) *buildCacheReapResult {
	if strings.TrimSpace(cfg.buildCacheRoots) == "" {
		return nil
	}
	var roots []string
	seen := map[string]bool{}
	for r := range strings.SplitSeq(cfg.buildCacheRoots, ",") {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		// Dedup after resolving: on macOS /tmp is a symlink to /private/tmp,
		// so the two spellings would otherwise scan the same tree twice and
		// double-count the bytes reclaimed.
		key := r
		if resolved, err := filepath.EvalSymlinks(r); err == nil {
			key = resolved
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		roots = append(roots, r)
	}
	res := reapBuildCaches(buildCacheConfig{
		Roots:    roots,
		MaxDepth: cfg.buildCacheDepth,
		MinAge:   cfg.buildCacheMinAge,
		Reap:     !cfg.dryRun,
	})
	return &res
}
