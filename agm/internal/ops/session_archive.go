package ops

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
	"github.com/vbonnet/dear-agent/agm/internal/delegation"
	"github.com/vbonnet/dear-agent/agm/internal/gclog"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/mcp"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// ArchiveSessionRequest defines the input for archiving a session.
type ArchiveSessionRequest struct {
	// Identifier is a session ID, name, or UUID prefix.
	Identifier string `json:"identifier"`
	// Force skips pre-archive verification checks.
	Force bool `json:"force,omitempty"`
	// KeepSandbox preserves the sandbox directory for debugging instead of removing it.
	KeepSandbox bool `json:"keep_sandbox,omitempty"`
}

// ArchiveSessionResult is the output of ArchiveSession.
type ArchiveSessionResult struct {
	Operation           string                          `json:"operation"`
	SessionID           string                          `json:"session_id"`
	Name                string                          `json:"name"`
	PreviousStatus      string                          `json:"previous_status"`
	DryRun              bool                            `json:"dry_run,omitempty"`
	Verification        *session.CompletionVerification `json:"verification,omitempty"`
	MCPProcessesCleaned int                             `json:"mcp_processes_cleaned,omitempty"`
	SandboxCleaned      bool                            `json:"sandbox_cleaned,omitempty"`
	PendingDelegations  int                             `json:"pending_delegations,omitempty"`
	PostCleanup         *CleanupResult                  `json:"post_cleanup,omitempty"`
}

