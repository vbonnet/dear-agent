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

const defaultCap = 20

// openPR is a lightweight summary of an open PR from gh pr list.
type openPR struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	HeadRefName string `json:"headRefName"`
	Body        string `json:"body"`
}

// listOpenPRs fetches all open PRs for the repo (up to 200).
func listOpenPRs(repo string) ([]openPR, error) {
	args := []string{
		"pr", "list",
		"--state", "open",
		"--json", "number,title,headRefName,body",
		"--limit", "200",
	}
	if repo != "" {
		args = append(args, "--repo", repo)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", args...) //nolint:gosec // args controlled by caller
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("gh pr list timed out after 30s")
		}
		return nil, fmt.Errorf("gh pr list: %w (stderr: %s)", err, stderr.String())
	}

	var prs []openPR
	if err := json.Unmarshal(stdout.Bytes(), &prs); err != nil {
		return nil, fmt.Errorf("parse gh output: %w", err)
	}
	return prs, nil
}

// checkBeadDedup returns any open PRs that already reference beadID.
// Returns nil if beadID is empty (no check needed).
func checkBeadDedup(prs []openPR, beadID string) []openPR {
	if beadID == "" {
		return nil
	}
	lower := strings.ToLower(beadID)
	var claims []openPR
	for _, pr := range prs {
		if prContainsToken(pr, lower) {
			claims = append(claims, pr)
		}
	}
	return claims
}

// prContainsToken returns true if the PR title, branch, or body contains beadID
// as a whole token (surrounded by non-alphanumeric chars or at boundaries).
func prContainsToken(pr openPR, needle string) bool {
	for _, field := range []string{pr.Title, pr.HeadRefName, pr.Body} {
		if containsWholeToken(strings.ToLower(field), needle) {
			return true
		}
	}
	return false
}

// containsWholeToken returns true if needle appears in haystack as a whole token.
func containsWholeToken(haystack, needle string) bool {
	idx := 0
	for {
		pos := strings.Index(haystack[idx:], needle)
		if pos < 0 {
			return false
		}
		abs := idx + pos
		before := abs == 0 || !isWordChar(rune(haystack[abs-1]))
		after := abs+len(needle) >= len(haystack) || !isWordChar(rune(haystack[abs+len(needle)]))
		if before && after {
			return true
		}
		idx = abs + 1
		if idx >= len(haystack) {
			return false
		}
	}
}

func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// detectRepo auto-detects the GitHub repo from the current git remote.
func detectRepo() (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	rawURL := strings.TrimSpace(string(out))
	rawURL = strings.TrimSuffix(rawURL, ".git")
	if idx := strings.LastIndex(rawURL, "github.com/"); idx >= 0 {
		return rawURL[idx+len("github.com/"):], nil
	}
	if idx := strings.LastIndex(rawURL, "github.com:"); idx >= 0 {
		return rawURL[idx+len("github.com:"):], nil
	}
	return "", fmt.Errorf("cannot parse GitHub repo from remote URL: %q", rawURL)
}
