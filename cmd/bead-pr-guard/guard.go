package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// prSummary is the JSON shape returned by `gh pr list --json`.
type prSummary struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	HeadRefName string `json:"headRefName"`
	Body        string `json:"body"`
	State       string `json:"state"`
}

// findClaimingPRs returns all open PRs that reference beadID in their
// title, head branch name, or body.
func findClaimingPRs(repo, beadID string) ([]prSummary, error) {
	args := []string{
		"pr", "list",
		"--state", "open",
		"--json", "number,title,headRefName,body,state",
		"--limit", "200",
	}
	if repo != "" {
		args = append(args, "--repo", repo)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("gh pr list timed out after 30s")
		}
		return nil, fmt.Errorf("gh pr list: %w (stderr: %s)", err, stderr.String())
	}

	var prs []prSummary
	if err := json.Unmarshal(stdout.Bytes(), &prs); err != nil {
		return nil, fmt.Errorf("parse gh output: %w", err)
	}

	var claims []prSummary
	for _, pr := range prs {
		if containsBeadID(pr, beadID) {
			claims = append(claims, pr)
		}
	}
	return claims, nil
}

// containsBeadID reports whether pr references beadID in title, branch, or body.
// Match is case-insensitive and word-boundary-aware: "ce-abc" must appear as
// a distinct token, not as a substring of "ce-abcdef".
func containsBeadID(pr prSummary, beadID string) bool {
	lower := strings.ToLower(beadID)
	for _, field := range []string{pr.Title, pr.HeadRefName, pr.Body} {
		if containsToken(strings.ToLower(field), lower) {
			return true
		}
	}
	return false
}

// containsToken returns true if needle appears in haystack as a whole token
// (surrounded by non-alphanumeric characters or at string boundaries).
func containsToken(haystack, needle string) bool {
	idx := 0
	for {
		pos := strings.Index(haystack[idx:], needle)
		if pos < 0 {
			return false
		}
		abs := idx + pos
		before := abs == 0 || !isAlnum(rune(haystack[abs-1]))
		after := abs+len(needle) >= len(haystack) || !isAlnum(rune(haystack[abs+len(needle)]))
		if before && after {
			return true
		}
		idx = abs + 1
		if idx >= len(haystack) {
			return false
		}
	}
}

func isAlnum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// detectRepo auto-detects the GitHub repo from the current git remote.
func detectRepo() (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	repo := extractRepoFromURL(strings.TrimSpace(string(out)))
	if repo == "" {
		return "", fmt.Errorf("cannot parse GitHub repo from remote URL: %q", strings.TrimSpace(string(out)))
	}
	return repo, nil
}

// extractRepoFromURL parses "owner/repo" from HTTPS or SSH GitHub remote URLs.
func extractRepoFromURL(rawURL string) string {
	rawURL = strings.TrimSuffix(rawURL, ".git")
	if idx := strings.LastIndex(rawURL, "github.com/"); idx >= 0 {
		return rawURL[idx+len("github.com/"):]
	}
	if idx := strings.LastIndex(rawURL, "github.com:"); idx >= 0 {
		return rawURL[idx+len("github.com:"):]
	}
	return ""
}
