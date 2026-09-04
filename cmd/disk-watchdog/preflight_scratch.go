package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Abandoned preflight runs that build an isolated HOME or GOCACHE leave multi-GB
// scratch directories behind that nothing reaped (ce-m6j1y).
//
// They accumulated under:
//   - ~/.cache/dear-agent/preflight-tmp/ (standard isolated preflight runs)
//   - ~/.cache/dear-agent/preflight-runs/ (prior task preflight runs)
//   - Legacy dotdirs and test scratch in $HOME (~/.ce68, ~/.preflight-home-*,
//     ~/.preflight-*, ~/ce1209-preflight*, ~/.tmp)
//   - Legacy preflight dirs in ~/.cache (ce*-preflight*, dear-agent-preflight-*,
//     ce-*-host-tmp*)
//
// Like build caches, preflight scratch directories are reaped on every tick,
// regardless of disk pressure: abandoned scratch older than the age gate has
// no value, and bounding their age prevents disk exhaustion before it occurs.

const defaultPreflightScratchMinAge = 24 * time.Hour

var (
	legacyHomePreflightRegex  = regexp.MustCompile(`^(\.preflight-home-|\.preflight-|ce[0-9]+-preflight|\.ce[0-9]+$)`)
	legacyCachePreflightRegex = regexp.MustCompile(`^(ce[0-9]+-preflight|dear-agent-preflight-|ce-.*-host-tmp)`)
	tmpTaskEntryRegex         = regexp.MustCompile(`^(ce-|go-tmp-|h5\.|p5\.)`)
)

// defaultPreflightScratchRoots resolves the default roots scanned for abandoned
// preflight scratch.
func defaultPreflightScratchRoots() string {
	cacheRoot, err := os.UserCacheDir()
	if err != nil || cacheRoot == "" {
		home, herr := os.UserHomeDir()
		if herr != nil {
			return ""
		}
		cacheRoot = filepath.Join(home, ".cache")
	}
	home, _ := os.UserHomeDir()
	var roots []string
	if cacheRoot != "" {
		roots = append(roots,
			filepath.Join(cacheRoot, "dear-agent", "preflight-tmp"),
			filepath.Join(cacheRoot, "dear-agent", "preflight-runs"),
			cacheRoot,
		)
	}
	if home != "" {
		roots = append(roots, home)
	}
	return strings.Join(roots, ",")
}

type preflightScratchConfig struct {
	Roots  []string
	MinAge time.Duration
	Reap   bool

	now    func() time.Time
	inUse  func(path string) (bool, error)
	remove func(path string) error
	sizeOf func(path string) int64
}

type preflightScratchReapResult struct {
	Scanned        int               `json:"scanned"`
	Reaped         []string          `json:"reaped,omitempty"`
	Skipped        map[string]string `json:"skipped,omitempty"`
	BytesReclaimed int64             `json:"bytes_reclaimed"`
	Errors         map[string]string `json:"errors,omitempty"`
}

type preflightScratchGates struct {
	now    func() time.Time
	inUse  func(string) (bool, error)
	remove func(string) error
	sizeOf func(string) int64
}

func removeAllSafe(path string) error {
	err := os.RemoveAll(path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	// On permission error (e.g. read-only Go module caches), make writable and retry
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr == nil && d.IsDir() {
			_ = os.Chmod(p, 0o700) //nolint:gosec // G302: directory traversal requires execute permission
		}
		return nil
	})
	return os.RemoveAll(path)
}

func (cfg preflightScratchConfig) withDefaults() preflightScratchGates {
	g := preflightScratchGates{now: cfg.now, inUse: cfg.inUse, remove: cfg.remove, sizeOf: cfg.sizeOf}
	if g.now == nil {
		g.now = time.Now
	}
	if g.inUse == nil {
		g.inUse = newLsofProber()
	}
	if g.remove == nil {
		g.remove = removeAllSafe
	}
	if g.sizeOf == nil {
		g.sizeOf = dirBytes
	}
	return g
}

func (g preflightScratchGates) keepReason(path string, minAge time.Duration) string {
	info, err := os.Lstat(path)
	if err != nil {
		return "stat failed: " + err.Error()
	}
	if age := g.now().Sub(info.ModTime()); age < minAge {
		return fmt.Sprintf("too recent (%s < %s)", age.Round(time.Second), minAge)
	}
	busy, err := g.inUse(path)
	if err != nil {
		return "liveness probe failed: " + err.Error()
	}
	if busy {
		return "a process holds a file open inside"
	}
	return ""
}