// ArchiveSession marks a session as archived.
// If ctx.DryRun is true, returns what would happen without executing.
// If verification finds critical issues and Force is false, returns an error.
func ArchiveSession(ctx *OpContext, req *ArchiveSessionRequest) (*ArchiveSessionResult, error) {
	if req == nil || req.Identifier == "" {
		return nil, ErrInvalidInput("identifier", "Session identifier is required. Provide a session ID, name, or UUID prefix.")
	}

	// Resolve session
	m, err := ctx.Storage.GetSession(req.Identifier)
	if err != nil {
		// Try name-based lookup
		m, err = findByName(ctx, req.Identifier)
		if err != nil {
			return nil, err
		}
	}
	if m == nil {
		return nil, ErrSessionNotFound(req.Identifier)
	}

	// Check if already archived
	if m.Lifecycle == manifest.LifecycleArchived {
		return nil, ErrSessionArchived(m.Name)
	}

	// Check for supervisor session protection unless --force is set
	if !req.Force && IsSupervisorSession(m.Name) {
		return nil, &OpError{
			Status:      403,
			Type:        "archive/supervisor_protected",
			Code:        ErrCodeVerificationFailed,
			Title:       "Cannot archive protected supervisor session",
			Detail:      fmt.Sprintf("Session '%s' is a protected supervisor session. Use --force to override.", m.Name),
			Suggestions: []string{"use --force to override supervisor protection"},
		}
	}

	// Check for active tmux session unless --force is set
	if !req.Force && m.Tmux.SessionName != "" {
		cmd := exec.Command("tmux", "has-session", "-t", m.Tmux.SessionName)
		if err := cmd.Run(); err == nil {
			// Session exists and is active
			return nil, &OpError{
				Status:      400,
				Type:        "archive/active_tmux_session",
				Code:        ErrCodeVerificationFailed,
				Title:       "Cannot archive session with active tmux pane",
				Detail:      fmt.Sprintf("cannot archive session with active tmux pane — use --force to override"),
				Suggestions: []string{"use --force to override and archive anyway"},
			}
		}
	}

	// Compute current status for the result
	previousStatus := computeSessionStatus(m, ctx.Tmux)

	// Run completion verification against working directory
	dir := m.WorkingDirectory
	if dir == "" {
		dir = m.Context.Project
	}
	verification := session.VerifyCompletion(dir)

	// Block on critical verification failures unless --force
	if verification.Critical() && !req.Force {
		errs := verification.CriticalErrors()
		detail := fmt.Sprintf("Cannot archive: %s. Fix and retry, or use --force to override.", strings.Join(errs, "; "))
		return nil, &OpError{
			Status:      400,
			Type:        "archive/verification_failed",
			Code:        ErrCodeVerificationFailed,
			Title:       "Pre-archive verification failed",
			Detail:      detail,
			Suggestions: append(errs, "use --force to skip verification checks"),
		}
	}

	// Check for pending delegations (warn, block unless --force)
	if delegationDir, err := delegation.DefaultDir(); err == nil {
		if tracker, err := delegation.NewTracker(delegationDir); err == nil {
			if pending, err := tracker.Pending(m.Name); err == nil && len(pending) > 0 {
				summaries := make([]string, 0, len(pending))
				for _, d := range pending {
					s := d.TaskSummary
					if len(s) > 80 {
						s = s[:77] + "..."
					}
					summaries = append(summaries, fmt.Sprintf("→ %s: %s [ID: %s]", d.To, s, d.MessageID))
				}
				if !req.Force {
					detail := fmt.Sprintf("Cannot archive: %d pending delegation(s) have not been resolved:\n%s\n\nResolve with: agm delegation resolve-all %s\nOr use --force to override.",
						len(pending), strings.Join(summaries, "\n"), m.Name)
					return nil, &OpError{
						Status:      400,
						Type:        "archive/pending_delegations",
						Code:        ErrCodeVerificationFailed,
						Title:       "Pending delegations block archive",
						Detail:      detail,
						Suggestions: []string{"resolve delegations first", "use --force to skip"},
					}
				}
				slog.Warn("Archiving with pending delegations", "session", m.Name, "count", len(pending))
			}
		}
	}

	// Dry run: return what would happen
	if ctx.DryRun {
		return &ArchiveSessionResult{
			Operation:      "archive_session",
			SessionID:      m.SessionID,
			Name:           m.Name,
			PreviousStatus: previousStatus,
			DryRun:         true,
			Verification:   verification,
		}, nil
	}

	// Update lifecycle
	m.Lifecycle = manifest.LifecycleArchived
	m.UpdatedAt = time.Now()

	if err := ctx.Storage.UpdateSession(m); err != nil {
		return nil, ErrStorageError("archive_session", err)
	}

	// Best-effort: record trust event based on commit presence.
	recordArchiveTrust(m.Name, m.WorkingDirectory, m.Context.Project, m.SessionID, m.CreatedAt)

	// Best-effort: deregister this session from all monitor lists
	deregisterMonitor(ctx, m.Name)

	// Best-effort MCP process cleanup
	sandboxPath := ""
	if m.Sandbox != nil {
		sandboxPath = m.Sandbox.MergedPath
	}
	mcpKilled, mcpErr := mcp.CleanupSessionMCPProcesses(
		&mcp.ProcFSFinder{}, &mcp.SignalKiller{},
		m.SessionID, sandboxPath,
	)
	if mcpErr != nil {
		slog.Warn("MCP cleanup error during archive", "session", m.SessionID, "error", mcpErr)
	}
	if mcpKilled > 0 {
		slog.Info("Cleaned up MCP processes during archive", "session", m.SessionID, "killed", mcpKilled)
	}

	// Best-effort: Kill process group before sandbox deletion to prevent zombie processes
	if m.Tmux.SessionName != "" {
		pidOut, err := exec.Command("tmux", "display-message", "-t", m.Tmux.SessionName, "-p", "#{pane_pid}").Output()
		if err == nil {
			pid := strings.TrimSpace(string(pidOut))
			if pid != "" {
				// Kill the entire process group (negative PID kills all processes in that group)
				exec.Command("kill", "-TERM", "-"+pid).Run()
				time.Sleep(contracts.Load().SessionLifecycle.ProcessKillGracePeriod.Duration)
				slog.Info("Killed process group during archive", "session", m.SessionID, "pane_pid", pid)
			}
		}
		// Kill tmux session
		exec.Command("tmux", "kill-session", "-t", m.Tmux.SessionName).Run()
		slog.Info("Killed tmux session during archive", "session", m.SessionID, "tmux", m.Tmux.SessionName)
	}

	// Post-archive resource cleanup: worktrees, branches, sandbox.
	// WorkingDirectory is the sandbox merged path or repo root.
	// The session name is used as the branch name by convention.
	worktreePath := m.WorkingDirectory
	repoPath := m.Context.Project
	branchName := m.Name // session branch typically matches session name

	postCleanup := CleanupAfterArchive(
		m.SessionID, m.Name,
		worktreePath, repoPath, sandboxPath, branchName,
		req.KeepSandbox,
	)

	// Also log sandbox cleanup to gc.jsonl for backward compatibility
	if postCleanup.SandboxRemoved {
		logGCEntry(gclog.Entry{
			Operation:   "archive_sandbox_cleanup",
			SessionID:   m.SessionID,
			SessionName: m.Name,
		})
	}

	// Best-effort: Remove sandbox path from Claude's additionalDirectories
	if sandboxPath != "" {
		if err := removeFromAdditionalDirectories(sandboxPath); err != nil {
			slog.Warn("Failed to remove sandbox from additionalDirectories", "session", m.SessionID, "path", sandboxPath, "error", err)
		}
	}

	return &ArchiveSessionResult{
		Operation:           "archive_session",
		SessionID:           m.SessionID,
		Name:                m.Name,
		PreviousStatus:      previousStatus,
		Verification:        verification,
		MCPProcessesCleaned: mcpKilled,
		SandboxCleaned:      postCleanup.SandboxRemoved,
		PostCleanup:         postCleanup,
	}, nil
}

