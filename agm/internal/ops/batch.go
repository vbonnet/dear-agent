package ops

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// BatchSpawnRequest defines input for spawning multiple workers.
type BatchSpawnRequest struct {
	Workers []WorkerSpec `json:"workers"`
}

// WorkerSpec defines a single worker to spawn.
type WorkerSpec struct {
	Name       string `json:"name" yaml:"name"`
	PromptFile string `json:"prompt-file" yaml:"prompt-file"`
	Model      string `json:"model" yaml:"model"`
}

// BatchSpawnResult holds the outcome of a batch spawn.
type BatchSpawnResult struct {
	Operation string              `json:"operation"`
	Spawned   []SpawnedWorker     `json:"spawned"`
	Failed    []FailedWorker      `json:"failed,omitempty"`
	Summary   BatchSpawnSummary   `json:"summary"`
}

// SpawnedWorker records a successfully spawned worker.
type SpawnedWorker struct {
	Name  string `json:"name"`
	Model string `json:"model"`
}

// FailedWorker records a worker that failed to spawn.
type FailedWorker struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

// BatchSpawnSummary provides counts for the spawn operation.
type BatchSpawnSummary struct {
	Total   int `json:"total"`
	Success int `json:"success"`
	Failed  int `json:"failed"`
}

// BatchStatusRequest defines input for querying worker status.
type BatchStatusRequest struct {
	// Sessions filters to specific session names. Empty means all active.
	Sessions []string `json:"sessions,omitempty"`
}

// BatchStatusResult holds the status of multiple workers.
type BatchStatusResult struct {
	Operation string         `json:"operation"`
	Workers   []WorkerStatus `json:"workers"`
	Total     int            `json:"total"`
}

// WorkerStatus holds the status of a single worker.
type WorkerStatus struct {
	Name     string  `json:"name"`
	State    string  `json:"state"`
	Duration string  `json:"duration"`
	Commits  int     `json:"commits"`
	Branch   string  `json:"branch"`
	Cost     float64 `json:"estimated_cost,omitempty"`
}

// BatchMergeRequest defines input for merging worker commits.
type BatchMergeRequest struct {
	// Sessions to merge from. Empty means all verified workers.
	Sessions []string `json:"sessions,omitempty"`
	// TargetBranch to merge into (default: current branch).
	TargetBranch string `json:"target_branch,omitempty"`
	// RepoDir is the repository directory.
	RepoDir string `json:"repo_dir"`
	// DryRun if true, only report what would be merged.
	DryRun bool `json:"dry_run,omitempty"`
}

// BatchMergeResult holds the outcome of a batch merge.
type BatchMergeResult struct {
	Operation string            `json:"operation"`
	DryRun    bool              `json:"dry_run"`
	Merged    []MergedWorker    `json:"merged"`
	Skipped   []SkippedWorker   `json:"skipped,omitempty"`
	Summary   BatchMergeSummary `json:"summary"`
}

// MergedWorker records a successfully merged worker.
type MergedWorker struct {
	Name    string   `json:"name"`
	Branch  string   `json:"branch"`
	Commits []string `json:"commits"`
}

// SkippedWorker records a worker that was skipped during merge.
type SkippedWorker struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// BatchMergeSummary provides counts for the merge operation.
type BatchMergeSummary struct {
	Total   int `json:"total"`
	Merged  int `json:"merged"`
	Skipped int `json:"skipped"`
}

// BatchStatus returns the status of active workers.
func BatchStatus(ctx *OpContext, req *BatchStatusRequest) (*BatchStatusResult, error) {
	if req == nil {
		req = &BatchStatusRequest{}
	}

	filter := &dolt.SessionFilter{
		ExcludeArchived: true,
	}

	manifests, err := ctx.Storage.ListSessions(filter)
	if err != nil {
		return nil, ErrStorageError("batch_status", err)
	}

	// If specific sessions requested, filter to those
	if len(req.Sessions) > 0 {
		nameSet := make(map[string]bool, len(req.Sessions))
		for _, s := range req.Sessions {
			nameSet[s] = true
		}
		var filtered []*manifest.Manifest
		for _, m := range manifests {
			if nameSet[m.Name] {
				filtered = append(filtered, m)
			}
		}
		manifests = filtered
	}

	// Compute statuses
	statuses := make(map[string]string)
	if ctx.Tmux != nil {
		statuses = computeStatuses(manifests, ctx.Tmux)
	}

	workers := make([]WorkerStatus, 0, len(manifests))
	for _, m := range manifests {
		status := statuses[m.Name]
		if status == "" {
			status = "unknown"
		}

		state := m.State
		if state == "" {
			state = status
		}

		duration := time.Since(m.CreatedAt).Truncate(time.Second).String()

		// Count commits on the worker's branch
		branch := fmt.Sprintf("agm/%s", m.SessionID)
		commits := countBranchCommits(m.Context.Project, branch)

		estCost := m.LastKnownCost
		if estCost == 0 && m.CostTracking != nil {
			inCost := float64(m.CostTracking.TokensIn) / 1_000_000 * 15.0
			outCost := float64(m.CostTracking.TokensOut) / 1_000_000 * 75.0
			estCost = inCost + outCost
		}

		workers = append(workers, WorkerStatus{
			Name:     m.Name,
			State:    state,
			Duration: duration,
			Commits:  commits,
			Branch:   branch,
			Cost:     estCost,
		})
	}

	return &BatchStatusResult{
		Operation: "batch_status",
		Workers:   workers,
		Total:     len(workers),
	}, nil
}

