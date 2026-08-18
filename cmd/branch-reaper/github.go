// github.go holds every subprocess this tool shells out to -- git for refs
// and deletions, gh for pull-request history -- each one bounded by its own
// deadline.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// branchInfo is one remote branch and its tip commit SHA.
type branchInfo struct {
	Name   string
	TipSHA string
}

// branchFieldSep is NUL because it is the one byte a git ref name can never
// contain (git check-ref-format accepts `|`, so a pipe-delimited record is
// ambiguous for a branch literally named `foo|bar`).
const branchFieldSep = "\x00"

// listBranches enumerates refs/remotes/origin/* branches (excluding the
// remote HEAD symref) with their tip commit SHA.
func listBranches(ctx context.Context) ([]branchInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, repoQueryTimeout)
	defer cancel()
	// #nosec G204,G702 -- fixed "git" binary, no user-controlled arguments.
	cmd := exec.CommandContext(ctx, "git", "for-each-ref",
		"--format=%(refname)%00%(objectname)", "refs/remotes/origin")
	out, err := runCombined(cmd)
	if err != nil {
		return nil, err
	}
	return parseBranchList(out), nil
}

// parseBranchList parses `git for-each-ref
// --format=%(refname)%00%(objectname)` output into branchInfo entries,
// dropping the remote HEAD symref and stripping the
// "refs/remotes/origin/" prefix down to a plain branch name.
func parseBranchList(out string) []branchInfo {
	var branches []branchInfo
	for line := range strings.SplitSeq(out, "\n") {
		if line == "" {
			continue
		}
		refname, sha, ok := strings.Cut(line, branchFieldSep)
		if !ok || sha == "" {
			continue
		}
		if refname == "refs/remotes/origin/HEAD" {
			continue
		}
		name := strings.TrimPrefix(refname, "refs/remotes/origin/")
		if name == "" || name == refname {
			continue
		}
		branches = append(branches, branchInfo{Name: name, TipSHA: sha})
	}
	return branches
}

// deleteBranch deletes b from origin, leased to the exact tip SHA that was
// classified. If anything landed on the branch since, git rejects the push
// with "stale info" rather than destroying the unseen commits.
func deleteBranch(ctx context.Context, b branchInfo) error {
	pushCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()
	// #nosec G204,G702 -- fixed "git" binary; branch comes from a prior
	// `git for-each-ref` listing of this repo's own remote refs, not
	// external input.
	cmd := exec.CommandContext(pushCtx, "git", "push", "origin",
		"--force-with-lease="+b.Name+":"+b.TipSHA, "--delete", b.Name)
	if _, err := runCombined(cmd); err != nil {
		// The requested end state may already hold: GitHub's own
		// delete-on-merge, or a human, can remove the ref between
		// enumeration and this push, and git then errors because there is
		// nothing to delete. Reporting that as a failure would claim the
		// branch is still on the remote when it is not.
		//
		// Confirm under the CALLER's context, not pushCtx: one way to reach
		// this branch is the push blowing its own deadline after the remote
		// already applied the delete, and an expired context would make the
		// confirmation fail instantly and mislabel the outcome.
		if gone, checkErr := remoteBranchAbsent(ctx, b.Name); checkErr == nil && gone {
			return nil
		}
		return err
	}
	return nil
}

// remoteBranchAbsent reports whether branch no longer exists on origin.
func remoteBranchAbsent(ctx context.Context, branch string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, repoQueryTimeout)
	defer cancel()
	// #nosec G204,G702 -- fixed "git" binary; branch comes from this repo's
	// own remote refs, not external input.
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--heads", "origin",
		"refs/heads/"+branch)
	out, err := runCombined(cmd)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "", nil
}

// confirmNoOpenPRs re-verifies, immediately before a deletion, that nothing
// open depends on branch -- neither a PR opened FROM it nor one based ON
// it. Any open PR, or any failure to find out, is an error: the caller must
// treat "cannot confirm" exactly like "not safe".
func confirmNoOpenPRs(ctx context.Context, repo, branch string) error {
	prs, err := fetchPRs(ctx, repo, branch)
	if err != nil {
		return fmt.Errorf("recheck head PRs: %w", err)
	}
	for _, pr := range prs {
		if pr.State == "OPEN" {
			return fmt.Errorf("pull request #%d was opened from this branch since classification", pr.Number)
		}
	}
	basePRs, err := fetchOpenBasePRs(ctx, repo, branch)
	if err != nil {
		return fmt.Errorf("recheck base PRs: %w", err)
	}
	if basePRs > 0 {
		return fmt.Errorf("%d open pull request(s) now target this branch as their base", basePRs)
	}
	return nil
}

