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

type GuardConfig struct {
	BeadID   string
	Repo     string
	BeadsDir string
	Force    bool
}

type GuardResult struct {
	BeadID     string
	Title      string
	PRs        []int
	UnmergedPR []UnmergedPR
	Passed     bool
	Forced     bool
}

type UnmergedPR struct {
	Number int
	State  string
}

type prViewResult struct {
	Number   int    `json:"number"`
	State    string `json:"state"`
	MergedAt string `json:"mergedAt"`
}

var prRE = regexp.MustCompile(`(\w*)#(\d+)`)

func CheckDoD(cfg GuardConfig, stderr io.Writer) (GuardResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	detail, err := bdShow(ctx, cfg.BeadsDir, cfg.BeadID)
	if err != nil {
		return GuardResult{}, fmt.Errorf("reading bead %s: %w", cfg.BeadID, err)
	}

	prNums := extractPRNumbers(detail)
	result := GuardResult{
		BeadID: cfg.BeadID,
		Title:  extractTitle(detail),
		PRs:    prNums,
		Forced: cfg.Force,
	}

	if len(prNums) == 0 {
		result.Passed = true
		return result, nil
	}

	for _, n := range prNums {
		merged, state, err := isPRMerged(ctx, cfg.Repo, n)
		if err != nil {
			fmt.Fprintf(stderr, "  warn: cannot check PR #%d: %v\n", n, err)
			continue
		}
		if !merged {
			result.UnmergedPR = append(result.UnmergedPR, UnmergedPR{Number: n, State: state})
		}
	}

	result.Passed = len(result.UnmergedPR) == 0 || cfg.Force
	return result, nil
}

func FormatResult(r GuardResult, w io.Writer) {
	if len(r.PRs) == 0 {
		fmt.Fprintf(w, "ok: bead %s has no PR references — close is allowed\n", r.BeadID)
		return
	}

	if len(r.UnmergedPR) == 0 {
		fmt.Fprintf(w, "ok: bead %s — all %d referenced PR(s) are merged\n", r.BeadID, len(r.PRs))
		return
	}

	prList := make([]string, len(r.UnmergedPR))
	for i, pr := range r.UnmergedPR {
		prList[i] = fmt.Sprintf("#%d (%s)", pr.Number, pr.State)
	}

	if r.Forced {
		fmt.Fprintf(w, "OVERRIDE: bead %s has unmerged PR(s) %s but --force was specified\n",
			r.BeadID, strings.Join(prList, ", "))
		return
	}

	fmt.Fprintf(w, "BLOCKED: cannot close bead %s — PR(s) not yet merged: %s\n",
		r.BeadID, strings.Join(prList, ", "))
	fmt.Fprintf(w, "\nDefinition of Done requires all referenced PRs to be merged to main.\n")
	fmt.Fprintf(w, "See CLAUDE.md §Agent Delegation Enforcement §6.\n\n")
	fmt.Fprintf(w, "To fix:\n")
	for _, pr := range r.UnmergedPR {
		fmt.Fprintf(w, "  • Merge PR #%d first, then close the bead\n", pr.Number)
	}
	fmt.Fprintf(w, "  • Or use --force if abandoning this bead (not completing it)\n")
}

func extractPRNumbers(text string) []int {
	seen := make(map[int]bool)
	var nums []int
	for _, m := range prRE.FindAllStringSubmatch(text, -1) {
		if m[1] != "" {
			continue
		}
		var n int
		for _, c := range m[2] {
			n = n*10 + int(c-'0')
		}
		if !seen[n] && n > 0 {
			seen[n] = true
			nums = append(nums, n)
		}
	}
	return nums
}

func extractTitle(text string) string {
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			return line
		}
		rest := strings.Join(parts[2:], " ")
		if idx := strings.Index(rest, " · "); idx >= 0 {
			rest = rest[idx+3:]
		}
		if idx := strings.LastIndex(rest, " ["); idx >= 0 {
			rest = rest[:idx]
		}
		return strings.TrimSpace(rest)
	}
	return "(unknown)"
}

func isPRMerged(ctx context.Context, repo string, prNum int) (bool, string, error) {
	args := []string{"pr", "view", fmt.Sprintf("%d", prNum), "--json", "state,mergedAt,number"}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return false, "", fmt.Errorf("gh pr view %d: %w (stderr: %s)", prNum, err, stderr.String())
	}

	var v prViewResult
	if err := json.Unmarshal(stdout.Bytes(), &v); err != nil {
		return false, "", fmt.Errorf("parse gh output: %w", err)
	}

	return strings.EqualFold(v.State, "MERGED"), v.State, nil
}

func bdShow(ctx context.Context, beadsDir, id string) (string, error) {
	args := []string{"show", id}
	if beadsDir != "" {
		args = append([]string{"--db", beadsDir}, args...)
	}
	cmd := exec.CommandContext(ctx, "bd", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("bd show %s: %w (stderr: %s)", id, err, stderr.String())
	}
	return stdout.String(), nil
}

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