// cleanupSandboxDir unmounts and removes the sandbox directory for a session.
// Returns true if a sandbox directory was found and removed.
// Preserves .claude/settings.local.json from the overlay upper layer before
// removal, as it contains RBAC permission rules that should not be lost.
func cleanupSandboxDir(sessionID, mergedPath string) bool {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		slog.Warn("Failed to get home dir for sandbox cleanup", "error", err)
		return false
	}

	sandboxDir := filepath.Join(homeDir, ".agm", "sandboxes", sessionID)
	if _, err := os.Stat(sandboxDir); os.IsNotExist(err) {
		return false
	}

	// Preserve .claude/settings.local.json from the upper layer before removal.
	// This file contains RBAC permission rules written by ConfigureProjectPermissions.
	preserveSettingsFromUpper(sandboxDir)

	// Unmount merged path if known
	if mergedPath != "" {
		if err := unmountBestEffort(mergedPath); err != nil {
			slog.Warn("Failed to unmount sandbox", "path", mergedPath, "error", err)
		}
	}

	if err := os.RemoveAll(sandboxDir); err != nil {
		slog.Warn("Failed to remove sandbox directory", "path", sandboxDir, "error", err)
		return false
	}

	slog.Info("Removed sandbox directory", "session", sessionID, "path", sandboxDir)
	return true
}


// preserveSettingsFromUpper copies .claude/settings.local.json from the sandbox
// upper layer back to the first repo that has a .claude/ directory. This prevents
// RBAC permission rules from being lost when the sandbox is cleaned up.
func preserveSettingsFromUpper(sandboxDir string) {
	upperSettings := filepath.Join(sandboxDir, "upper", ".claude", "settings.local.json")
	data, err := os.ReadFile(upperSettings)
	if err != nil {
		return // file doesn't exist in upper layer - nothing to preserve
	}

	// Find the original repo's .claude/ directory from the merged symlinks.
	// The merged dir contains symlinks pointing to the original repos.
	mergedClaude := filepath.Join(sandboxDir, "merged", ".claude")
	target, err := os.Readlink(mergedClaude)
	if err != nil {
		// Not a symlink (kernel overlayfs or already unmounted).
		slog.Debug("Cannot determine repo .claude/ path for settings preservation",
			"sandbox", sandboxDir)
		return
	}

	destPath := filepath.Join(target, "settings.local.json")
	if err := os.WriteFile(destPath, data, 0600); err != nil {
		slog.Warn("Failed to preserve settings.local.json to repo",
			"dest", destPath, "error", err)
	} else {
		slog.Info("Preserved settings.local.json from sandbox upper layer",
			"dest", destPath)
	}
}

// CleanupOrphanedSandboxesRequest defines input for orphaned sandbox cleanup.
type CleanupOrphanedSandboxesRequest struct {
	DryRun bool `json:"dry_run,omitempty"`
}

// CleanupOrphanedSandboxesResult is the output of CleanupOrphanedSandboxes.
type CleanupOrphanedSandboxesResult struct {
	Operation string   `json:"operation"`
	Scanned   int      `json:"scanned"`
	Removed   int      `json:"removed"`
	Errors    int      `json:"errors"`
	Orphans   []string `json:"orphans,omitempty"`
	DryRun    bool     `json:"dry_run,omitempty"`
}