// fetchPRs returns branch's PR history for PRs opened from this repository.
// Fork PRs are dropped: `gh pr list --head` filters on head branch *name*
// only, so a fork's same-named branch would otherwise be read as this
// branch's history.
func fetchPRs(ctx context.Context, repo, branch string) ([]prRecord, error) {
	// One stalled `gh` call must not hold up every remaining branch: without
	// a per-call deadline a hung subprocess runs until the job timeout kills
	// the whole run with no report at all, which is exactly the "one bad
	// lookup denies results for everything else" failure the lookup_failed
	// bucket exists to prevent. Deadline expiry surfaces as an ordinary
	// lookup error, so the branch lands in lookup_failed like any other.
	ctx, cancel := context.WithTimeout(ctx, prFetchTimeout)
	defer cancel()
	// #nosec G204,G702 -- fixed "gh" binary; repo/branch are argv elements, not
	// shell input.
	cmd := exec.CommandContext(ctx, "gh", "pr", "list",
		"--repo", repo, "--head", branch, "--state", "all",
		"--json", "number,state,mergedAt,headRefOid,isCrossRepository,baseRefName",
		"--limit", fmt.Sprint(prFetchLimit))
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if msg := bytes.TrimSpace(errBuf.Bytes()); len(msg) > 0 {
			return nil, fmt.Errorf("gh pr list: %w: %s", err, msg)
		}
		return nil, fmt.Errorf("gh pr list: %w", err)
	}
	raw := bytes.TrimSpace(out.Bytes())
	if len(raw) == 0 {
		return nil, nil
	}
	var prs []prRecord
	if err := json.Unmarshal(raw, &prs); err != nil {
		return nil, fmt.Errorf("parse pr list: %w", err)
	}
	if len(prs) >= prFetchLimit {
		return nil, fmt.Errorf("more than %d pull requests reference this branch; "+
			"classification would be based on a truncated history", prFetchLimit)
	}
	return sameRepoPRs(prs), nil
}

// fetchOpenBasePRs counts the open pull requests that target branch as
// their BASE. Unlike fetchPRs this deliberately does NOT drop fork PRs: a
// fork's PR based on this branch breaks just as badly when the base
// disappears.
func fetchOpenBasePRs(ctx context.Context, repo, branch string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, prFetchTimeout)
	defer cancel()
	// #nosec G204,G702 -- fixed "gh" binary; repo/branch are argv elements, not
	// shell input.
	cmd := exec.CommandContext(ctx, "gh", "pr", "list",
		"--repo", repo, "--base", branch, "--state", "open",
		"--json", "number", "--limit", fmt.Sprint(prFetchLimit))
	out, err := runCombined(cmd)
	if err != nil {
		return 0, fmt.Errorf("gh pr list --base: %w", err)
	}
	raw := strings.TrimSpace(out)
	if raw == "" {
		return 0, nil
	}
	var prs []prRecord
	if err := json.Unmarshal([]byte(raw), &prs); err != nil {
		return 0, fmt.Errorf("parse base pr list: %w", err)
	}
	return len(prs), nil
}

// sameRepoPRs drops fork-originated pull requests.
func sameRepoPRs(prs []prRecord) []prRecord {
	kept := make([]prRecord, 0, len(prs))
	for _, pr := range prs {
		if pr.IsCrossRepository {
			continue
		}
		kept = append(kept, pr)
	}
	return kept
}