func evalOrClean(path string) string {
	if p, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(p)
	}
	return filepath.Clean(path)
}

func scanTmpSubdirectory(tmpDir string, euid int64) []string {
	subEntries, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil
	}
	var res []string
	allScratch := true
	for _, se := range subEntries {
		subChild := filepath.Join(tmpDir, se.Name())
		sinfo, slerr := os.Lstat(subChild)
		if slerr != nil || sinfo.Mode()&os.ModeSymlink != 0 || !sinfo.IsDir() {
			allScratch = false
			continue
		}
		if tmpTaskEntryRegex.MatchString(se.Name()) && isOwner(sinfo, euid) {
			res = append(res, subChild)
		} else {
			allScratch = false
		}
	}
	if allScratch && len(subEntries) > 0 {
		res = append(res, tmpDir)
	}
	return res
}

func isPreflightCandidate(normRoot, normHome, normCache, name string) bool {
	if normRoot == normHome {
		return legacyHomePreflightRegex.MatchString(name)
	}
	if normRoot == normCache {
		return legacyCachePreflightRegex.MatchString(name)
	}
	return true
}

// findPreflightScratchDirs discovers candidate scratch directories beneath root.
func findPreflightScratchDirs(root string, homeDir, cacheDir string, euid int64) ([]string, error) {
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("resolve preflight scratch root %q: %w", root, err)
	}

	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, err
	}

	normRoot := evalOrClean(resolved)
	normHome := evalOrClean(homeDir)
	normCache := evalOrClean(cacheDir)

	var found []string
	for _, e := range entries {
		name := e.Name()
		child := filepath.Join(resolved, name)

		info, lerr := os.Lstat(child)
		if lerr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !isOwner(info, euid) {
			continue
		}

		if normRoot == normHome && name == ".tmp" {
			found = append(found, scanTmpSubdirectory(child, euid)...)
			continue
		}

		if !isPreflightCandidate(normRoot, normHome, normCache, name) {
			continue
		}

		found = append(found, child)
	}

	sort.Strings(found)
	return found, nil
}

// reapPreflightScratch finds and removes abandoned preflight scratch directories.
func reapPreflightScratch(cfg preflightScratchConfig) preflightScratchReapResult {
	res := preflightScratchReapResult{Skipped: map[string]string{}, Errors: map[string]string{}}
	g := cfg.withDefaults()

	homeDir, _ := os.UserHomeDir()
	cacheDir, _ := os.UserCacheDir()
	if cacheDir == "" && homeDir != "" {
		cacheDir = filepath.Join(homeDir, ".cache")
	}
	if h, herr := filepath.EvalSymlinks(homeDir); herr == nil {
		homeDir = h
	}
	if c, cerr := filepath.EvalSymlinks(cacheDir); cerr == nil {
		cacheDir = c
	}
	euid := int64(os.Geteuid())

	seen := map[string]bool{}
	for _, root := range cfg.Roots {
		candidates, err := findPreflightScratchDirs(root, homeDir, cacheDir, euid)
		if err != nil {
			res.Errors[root] = err.Error()
			continue
		}
		for _, cand := range candidates {
			if seen[cand] {
				continue
			}
			seen[cand] = true
			res.Scanned++

			if reason := g.keepReason(cand, cfg.MinAge); reason != "" {
				res.Skipped[cand] = reason
				continue
			}
			size := g.sizeOf(cand)
			if !cfg.Reap {
				res.Skipped[cand] = "scan only"
				res.BytesReclaimed += size
				continue
			}
			if err := g.remove(cand); err != nil {
				res.Errors[cand] = err.Error()
				continue
			}
			res.Reaped = append(res.Reaped, cand)
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

func summarizePreflightScratchReap(res preflightScratchReapResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "preflight scratch: %d found, %d reaped, %.1f GiB",
		res.Scanned, len(res.Reaped), float64(res.BytesReclaimed)/(1<<30))
	if len(res.Errors) > 0 {
		fmt.Fprintf(&b, ", %d error(s)", len(res.Errors))
	}
	return b.String()
}

func reapAbandonedPreflightScratch(cfg config) *preflightScratchReapResult {
	if strings.TrimSpace(cfg.preflightScratchRoots) == "" {
		return nil
	}
	var roots []string
	seen := map[string]bool{}
	for r := range strings.SplitSeq(cfg.preflightScratchRoots, ",") {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
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
	res := reapPreflightScratch(preflightScratchConfig{
		Roots:  roots,
		MinAge: cfg.preflightScratchMinAge,
		Reap:   !cfg.dryRun,
	})
	return &res
}