// BatchMerge cherry-picks commits from worker branches to the target branch.
func BatchMerge(ctx *OpContext, req *BatchMergeRequest) (*BatchMergeResult, error) {
	if req == nil {
		return nil, ErrInvalidInput("request", "BatchMergeRequest is required.")
	}
	if req.RepoDir == "" {
		return nil, ErrInvalidInput("repo_dir", "Repository directory is required.")
	}

	filter := &dolt.SessionFilter{
		ExcludeArchived: true,
	}

	manifests, err := ctx.Storage.ListSessions(filter)
	if err != nil {
		return nil, ErrStorageError("batch_merge", err)
	}

	// Filter to requested sessions or all DONE/OFFLINE
	if len(req.Sessions) > 0 {
		nameSet := make(map[string]bool, len(req.Sessions))
		for _, s := range req.Sessions {
			nameSet[s] = true
		}
		var filtered []*manifest.Manifest
		for _, m := range manifests {
			if nameSet[m.Name] {
				filtered = append(filtered, m)
			}
		}
		manifests = filtered
	} else {
		// Default: only merge from DONE or OFFLINE sessions
		var filtered []*manifest.Manifest
		for _, m := range manifests {
			if m.State == manifest.StateDone || m.State == manifest.StateOffline {
				filtered = append(filtered, m)
			}
		}
		manifests = filtered
	}

	mergeStart := time.Now()

	result := &BatchMergeResult{
		Operation: "batch_merge",
		DryRun:    req.DryRun,
	}

	for _, m := range manifests {
		branch := fmt.Sprintf("agm/%s", m.SessionID)

		// Get commits on this branch that are not on target
		commits, err := getBranchCommits(req.RepoDir, branch, req.TargetBranch)
		if err != nil {
			result.Skipped = append(result.Skipped, SkippedWorker{
				Name:   m.Name,
				Reason: fmt.Sprintf("failed to list commits: %v", err),
			})
			result.Summary.Skipped++
			result.Summary.Total++
			continue
		}

		if len(commits) == 0 {
			result.Skipped = append(result.Skipped, SkippedWorker{
				Name:   m.Name,
				Reason: "no commits to merge",
			})
			result.Summary.Skipped++
			result.Summary.Total++
			continue
		}

		if req.DryRun {
			result.Merged = append(result.Merged, MergedWorker{
				Name:    m.Name,
				Branch:  branch,
				Commits: commits,
			})
			result.Summary.Merged++
			result.Summary.Total++
			continue
		}

		// Cherry-pick each commit
		if err := cherryPickCommits(req.RepoDir, commits); err != nil {
			result.Skipped = append(result.Skipped, SkippedWorker{
				Name:   m.Name,
				Reason: fmt.Sprintf("cherry-pick failed: %v", err),
			})
			result.Summary.Skipped++
			result.Summary.Total++
			continue
		}

		result.Merged = append(result.Merged, MergedWorker{
			Name:    m.Name,
			Branch:  branch,
			Commits: commits,
		})
		result.Summary.Merged++
		result.Summary.Total++
	}

	// Record merge duration for observability
	if !req.DryRun {
		_ = RecordMergeDuration(time.Since(mergeStart)) // best-effort
	}

	return result, nil
}

// countBranchCommits counts commits on a branch ahead of main.
func countBranchCommits(repoDir, branch string) int {
	if repoDir == "" {
		return 0
	}
	cmd := exec.Command("git", "-C", repoDir, "rev-list", "--count", "main.."+branch)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	var count int
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &count)
	return count
}

// getBranchCommits returns commit hashes on branch that are not on target.
func getBranchCommits(repoDir, branch, target string) ([]string, error) {
	if target == "" {
		target = "HEAD"
	}
	cmd := exec.Command("git", "-C", repoDir, "rev-list", "--reverse", target+".."+branch)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git rev-list failed: %w", err)
	}
	lines := strings.TrimSpace(string(out))
	if lines == "" {
		return nil, nil
	}
	return strings.Split(lines, "\n"), nil
}

// cherryPickCommits cherry-picks a list of commits in order.
func cherryPickCommits(repoDir string, commits []string) error {
	for _, commit := range commits {
		cmd := exec.Command("git", "-C", repoDir, "cherry-pick", commit)
		if out, err := cmd.CombinedOutput(); err != nil {
			// Abort the cherry-pick on failure
			abortCmd := exec.Command("git", "-C", repoDir, "cherry-pick", "--abort")
			_ = abortCmd.Run()
			return fmt.Errorf("cherry-pick %s failed: %w\n%s", commit[:8], err, string(out))
		}
	}
	return nil
}
