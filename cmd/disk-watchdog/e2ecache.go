package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// Abandoned E2E test fixture directories accumulate in the user cache directory
// (~/Library/Caches/dear-agent/e2e or ~/.cache/dear-agent/e2e). Each fixture
// directory contains a full AGM binary (~70 MB) built for a specific test source key.
// Without an external reaper, older fixtures accumulate indefinitely (151 dirs / 10 GB
// in 2 days observed on this host, bead ce-4cp3a).
//
// Like build caches, e2e fixture directories are reaped on every tick, regardless of
// disk pressure: older test fixtures have no value once replaced, so bounding their
// count and age prevents disk pressure before it occurs.

const (
	defaultE2ECacheMinAge     = 24 * time.Hour
	defaultE2ECacheMaxEntries = 5
)

// defaultE2ECacheDir resolves the per-user cache directory for E2E AGM fixtures.
func defaultE2ECacheDir() string {
	cacheRoot, err := os.UserCacheDir()
	if err != nil || cacheRoot == "" {
		home, herr := os.UserHomeDir()
		if herr != nil {
			return ""
		}
		cacheRoot = filepath.Join(home, ".cache")
	}
	return filepath.Join(cacheRoot, "dear-agent", "e2e")
}

type e2eCacheConfig struct {
	Dir        string
	MinAge     time.Duration
	MaxEntries int
	Reap       bool

	now    func() time.Time
	inUse  func(path string) (bool, error)
	remove func(path string) error
	sizeOf func(path string) int64
}

type e2eCacheReapResult struct {
	Scanned        int               `json:"scanned"`
	Reaped         []string          `json:"reaped,omitempty"`
	Skipped        map[string]string `json:"skipped,omitempty"`
	BytesReclaimed int64             `json:"bytes_reclaimed"`
	Errors         map[string]string `json:"errors,omitempty"`
}

type e2eCacheGates struct {
	now    func() time.Time
	inUse  func(string) (bool, error)
	remove func(string) error
	sizeOf func(string) int64
}

func (cfg e2eCacheConfig) withDefaults() e2eCacheGates {
	g := e2eCacheGates{now: cfg.now, inUse: cfg.inUse, remove: cfg.remove, sizeOf: cfg.sizeOf}
	if g.now == nil {
		g.now = time.Now
	}
	if g.inUse == nil {
		lsof := newLsofProber()
		g.inUse = func(path string) (bool, error) {
			return isE2ECacheInUse(path, lsof)
		}
	}
	if g.remove == nil {
		g.remove = removeE2EFixtureDir
	}
	if g.sizeOf == nil {
		g.sizeOf = dirBytes
	}
	return g
}

func removeE2EFixtureDir(path string) (err error) {
	lockPath := filepath.Join(path, "agm.lock")
	lockFile, oerr := os.Open(lockPath)
	if oerr != nil {
		return os.RemoveAll(path)
	}
	defer func() {
		if cerr := lockFile.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("lock active: %w", err)
	}
	return os.RemoveAll(path)
}

func isE2ECacheInUse(path string, lsof func(string) (bool, error)) (bool, error) {
	lockPath := filepath.Join(path, "agm.lock")
	if lockFile, err := os.Open(lockPath); err == nil {
		flockErr := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		_ = lockFile.Close()
		if flockErr != nil {
			return true, nil //nolint:nilerr // flock error signals active lock held by another process
		}
	}
	if lsof != nil {
		return lsof(path)
	}
	return false, nil
}

type e2eCandidate struct {
	name    string
	path    string
	modTime time.Time
}

func isValidE2EFixtureDir(dirPath string, rootInfo os.FileInfo, euid int64) bool {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			return false
		}
		info, err := e.Info()
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return false
		}
		if !sameDevice(info, rootInfo) || !isOwner(info, euid) {
			return false
		}
		if name != "agm" && name != "agm.lock" && !strings.HasPrefix(name, "agm-build-") {
			return false
		}
	}
	return true
}

