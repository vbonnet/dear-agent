package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// git runs `git -C dir args...` and returns trimmed stdout. stderr is folded
// into the error for diagnosis.
func git(dir string, args ...string) (string, error) {
	full := append([]string{"-C", dir}, args...)
	out, err := exec.Command("git", full...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// isGitRepo reports whether dir is the top level of a git working tree.
func isGitRepo(dir string) bool {
	out, err := git(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

// DiscoverRepos returns the immediate child directories of root that are git
// repositories, sorted by name. Non-directories, dotfiles, and non-repos are
// skipped silently.
func DiscoverRepos(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read root %s: %w", root, err)
	}
	var repos []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		p := filepath.Join(root, e.Name())
		if isGitRepo(p) {
			repos = append(repos, p)
		}
	}
	sort.Strings(repos)
	return repos, nil
}

// resolveBaseRef picks the ref branches are measured against: origin/HEAD,
// then origin/main, origin/master, main, master. Returns "" if none resolve.
func resolveBaseRef(dir string) string {
	if ref, err := git(dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil && ref != "" {
		return ref
	}
	for _, ref := range []string{"origin/main", "origin/master", "main", "master"} {
		if git0(dir, "rev-parse", "--verify", "--quiet", ref) {
			return ref
		}
	}
	return ""
}

// git0 reports whether `git -C dir args...` exits 0 (output ignored).
func git0(dir string, args ...string) bool {
	full := append([]string{"-C", dir}, args...)
	return exec.Command("git", full...).Run() == nil
}

// commitTime returns the committer timestamp of rev, or the zero time on error.
func commitTime(dir, rev string) time.Time {
	out, err := git(dir, "show", "-s", "--format=%ct", rev)
	if err != nil {
		return time.Time{}
	}
	// %ct may be multi-line if rev resolves oddly; take the first field.
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return time.Time{}
	}
	sec, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}

// remoteBranchSet returns the set of branch names that exist under
// refs/remotes/origin (without the "origin/" prefix). HEAD is excluded.
func remoteBranchSet(dir string) map[string]bool {
	set := map[string]bool{}
	out, err := git(dir, "for-each-ref", "--format=%(refname:short)", "refs/remotes/origin")
	if err != nil {
		return set
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		name := strings.TrimPrefix(line, "origin/")
		if name == "" || name == "HEAD" {
			continue
		}
		set[name] = true
	}
	return set
}

// mergedBranchSet returns local branch names merged into base (per
// `git branch --merged base`), with leading markers stripped.
func mergedBranchSet(dir, base string) map[string]bool {
	set := map[string]bool{}
	if base == "" {
		return set
	}
	out, err := git(dir, "branch", "--merged", base, "--format=%(refname:short)")
	if err != nil {
		return set
	}
	for _, line := range strings.Split(out, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			set[name] = true
		}
	}
	return set
}

// aheadBehind returns commits (ahead, behind) of branch relative to base via
// `git rev-list --left-right --count base...branch`. On error returns (-1, -1).
func aheadBehind(dir, base, branch string) (ahead, behind int) {
	if base == "" {
		return -1, -1
	}
	out, err := git(dir, "rev-list", "--left-right", "--count", base+"..."+branch)
	if err != nil {
		return -1, -1
	}
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return -1, -1
	}
	behind, _ = strconv.Atoi(fields[0]) // left = base side
	ahead, _ = strconv.Atoi(fields[1])  // right = branch side
	return ahead, behind
}

// worktreeBranches maps branch name -> true for branches checked out in any
// worktree, parsed from `git worktree list --porcelain`.
func collectWorktrees(dir string, remotes map[string]bool) ([]Worktree, map[string]bool) {
	out, err := git(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, nil
	}
	var (
		wts        []Worktree
		checkedOut = map[string]bool{}
		cur        Worktree
		curHEAD    string
		first      = true
		flush      = func() {
			if cur.Path == "" {
				return
			}
			cur.IsMain = first
			first = false
			if cur.Branch != "" {
				checkedOut[cur.Branch] = true
				cur.HasRemote = remotes[cur.Branch]
			}
			if curHEAD != "" {
				cur.LastCommit = commitTime(dir, curHEAD)
			}
			wts = append(wts, cur)
			cur, curHEAD = Worktree{}, ""
		}
	)
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			curHEAD = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		}
	}
	flush()
	return wts, checkedOut
}

// CollectRepo gathers all audit-relevant data for a single repository.
func CollectRepo(dir string) RepoData {
	base := resolveBaseRef(dir)
	remotes := remoteBranchSet(dir)
	merged := mergedBranchSet(dir, base)
	wts, checkedOut := collectWorktrees(dir, remotes)

	// Base branch's short name (strip "origin/") so we can flag IsBase.
	baseShort := strings.TrimPrefix(base, "origin/")

	rd := RepoData{
		Name:      filepath.Base(dir),
		Path:      dir,
		BaseRef:   base,
		Worktrees: wts,
	}

	out, err := git(dir, "for-each-ref", "--format=%(refname:short)%09%(committerdate:unix)", "refs/heads")
	if err == nil && out != "" {
		for _, line := range strings.Split(out, "\n") {
			fields := strings.Split(line, "\t")
			if len(fields) < 2 {
				continue
			}
			name := strings.TrimSpace(fields[0])
			if name == "" {
				continue
			}
			var last time.Time
			if sec, e := strconv.ParseInt(strings.TrimSpace(fields[1]), 10, 64); e == nil {
				last = time.Unix(sec, 0)
			}
			ahead, behind := aheadBehind(dir, base, name)
			rd.Branches = append(rd.Branches, Branch{
				Name:       name,
				LastCommit: last,
				Ahead:      ahead,
				Behind:     behind,
				Merged:     merged[name] && name != baseShort,
				HasRemote:  remotes[name],
				IsBase:     name == baseShort,
				CheckedOut: checkedOut[name],
			})
		}
	}
	return rd
}
