package ops

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// GenerateSessionSummaryRequest defines the input for generating a session summary report.
type GenerateSessionSummaryRequest struct {
	// SessionName is the name (or identifier) of the session to summarize.
	SessionName string `json:"session_name"`
}

// CommitInfo describes a single commit on the session's branch.
type CommitInfo struct {
	Hash      string `json:"hash"`
	Subject   string `json:"subject"`
	Author    string `json:"author"`
	Timestamp string `json:"timestamp"`
}

// FileStats summarizes the files changed across all commits.
type FileStats struct {
	TotalFiles   int `json:"total_files"`
	LinesAdded   int `json:"lines_added"`
	LinesRemoved int `json:"lines_removed"`
}

// GenerateSessionSummaryResult is the output of GenerateSessionSummary.
type GenerateSessionSummaryResult struct {
	Operation   string        `json:"operation"`
	SessionName string        `json:"session_name"`
	Branch      string        `json:"branch"`
	Commits     []CommitInfo  `json:"commits"`
	CommitCount int           `json:"commit_count"`
	Files       FileStats     `json:"files"`
	Duration    *DurationInfo `json:"duration,omitempty"`
	Trust       *TrustSummary `json:"trust,omitempty"`
	Cost        *CostSummary  `json:"cost,omitempty"`
}

// DurationInfo captures session timing.
type DurationInfo struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Elapsed   string `json:"elapsed"`
}

// TrustSummary includes trust score and event counts.
type TrustSummary struct {
	Score       int          `json:"score"`
	TotalEvents int          `json:"total_events"`
	Events      []TrustEvent `json:"events,omitempty"`
}

// CostSummary includes token usage and estimated cost.
type CostSummary struct {
	TokensIn      int64   `json:"tokens_in"`
	TokensOut     int64   `json:"tokens_out"`
	EstimatedCost float64 `json:"estimated_cost"`
}

// gitRunner abstracts git command execution for testability.
type gitRunner interface {
	run(args ...string) (string, error)
}

// execGitRunner runs real git commands.
type execGitRunner struct {
	workDir string
}

func (r *execGitRunner) run(args ...string) (string, error) {
	if r.workDir != "" {
		args = append([]string{"-C", r.workDir}, args...)
	}
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// GenerateSessionSummary generates a structured post-session summary report.
func GenerateSessionSummary(ctx *OpContext, req *GenerateSessionSummaryRequest) (*GenerateSessionSummaryResult, error) {
	if req == nil || req.SessionName == "" {
		return nil, ErrInvalidInput("session_name", "Session name is required.")
	}

	// Look up session manifest
	m, err := findByName(ctx, req.SessionName)
	if err != nil {
		return nil, err
	}

	workDir := m.WorkingDirectory
	if workDir == "" {
		workDir = m.Context.Project
	}

	runner := &execGitRunner{workDir: workDir}

	// Detect branch — prefer agm/<session-id> convention, fall back to session name
	branch := "agm/" + m.SessionID
	if _, branchErr := runner.run("rev-parse", "--verify", branch); branchErr != nil {
		branch = m.Name
		if _, branchErr2 := runner.run("rev-parse", "--verify", branch); branchErr2 != nil {
			if cur, curErr := runner.run("rev-parse", "--abbrev-ref", "HEAD"); curErr == nil {
				branch = cur
			}
		}
	}

	// Build cost summary from manifest
	var cost *CostSummary
	if m.CostTracking != nil && (m.CostTracking.TokensIn > 0 || m.CostTracking.TokensOut > 0) {
		cost = &CostSummary{
			TokensIn:      m.CostTracking.TokensIn,
			TokensOut:     m.CostTracking.TokensOut,
			EstimatedCost: estimateCostFromTokens(m.CostTracking.TokensIn, m.CostTracking.TokensOut),
		}
	} else if m.LastKnownCost > 0 {
		cost = &CostSummary{
			EstimatedCost: m.LastKnownCost,
		}
	}

	return buildSessionSummary(ctx, req.SessionName, branch, runner, m.CreatedAt, cost)
}

// Approximate Opus pricing per million tokens (USD).
const (
	summaryInputPricePerMillion  = 15.0
	summaryOutputPricePerMillion = 75.0
)

// estimateCostFromTokens approximates cost using Opus pricing.
func estimateCostFromTokens(tokensIn, tokensOut int64) float64 {
	inCost := float64(tokensIn) / 1_000_000 * summaryInputPricePerMillion
	outCost := float64(tokensOut) / 1_000_000 * summaryOutputPricePerMillion
	return inCost + outCost
}

// buildSessionSummary assembles the summary from git data, trust events, and cost.
func buildSessionSummary(ctx *OpContext, sessionName, branch string, git gitRunner, createdAt time.Time, cost *CostSummary) (*GenerateSessionSummaryResult, error) {
	// 1. Get commits on this branch but not on main
	commits, err := getCommits(git, branch)
	if err != nil {
		return nil, ErrStorageError("git-log", fmt.Errorf("failed to get commits: %w", err))
	}

	// 2. Get file change stats
	files, err := getFileStats(git, branch)
	if err != nil {
		files = FileStats{}
	}

	// 3. Compute duration
	var dur *DurationInfo
	if len(commits) > 0 {
		dur = computeDuration(createdAt, commits)
	}

	// 4. Get trust events
	var trust *TrustSummary
	trustResult, trustErr := TrustScore(ctx, &TrustScoreRequest{SessionName: sessionName})
	if trustErr == nil && trustResult.TotalEvents > 0 {
		events, _ := readTrustEvents(sessionName)
		trust = &TrustSummary{
			Score:       trustResult.Score,
			TotalEvents: trustResult.TotalEvents,
			Events:      events,
		}
	}

	return &GenerateSessionSummaryResult{
		Operation:   "session_summary",
		SessionName: sessionName,
		Branch:      branch,
		Commits:     commits,
		CommitCount: len(commits),
		Files:       files,
		Duration:    dur,
		Trust:       trust,
		Cost:        cost,
	}, nil
}

// getCommits returns commits on branch that aren't on main.
func getCommits(git gitRunner, branch string) ([]CommitInfo, error) {
	out, err := git.run("log", "main.."+branch, "--format=%H\t%s\t%an\t%aI", "--reverse")
	if err != nil {
		out, err = git.run("log", "origin/main.."+branch, "--format=%H\t%s\t%an\t%aI", "--reverse")
		if err != nil {
			return nil, fmt.Errorf("git log failed: %w", err)
		}
	}

	if out == "" {
		return nil, nil
	}

	var commits []CommitInfo
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 4 {
			continue
		}
		hash := parts[0]
		if len(hash) > 12 {
			hash = hash[:12]
		}
		commits = append(commits, CommitInfo{
			Hash:      hash,
			Subject:   parts[1],
			Author:    parts[2],
			Timestamp: parts[3],
		})
	}
	return commits, nil
}