// detectRepo infers owner/repo via `gh repo view`.
func detectRepo(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, repoQueryTimeout)
	defer cancel()
	// #nosec G204,G702 -- fixed "gh" binary, no user-controlled arguments.
	cmd := exec.CommandContext(ctx, "gh", "repo", "view",
		"--json", "nameWithOwner", "--jq", ".nameWithOwner")
	out, err := runCombined(cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// defaultBranch returns repo's default branch name via `gh repo view`.
func defaultBranch(ctx context.Context, repo string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, repoQueryTimeout)
	defer cancel()
	// #nosec G204,G702 -- fixed "gh" binary; repo is an argv element, not
	// shell input.
	// The repository is POSITIONAL here: `gh repo view [<repository>]` has
	// no --repo flag and rejects one outright, which would have made every
	// default-branch lookup fail.
	cmd := exec.CommandContext(ctx, "gh", "repo", "view", repo,
		"--json", "defaultBranchRef", "--jq", ".defaultBranchRef.name")
	out, err := runCombined(cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// originRemote returns the `origin` remote's host and owner/repo.
func originPushURLs(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, repoQueryTimeout)
	defer cancel()
	// --push --all: `remote.origin.pushurl` overrides the fetch URL for
	// pushes, and there can be several. Since deletion is a push, the push
	// URLs are the ones that decide where a ref actually dies -- validating
	// the fetch URL alone would confirm one repository while reaping
	// another.
	// #nosec G204,G702 -- fixed "git" binary, no user-controlled arguments.
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "--push", "--all", "origin")
	out, err := runCombined(cmd)
	if err != nil {
		return nil, err
	}
	var urls []string
	for line := range strings.SplitSeq(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			urls = append(urls, line)
		}
	}
	if len(urls) == 0 {
		return nil, errors.New("origin has no push URL")
	}
	return urls, nil
}

// parseRemote splits a git remote URL into its host and its owner/repo,
// covering the https, ssh and scp-like forms git accepts. Either half comes
// back empty when it cannot be determined, which callers must treat as
// "cannot prove a match" -- a filesystem path remote, for instance, has no
// host and so can never be confirmed to be the GitHub repository whose PR
// history was read.
func parseRemote(url string) (host, ownerRepo string) {
	url = strings.TrimSuffix(strings.TrimSpace(url), "/")
	url = strings.TrimSuffix(url, ".git")
	if url == "" {
		return "", ""
	}
	switch {
	case strings.Contains(url, "://"):
		_, rest, _ := strings.Cut(url, "://")
		host, url, _ = strings.Cut(rest, "/")
	case strings.Contains(url, ":"):
		// scp-like "git@host:owner/repo".
		host, url, _ = strings.Cut(url, ":")
	}
	// Strip any "user@" and ":port" decoration from the host.
	if _, after, ok := strings.Cut(host, "@"); ok {
		host = after
	}
	if before, _, ok := strings.Cut(host, ":"); ok {
		host = before
	}
	parts := strings.Split(url, "/")
	if len(parts) < 2 {
		return host, ""
	}
	owner, name := parts[len(parts)-2], parts[len(parts)-1]
	if owner == "" || name == "" {
		return host, ""
	}
	return host, owner + "/" + name
}

// ghHost is the GitHub host `gh` is talking to, so the origin check
// compares the same identity `gh pr list` resolved against.
func ghHost() string {
	if h := strings.TrimSpace(os.Getenv("GH_HOST")); h != "" {
		return h
	}
	return "github.com"
}

// sameRepository reports whether an origin remote names the same repository
// as ownerRepo on the GitHub host in play. Host is part of the identity:
// `gitlab.com/acme/project` and `github.com/acme/project` share an
// owner/repo path and are entirely different repositories.
func sameRepository(originHost, originOwnerRepo, ownerRepo string) bool {
	if originHost == "" || originOwnerRepo == "" || ownerRepo == "" {
		return false
	}
	return strings.EqualFold(originHost, ghHost()) && strings.EqualFold(originOwnerRepo, ownerRepo)
}

// runCombined runs cmd and returns stdout, folding stderr into the error.
func runCombined(cmd *exec.Cmd) (string, error) {
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if msg := bytes.TrimSpace(errBuf.Bytes()); len(msg) > 0 {
			return "", fmt.Errorf("%s: %w: %s", cmd.Args[0], err, msg)
		}
		return "", fmt.Errorf("%s: %w", cmd.Args[0], err)
	}
	return out.String(), nil
}
