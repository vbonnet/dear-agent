package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type opts struct {
	repo     string
	fix      bool
	maxAge   int
	preserve map[string]bool
}

type wtRecord struct {
	path     string
	branch   string
	head     string
	bare     bool
	detached bool
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	o, err := parse(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		usage()
		return 1
	}
	if err := git(o.repo, "rev-parse", "--git-dir"); err != nil {
		fmt.Fprintf(os.Stderr, "error: not a git directory: %s\n", o.repo)
		return 2
	}
	target, err := targetRef(o.repo)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	logf("repo: %s", o.repo)
	logf("target ref: %s", target)
	logf("max-age: %d days", o.maxAge)
	if o.fix {
		logf("mode: FIX")
	} else {
		logf("mode: DRY-RUN")
	}
	if err := git(o.repo, "fetch", "--quiet", "origin"); err != nil {
		logf("warning: fetch failed; using cached refs")
	}
	failed := cleanup(o, target)
	if failed > 0 {
		return 3
	}
	return 0
}

func parse(args []string) (opts, error) {
	o := opts{maxAge: 14, preserve: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			usage()
			os.Exit(0)
		case "--fix":
			o.fix = true
		case "--max-age":
			if i+1 >= len(args) {
				return o, errors.New("error: --max-age needs a value")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil {
				return o, fmt.Errorf("error: invalid --max-age: %w", err)
			}
			o.maxAge = n
			i++
		case "--preserve":
			if i+1 >= len(args) {
				return o, errors.New("error: --preserve needs a value")
			}
			o.preserve[args[i+1]] = true
			i++
		default:
			if strings.HasPrefix(args[i], "-") {
				return o, fmt.Errorf("error: unknown flag: %s", args[i])
			}
			if o.repo != "" {
				return o, fmt.Errorf("error: unexpected argument: %s", args[i])
			}
			o.repo = args[i]
		}
	}
	if o.repo == "" {
		return o, errors.New("error: expected exactly one repo path")
	}
	return o, nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: cleanup-worktrees.sh <repo-path> [--fix] [--max-age DAYS] [--preserve NAME ...]")
}

func targetRef(repo string) (string, error) {
	if err := git(repo, "rev-parse", "--verify", "--quiet", "origin/HEAD"); err == nil {
		out, err := gitOut(repo, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
		if err == nil && strings.TrimSpace(out) != "" {
			return strings.TrimSpace(out), nil
		}
	}
	if err := git(repo, "rev-parse", "--verify", "--quiet", "origin/main"); err == nil {
		return "origin/main", nil
	}
	return "", errors.New("error: target ref not found: origin/main")
}

func cleanup(o opts, target string) int {
	out, err := gitOut(o.repo, "worktree", "list", "--porcelain")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: git worktree list failed: %v\n", err)
		return 1
	}
	repoPath := canonicalPath(o.repo)
	now := time.Now().Unix()
	maxAge := int64(o.maxAge) * 86400
	failed := 0
	for _, rec := range parseWorktrees(out) {
		if rec.path == "" || canonicalPath(rec.path) == repoPath || rec.bare || rec.detached {
			continue
		}
		name := filepath.Base(rec.path)
		if o.preserve[name] {
			logf("preserve: %s (--preserve)", name)
			continue
		}
		ahead := gitInt(o.repo, "rev-list", "--count", target+".."+rec.head)
		last := gitInt(o.repo, "log", "-1", "--format=%ct", rec.head)
		reason := ""
		if ahead == 0 {
			reason = fmt.Sprintf("merged-or-empty (0 commits ahead of %s)", target)
		} else if now-int64(last) >= maxAge {
			reason = fmt.Sprintf("idle (%d days since last commit, ahead=%d)", (now-int64(last))/86400, ahead)
		}
		if reason == "" {
			continue
		}
		branch := strings.TrimPrefix(rec.branch, "refs/heads/")
		logf("stale: %s [%s] - %s", name, branch, reason)
		if !o.fix {
			logf("  would: git -C %s worktree remove --force %s", o.repo, rec.path)
			logf("  would: git -C %s branch -D %s", o.repo, branch)
			logf("  would: git -C %s push origin --delete %s", o.repo, branch)
			continue
		}
		if err := git(o.repo, "worktree", "remove", "--force", rec.path); err != nil {
			logf("  FAILED: worktree remove %s", rec.path)
			logf("  skipped branch cleanup for %s because the worktree was preserved", branch)
			failed++
			continue
		}
		if err := git(o.repo, "branch", "-D", branch); err != nil {
			logf("  note: local branch %s already gone or delete failed", branch)
		}
		if err := git(o.repo, "push", "origin", "--delete", branch); err != nil {
			logf("  note: remote branch %s already gone or push failed", branch)
		}
	}
	return failed
}

func canonicalPath(path string) string {
	realPath, err := filepath.EvalSymlinks(path)
	if err == nil {
		return realPath
	}
	absPath, err := filepath.Abs(path)
	if err == nil {
		return absPath
	}
	return path
}

func parseWorktrees(s string) []wtRecord {
	var records []wtRecord
	rec := wtRecord{}
	flush := func() {
		if rec.path != "" {
			records = append(records, rec)
			rec = wtRecord{}
		}
	}
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			flush()
			continue
		}
		fields := strings.SplitN(line, " ", 2)
		key := fields[0]
		val := ""
		if len(fields) == 2 {
			val = fields[1]
		}
		switch key {
		case "worktree":
			rec.path = val
		case "HEAD":
			rec.head = val
		case "branch":
			rec.branch = val
		case "bare":
			rec.bare = true
		case "detached":
			rec.detached = true
		}
	}
	flush()
	return records
}

func git(repo string, args ...string) error {
	_, err := gitOut(repo, args...)
	return err
}

func gitOut(repo string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%w: %s", err, out)
	}
	return string(out), nil
}

func gitInt(repo string, args ...string) int {
	out, err := gitOut(repo, args...)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(out))
	return n
}

func logf(format string, args ...any) {
	fmt.Printf("[cleanup-worktrees] "+format+"\n", args...)
}
