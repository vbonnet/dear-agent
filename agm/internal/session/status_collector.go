package session

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/budget"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// SessionStatus represents the status of a single session
type SessionStatus struct {
	Name            string
	SessionID       string
	State           string
	StateSource     string
	Branch          string
	Uncommitted     int
	WorktreePath    string
	TestsPassing    bool // Placeholder - implementation deferred
	Workspace       string
	LastStateUpdate string
	Budget          *budget.Status // Context budget status (nil if no usage data)
}

// CollectStatus gathers status information for a single session.
// Uses ResolveSessionState for hybrid state detection (hook primary,
// terminal fallback) to ensure consistency with send_msg and list commands.
func CollectStatus(m *manifest.Manifest) (*SessionStatus, error) {
	// Resolve state using the canonical hybrid approach
	tmuxName := getTmuxSessionName(m)
	resolvedState := ResolveSessionState(tmuxName, m.State, m.Claude.UUID, m.StateUpdatedAt)

	status := &SessionStatus{
		Name:         m.Name,
		SessionID:    m.SessionID,
		State:        resolvedState,
		StateSource:  m.StateSource,
		Workspace:    m.Workspace,
		WorktreePath: m.Context.Project,
	}

	// Format last state update
	if !m.StateUpdatedAt.IsZero() {
		status.LastStateUpdate = m.StateUpdatedAt.Format("15:04:05")
	} else {
		status.LastStateUpdate = "never"
	}

	// Get git branch (if project directory exists)
	branch, err := getCurrentBranch(m.Context.Project)
	if err == nil {
		status.Branch = branch
	} else {
		status.Branch = "unknown"
	}

	// Get uncommitted file count
	uncommitted, err := getUncommittedCount(m.Context.Project)
	if err == nil {
		status.Uncommitted = uncommitted
	} else {
		status.Uncommitted = -1 // Indicates error
	}

	// Tests status - placeholder for now
	// TODO: Implement test detection in future iteration
	status.TestsPassing = false

	// Check context budget
	if m.ContextUsage != nil {
		tracker := budget.NewTracker()
		bs := tracker.Check(m)
		status.Budget = &bs
	}

	return status, nil
}

// getCurrentBranch returns the current git branch for a directory
func getCurrentBranch(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get branch: %w", err)
	}

	branch := strings.TrimSpace(string(output))
	return branch, nil
}

// getUncommittedCount returns the number of uncommitted files in a directory
func getUncommittedCount(dir string) (int, error) {
	// git status --porcelain shows one line per uncommitted file
	cmd := exec.Command("git", "-C", dir, "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get uncommitted count: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return 0, nil
	}

	return len(lines), nil
}

// IsWorktree checks if a directory is a git worktree (not the main repository)
func IsWorktree(dir string) bool {
	// Git worktrees have a .git file (not directory) pointing to the main repo
	gitPath := filepath.Join(dir, ".git")

	// Check if .git exists and is a file (worktree) or directory (main repo)
	cmd := exec.Command("test", "-f", gitPath)
	err := cmd.Run()
	return err == nil // If .git is a file, it's a worktree
}

// WorkspaceStatus aggregates status across all sessions in a workspace
type WorkspaceStatus struct {
	Workspace       string
	Sessions        []*SessionStatus
	TotalSessions   int
	DoneSessions    int
	WorkingSessions int
}

// AggregateWorkspaceStatus collects status for all sessions in a workspace
func AggregateWorkspaceStatus(adapter dolt.Storage, workspace string) (*WorkspaceStatus, error) {
	// Filter: empty Lifecycle means active sessions only (excludes archived)
	manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions from Dolt: %w", err)
	}

	workspaceStatus := &WorkspaceStatus{
		Workspace: workspace,
		Sessions:  []*SessionStatus{},
	}

	// Filter by workspace and collect status
	for _, m := range manifests {
		// Filter by workspace (if specified)
		if workspace != "" && m.Workspace != workspace {
			continue
		}

		// Collect status for this session
		status, err := CollectStatus(m)
		if err != nil {
			// Log error but continue with other sessions
			continue
		}

		workspaceStatus.Sessions = append(workspaceStatus.Sessions, status)

		// Update counters
		if status.State == manifest.StateDone || status.State == manifest.StateReady {
			workspaceStatus.DoneSessions++
		} else if status.State == manifest.StateWorking || status.State == "THINKING" {
			workspaceStatus.WorkingSessions++
		}
	}

	workspaceStatus.TotalSessions = len(workspaceStatus.Sessions)

	return workspaceStatus, nil
}
