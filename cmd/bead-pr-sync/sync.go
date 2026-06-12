package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Config holds runtime parameters for the reconciler.
type Config struct {
	DryRun   bool
	Repo     string
	BeadsDir string
	Limit    int
}

// Stats reports what was found and fixed.
type Stats struct {
	Scanned    int
	Violations int
	Fixed      int
}

// Violation describes a bead whose PR is not yet merged.
type Violation struct {
	BeadID  string
	Title   string
	PRs     []int
	OpenPRs []int
}

// prView is the JSON shape returned by `gh pr view --json`.
type prView struct {
	Number   int    `json:"number"`
	State    string `json:"state"`
	MergedAt string `json:"mergedAt"`
}

// prRE matches "#NNN" patterns in bead descriptions/comments.
var prRE = regexp.MustCompile(`#(\d+)`)

// Reconcile scans closed beads and reports/fixes DoD violations.
func Reconcile(cfg Config, stdout, stderr io.Writer) (Stats, error) {
	var stats Stats
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ids, err := listClosedBeadIDs(ctx, cfg.BeadsDir, cfg.Limit)
	if err != nil {
		return stats, fmt.Errorf("listing closed beads: %w", err)
	}
	stats.Scanned = len(ids)

	for _, id := range ids {
		if ctx.Err() != nil {
			return stats, ctx.Err()
		}
		detail, err := showBead(ctx, cfg.BeadsDir, id)
		if err != nil {
			fmt.Fprintf(stderr, "  warn: cannot show bead %s: %v\n", id, err)
			continue
		}

		prNums := extractPRNumbers(detail)
		if len(prNums) == 0 {
			continue
		}

		var openPRs []int
		for _, n := range prNums {
			merged, err := isPRMerged(ctx, cfg.Repo, n)
			if err != nil {
				fmt.Fprintf(stderr, "  warn: cannot check PR #%d: %v\n", n, err)
				continue
			}
			if !merged {
				openPRs = append(openPRs, n)
			}
		}
		if len(openPRs) == 0 {
			continue
		}

		stats.Violations++
		v := Violation{BeadID: id, Title: extractTitle(detail), PRs: prNums, OpenPRs: openPRs}
		fmt.Fprintf(stdout, "  VIOLATION %s — %s (open PRs: %v)\n", v.BeadID, v.Title, v.OpenPRs)

		if cfg.DryRun {
			continue
		}

		if err := reopenBead(ctx, cfg.BeadsDir, id, openPRs); err != nil {
			fmt.Fprintf(stderr, "  warn: cannot reopen bead %s: %v\n", id, err)
			continue
		}
		stats.Fixed++
		fmt.Fprintf(stdout, "  → reopened %s to in_progress\n", id)
	}
	return stats, nil
}

// listClosedBeadIDs returns IDs of beads in CLOSED status, up to limit.
func listClosedBeadIDs(ctx context.Context, beadsDir string, limit int) ([]string, error) {
	args := []string{"list", "--status", "closed"}
	out, err := runBd(ctx, beadsDir, args...)
	if err != nil {
		return nil, err
	}

	var ids []string
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Total:") || strings.HasPrefix(line, "Status:") || strings.HasPrefix(line, "---") {
			continue
		}
		// Lines look like: "✓ ce-abc123 · Title [● P1 · CLOSED]"
		// Extract the second whitespace-separated token as the bead ID.
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		id := parts[1]
		// Skip lines that don't look like bead IDs (must contain at least one letter and digit).
		if !isMaybBeadID(id) {
			continue
		}
		ids = append(ids, id)
		if len(ids) >= limit {
			break
		}
	}
	return ids, nil
}

// isMaybBeadID returns true if s looks like a bead ID (e.g. "ce-abc123", "ce-6as.80").
func isMaybBeadID(s string) bool {
	return strings.Contains(s, "-") && len(s) >= 3
}

