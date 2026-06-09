package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// collectAgentHealth fills the agent-specific section: worktree
// accumulation, stale branches, BDD scenario coverage and EARS spec
// coverage.
func collectAgentHealth(sc *scanCtx) AgentHealth {
	ah := AgentHealth{}
	ah.Worktrees, ah.WorktreeCount = worktreeCount(sc)
	ah.StaleBranches, ah.StaleBranchCount = staleBranches(sc)
	ah.BDD, ah.FeaturesTotal, ah.FeaturesImpl = bddCoverage(sc)
	ah.EARS, ah.PackagesTotal, ah.PackagesWithSpec = earsCoverage(sc)
	return ah
}

// worktreeCount returns the number of *linked* worktrees (excluding the
// primary checkout). Stranded worktrees are the dominant agent-hygiene
// failure mode in this repo, so the raw count is the signal.
func worktreeCount(sc *scanCtx) (Metric, int) {
	res := run(sc.root, sc.opts.gitTimeout, "git", "worktree", "list", "--porcelain")
	if !res.ok() {
		return Metric{Available: false, Note: "git worktree list failed: " + firstLine(res.stderr)}, 0
	}
	n := 0
	for line := range strings.SplitSeq(res.stdout, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			n++
		}
	}
	if n > 0 {
		n-- // first entry is the primary working tree
	}
	return Metric{Available: true}, n
}

// staleBranches counts branches (local + origin remotes) whose tip commit
// is older than opts.staleDays, excluding the protected main/develop refs.
// A remote branch with a same-named local branch is counted once.
func staleBranches(sc *scanCtx) (Metric, int) {
	cutoff := sc.now.AddDate(0, 0, -sc.opts.staleDays).Unix()
	res := run(sc.root, sc.opts.gitTimeout, "git", "for-each-ref",
		"--format=%(refname:short) %(committerdate:unix)",
		"refs/heads", "refs/remotes/origin")
	if !res.ok() {
		return Metric{Available: false, Note: "git for-each-ref failed: " + firstLine(res.stderr)}, 0
	}
	protected := map[string]bool{"main": true, "develop": true, "master": true, "HEAD": true}
	seen := map[string]bool{}
	stale := 0
	for line := range strings.SplitSeq(res.stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		name := strings.TrimPrefix(fields[0], "origin/")
		if protected[name] || seen[name] {
			continue
		}
		ts, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		seen[name] = true
		if ts < cutoff {
			stale++
		}
	}
	return Metric{Available: true}, stale
}

// bddCoverage counts .feature files and how many carry an @implemented tag,
// the repo's convention for "this scenario is backed by real code".
func bddCoverage(sc *scanCtx) (Metric, int, int) {
	var total, impl int
	err := walkRepoFiles(sc.root, func(path string) {
		if !strings.HasSuffix(path, ".feature") {
			return
		}
		total++
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		if strings.Contains(string(data), "@implemented") {
			impl++
		}
	})
	if err != nil {
		return Metric{Available: false, Note: "feature scan failed: " + err.Error()}, 0, 0
	}
	if total == 0 {
		return Metric{Available: false, Note: "no .feature files found"}, 0, 0
	}
	return Metric{Available: true}, total, impl
}

// earsCoverage counts Go package directories and how many ship a SPEC.md
// (the repo's EARS-format spec convention). A "package directory" is any
// directory holding at least one non-test .go file.
func earsCoverage(sc *scanCtx) (Metric, int, int) {
	pkgDirs := map[string]bool{}
	for _, s := range sc.sources {
		if s.isTest {
			continue
		}
		pkgDirs[filepath.Dir(s.path)] = true
	}
	if len(pkgDirs) == 0 {
		return Metric{Available: false, Note: "no Go packages found"}, 0, 0
	}
	withSpec := 0
	for dir := range pkgDirs {
		if _, err := os.Stat(filepath.Join(dir, "SPEC.md")); err == nil {
			withSpec++
		}
	}
	return Metric{Available: true}, len(pkgDirs), withSpec
}
