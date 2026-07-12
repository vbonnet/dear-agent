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
	ah.BDD, ah.FeaturesTotal, ah.FeaturesExecutable = bddCoverage(sc)
	ah.EARS, ah.ImplementationDirsTotal, ah.ImplementationDirsWithSpec = earsCoverage(sc)
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

// bddCoverage counts feature files and how many live in the canonical directory
// executed by the tag-free godog suite. ADR-027 retired @implemented tags.
func bddCoverage(sc *scanCtx) (Metric, int, int) {
	canonicalDir := filepath.Clean(filepath.Join(sc.root, "agm", "test", "bdd", "features"))
	var total, executable int
	err := walkRepoFiles(sc.root, func(path string) {
		if !strings.HasSuffix(path, ".feature") {
			return
		}
		total++
		if filepath.Clean(filepath.Dir(path)) == canonicalDir {
			executable++
		}
	})
	if err != nil {
		return Metric{Available: false, Note: "feature scan failed: " + err.Error()}, 0, 0
	}
	if total == 0 {
		return Metric{Available: false, Note: "no .feature files found"}, 0, 0
	}
	return Metric{Available: true}, total, executable
}

// earsCoverage counts implementation directories across supported source and
// runtime configuration formats and reports how many ship a SPEC.md.
func earsCoverage(sc *scanCtx) (Metric, int, int) {
	implementationDirs := map[string]bool{}
	err := walkRepoFiles(sc.root, func(path string) {
		if isHealthImplementationSource(path) {
			implementationDirs[filepath.Dir(path)] = true
		}
	})
	if err != nil {
		return Metric{Available: false, Note: "implementation scan failed: " + err.Error()}, 0, 0
	}
	if len(implementationDirs) == 0 {
		return Metric{Available: false, Note: "no implementation directories found"}, 0, 0
	}
	withSpec := 0
	for dir := range implementationDirs {
		if _, err := os.Stat(filepath.Join(dir, "SPEC.md")); err == nil {
			withSpec++
		}
	}
	return Metric{Available: true}, len(implementationDirs), withSpec
}

func isHealthImplementationSource(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".rs", ".py",
		".sh", ".bash", ".zsh", ".bats", ".tf", ".sql", ".yaml", ".yml",
		".json", ".toml", ".plist", ".service", ".dockerfile":
		return true
	}
	if filepath.Ext(path) != "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}