// showBead returns the full `bd show` output for a bead.
func showBead(ctx context.Context, beadsDir, id string) (string, error) {
	out, err := runBd(ctx, beadsDir, "show", id)
	return string(out), err
}

// extractPRNumbers returns all distinct PR numbers referenced in text.
func extractPRNumbers(text string) []int {
	seen := make(map[int]bool)
	var nums []int
	for _, m := range prRE.FindAllStringSubmatch(text, -1) {
		var n int
		for _, c := range m[1] {
			n = n*10 + int(c-'0')
		}
		if !seen[n] && n > 0 {
			seen[n] = true
			nums = append(nums, n)
		}
	}
	return nums
}

// extractTitle extracts the bead title from `bd show` output.
func extractTitle(text string) string {
	// First non-empty line typically: "✓ ce-abc · Title [...]"
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Strip leading status icon and bead ID, keep the rest up to " ["
		parts := strings.Fields(line)
		if len(parts) < 3 {
			return line
		}
		// Join parts[2:], trimming the trailing "[…]" bracket.
		rest := strings.Join(parts[2:], " ")
		// The format uses "·" as separator
		if idx := strings.Index(rest, " · "); idx >= 0 {
			rest = rest[idx+3:]
		}
		if idx := strings.Index(rest, "   ["); idx >= 0 {
			rest = rest[:idx]
		}
		if idx := strings.LastIndex(rest, " ["); idx >= 0 {
			rest = rest[:idx]
		}
		return strings.TrimSpace(rest)
	}
	return "(unknown)"
}

// isPRMerged returns true if the PR has a non-empty mergedAt timestamp.
func isPRMerged(ctx context.Context, repo string, prNum int) (bool, error) {
	args := []string{"pr", "view", fmt.Sprintf("%d", prNum), "--json", "state,mergedAt,number"}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// PR not found or permission error — treat as unknown, not as unmerged.
		return false, fmt.Errorf("gh pr view %d: %w (stderr: %s)", prNum, err, stderr.String())
	}

	var v prView
	if err := json.Unmarshal(stdout.Bytes(), &v); err != nil {
		return false, fmt.Errorf("parse gh output: %w", err)
	}

	return v.MergedAt != "" && v.MergedAt != "null", nil
}

// reopenBead sets the bead status to in_progress and leaves a comment.
func reopenBead(ctx context.Context, beadsDir, id string, openPRs []int) error {
	if _, err := runBd(ctx, beadsDir, "update", id, "--status", "in_progress"); err != nil {
		return err
	}
	prs := make([]string, len(openPRs))
	for i, n := range openPRs {
		prs[i] = fmt.Sprintf("#%d", n)
	}
	comment := fmt.Sprintf("bead-pr-sync: reopened — PR(s) %s not yet merged to main (DoD violation)", strings.Join(prs, ", "))
	_, err := runBd(ctx, beadsDir, "comment", id, comment)
	return err
}

// runBd executes a bd command and returns stdout.
func runBd(ctx context.Context, beadsDir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "bd", args...)
	if beadsDir != "" {
		cmd.Env = append(cmd.Environ(), "BEADS_DIR="+beadsDir)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("bd %s: %w (stderr: %s)", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// detectRepo tries to infer the GitHub repo from `git remote get-url origin`.
func detectRepo() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
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
// Returns empty string if the URL is not a recognizable GitHub URL.
func extractRepoFromURL(rawURL string) string {
	// Strip trailing .git
	rawURL = strings.TrimSuffix(rawURL, ".git")
	// HTTPS: https://github.com/owner/repo
	if idx := strings.LastIndex(rawURL, "github.com/"); idx >= 0 {
		return rawURL[idx+len("github.com/"):]
	}
	// SSH: git@github.com:owner/repo
	if idx := strings.LastIndex(rawURL, "github.com:"); idx >= 0 {
		return rawURL[idx+len("github.com:"):]
	}
	return ""
}