// getFileStats returns aggregate file change stats for the branch.
func getFileStats(git gitRunner, branch string) (FileStats, error) {
	out, err := git.run("diff", "--stat", "main..."+branch)
	if err != nil {
		out, err = git.run("diff", "--stat", "origin/main..."+branch)
		if err != nil {
			return FileStats{}, fmt.Errorf("git diff --stat failed: %w", err)
		}
	}

	return parseFileStats(out), nil
}

// parseFileStats extracts file count, additions, and deletions from git diff --stat output.
func parseFileStats(statOutput string) FileStats {
	lines := strings.Split(strings.TrimSpace(statOutput), "\n")
	if len(lines) == 0 {
		return FileStats{}
	}

	// The summary line is the last line, like:
	// " 3 files changed, 120 insertions(+), 15 deletions(-)"
	summary := lines[len(lines)-1]
	stats := FileStats{}

	// Parse "N files changed"
	if idx := strings.Index(summary, " file"); idx > 0 {
		numStr := strings.TrimSpace(summary[:idx])
		if n, err := strconv.Atoi(numStr); err == nil {
			stats.TotalFiles = n
		}
	}

	// Parse "N insertions(+)"
	if idx := strings.Index(summary, " insertion"); idx > 0 {
		sub := summary[:idx]
		parts := strings.Split(sub, ",")
		last := strings.TrimSpace(parts[len(parts)-1])
		if n, err := strconv.Atoi(last); err == nil {
			stats.LinesAdded = n
		}
	}

	// Parse "N deletions(-)"
	if idx := strings.Index(summary, " deletion"); idx > 0 {
		sub := summary[:idx]
		parts := strings.Split(sub, ",")
		last := strings.TrimSpace(parts[len(parts)-1])
		if n, err := strconv.Atoi(last); err == nil {
			stats.LinesRemoved = n
		}
	}

	return stats
}

// computeDuration calculates elapsed time from session start to last commit.
func computeDuration(sessionStart time.Time, commits []CommitInfo) *DurationInfo {
	if len(commits) == 0 {
		return nil
	}

	lastCommit := commits[len(commits)-1]
	endTime, err := time.Parse(time.RFC3339, lastCommit.Timestamp)
	if err != nil {
		return nil
	}

	startTime := sessionStart
	firstTime, err := time.Parse(time.RFC3339, commits[0].Timestamp)
	if err == nil && firstTime.Before(startTime) {
		startTime = firstTime
	}

	elapsed := endTime.Sub(startTime)

	return &DurationInfo{
		StartTime: startTime.Format(time.RFC3339),
		EndTime:   endTime.Format(time.RFC3339),
		Elapsed:   formatElapsed(elapsed),
	}
}

// formatElapsed formats a duration as a human-readable string.
func formatElapsed(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