func resolveE2ECacheDir(dir string) (string, os.FileInfo, int64, error) {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, 0, nil
		}
		return "", nil, 0, fmt.Errorf("resolve e2e cache dir %q: %w", dir, err)
	}
	rootInfo, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, 0, nil
		}
		return "", nil, 0, err
	}
	euid := int64(os.Geteuid())
	if !isOwner(rootInfo, euid) {
		return "", nil, 0, nil
	}
	return resolved, rootInfo, euid, nil
}

func isCandidateEntry(e os.DirEntry, resolved string, rootInfo os.FileInfo, euid int64) (*e2eCandidate, bool) {
	if !e.IsDir() {
		return nil, false
	}
	name := e.Name()
	if !strings.HasPrefix(name, "agm-") || len(name) <= 4 {
		return nil, false
	}
	childPath := filepath.Join(resolved, name)
	info, lerr := os.Lstat(childPath)
	if lerr != nil || info.Mode()&os.ModeSymlink != 0 {
		return nil, false
	}
	if !sameDevice(info, rootInfo) || !isOwner(info, euid) {
		return nil, false
	}
	if !isValidE2EFixtureDir(childPath, rootInfo, euid) {
		return nil, false
	}
	return &e2eCandidate{
		name:    name,
		path:    childPath,
		modTime: info.ModTime(),
	}, true
}

func findE2ECacheCandidates(dir string) ([]e2eCandidate, error) {
	resolved, rootInfo, euid, err := resolveE2ECacheDir(dir)
	if err != nil || rootInfo == nil {
		return nil, err
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, err
	}

	var candidates []e2eCandidate
	for _, e := range entries {
		if c, ok := isCandidateEntry(e, resolved, rootInfo, euid); ok {
			candidates = append(candidates, *c)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime.After(candidates[j].modTime)
	})
	return candidates, nil
}

func reapE2ECaches(cfg e2eCacheConfig) e2eCacheReapResult {
	res := e2eCacheReapResult{Skipped: map[string]string{}, Errors: map[string]string{}}
	g := cfg.withDefaults()

	candidates, err := findE2ECacheCandidates(cfg.Dir)
	if err != nil {
		res.Errors[cfg.Dir] = err.Error()
		return res
	}

	now := g.now()
	for i, c := range candidates {
		res.Scanned++
		age := now.Sub(c.modTime)
		withinLimits := (i < cfg.MaxEntries) && (cfg.MinAge <= 0 || age < cfg.MinAge)
		if withinLimits {
			res.Skipped[c.path] = fmt.Sprintf("within max-entries and age limits (%s < %s)", age.Round(time.Second), cfg.MinAge)
			continue
		}
		busy, inUseErr := g.inUse(c.path)
		if inUseErr != nil {
			res.Skipped[c.path] = "liveness probe failed: " + inUseErr.Error()
			continue
		}
		if busy {
			res.Skipped[c.path] = "a process holds a file open inside"
			continue
		}
		size := g.sizeOf(c.path)
		if !cfg.Reap {
			res.Skipped[c.path] = "scan only"
			res.BytesReclaimed += size
			continue
		}
		if rerr := g.remove(c.path); rerr != nil {
			res.Errors[c.path] = rerr.Error()
			continue
		}
		res.Reaped = append(res.Reaped, c.path)
		res.BytesReclaimed += size
	}

	if len(res.Skipped) == 0 {
		res.Skipped = nil
	}
	if len(res.Errors) == 0 {
		res.Errors = nil
	}
	return res
}

func summarizeE2ECacheReap(res e2eCacheReapResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "e2e caches: %d found, %d reaped, %.1f GiB",
		res.Scanned, len(res.Reaped), float64(res.BytesReclaimed)/(1<<30))
	if len(res.Errors) > 0 {
		fmt.Fprintf(&b, ", %d error(s)", len(res.Errors))
	}
	return b.String()
}

func reapAbandonedE2ECaches(cfg config) *e2eCacheReapResult {
	if strings.TrimSpace(cfg.e2eCacheDir) == "" {
		return nil
	}
	res := reapE2ECaches(e2eCacheConfig{
		Dir:        cfg.e2eCacheDir,
		MinAge:     cfg.e2eCacheMinAge,
		MaxEntries: cfg.e2eCacheMaxEntries,
		Reap:       !cfg.dryRun,
	})
	return &res
}