// CleanupOrphanedSandboxes removes sandbox directories that have no matching active session.
func CleanupOrphanedSandboxes(ctx *OpContext, req *CleanupOrphanedSandboxesRequest) (*CleanupOrphanedSandboxesResult, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	sandboxBase := filepath.Join(homeDir, ".agm", "sandboxes")
	entries, err := os.ReadDir(sandboxBase)
	if err != nil {
		if os.IsNotExist(err) {
			return &CleanupOrphanedSandboxesResult{Operation: "cleanup_orphaned_sandboxes"}, nil
		}
		return nil, fmt.Errorf("failed to read sandboxes directory: %w", err)
	}

	// Build set of active (non-archived) session IDs
	sessions, err := ctx.Storage.ListSessions(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	activeIDs := make(map[string]bool, len(sessions))
	for _, s := range sessions {
		if s.Lifecycle != manifest.LifecycleArchived {
			activeIDs[s.SessionID] = true
		}
	}

	result := &CleanupOrphanedSandboxesResult{
		Operation: "cleanup_orphaned_sandboxes",
		DryRun:    req.DryRun,
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		result.Scanned++
		sessionID := entry.Name()

		if activeIDs[sessionID] {
			continue
		}

		// This sandbox has no matching active session — it's orphaned
		sandboxDir := filepath.Join(sandboxBase, sessionID)
		result.Orphans = append(result.Orphans, sessionID)

		if req.DryRun {
			continue
		}

		// Unmount merged path before removal
		mergedPath := filepath.Join(sandboxDir, "merged")
		if err := unmountBestEffort(mergedPath); err != nil {
			slog.Warn("Failed to unmount orphaned sandbox", "path", mergedPath, "error", err)
		}

		if err := os.RemoveAll(sandboxDir); err != nil {
			slog.Warn("Failed to remove orphaned sandbox", "path", sandboxDir, "error", err)
			result.Errors++
			continue
		}

		result.Removed++
		slog.Info("Removed orphaned sandbox", "session_id", sessionID)
		logGCEntry(gclog.Entry{
			Operation:      "cleanup_orphaned_sandbox",
			SessionID:      sessionID,
			SandboxRemoved: sandboxDir,
		})
	}

	return result, nil
}

// logGCEntry writes a best-effort entry to the gc.jsonl log.
func logGCEntry(entry gclog.Entry) {
	logger, err := gclog.NewDefault()
	if err != nil {
		slog.Warn("Failed to create gc logger", "error", err)
		return
	}
	if err := logger.Log(entry); err != nil {
		slog.Warn("Failed to write gc log entry", "error", err)
	}
}

// recordArchiveTrust counts branch commits and records a success or
// false_completion trust event. All errors are logged and swallowed — trust
// recording must never block the archive operation.
func recordArchiveTrust(sessionName, workDir, projectDir, sessionID string, createdAt time.Time) {
	dir := workDir
	if dir == "" {
		dir = projectDir
	}

	runner := &execGitRunner{workDir: dir}

	// Prefer agm/<uuid> branch convention; fall back to session name.
	branch := "agm/" + sessionID
	if _, err := runner.run("rev-parse", "--verify", branch); err != nil {
		branch = sessionName
	}

	commits, _ := getCommits(runner, branch)
	duration := time.Since(createdAt).Round(time.Second)

	var eventType TrustEventType
	var detail string
	if len(commits) > 0 {
		eventType = TrustEventSuccess
		detail = fmt.Sprintf("commits: %d, duration: %s", len(commits), duration)
	} else {
		eventType = TrustEventFalseCompletion
		detail = fmt.Sprintf("no commits, duration: %s", duration)

		recordErrorMemory(
			"session archived with no commits",
			ErrMemCatFalseCompletion,
			fmt.Sprintf("session=%s branch=%s duration=%s", sessionName, branch, duration),
			"Review session logs; may indicate early termination or permission blocks",
			SourceAGMArchive,
			sessionName,
		)
	}

	if err := RecordTrustEventForSession(sessionName, eventType, detail); err != nil {
		slog.Warn("Failed to record archive trust event", "session", sessionName, "error", err)
	}
}

// deregisterMonitor removes the given session name from all other sessions' monitor lists.
func deregisterMonitor(ctx *OpContext, sessionName string) {
	sessions, err := ctx.Storage.ListSessions(nil)
	if err != nil {
		slog.Warn("Failed to list sessions for monitor deregistration", "error", err)
		return
	}

	for _, s := range sessions {
		if len(s.Monitors) == 0 {
			continue
		}
		monitors := make([]string, 0, len(s.Monitors))
		found := false
		for _, mon := range s.Monitors {
			if mon == sessionName {
				found = true
				continue
			}
			monitors = append(monitors, mon)
		}
		if found {
			s.Monitors = monitors
			if err := ctx.Storage.UpdateSession(s); err != nil {
				slog.Warn("Failed to deregister monitor from session",
					"session", s.Name, "monitor", sessionName, "error", err)
			} else {
				slog.Info("Deregistered monitor on archive",
					"session", s.Name, "monitor", sessionName)
			}
		}
	}
}

