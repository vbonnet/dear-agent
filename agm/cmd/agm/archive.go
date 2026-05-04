package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/cleanup"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	gitpkg "github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	asyncArchive     bool // Spawn background reaper for async archival
	archiveAll       bool
	olderThan        string
	dryRun           bool
	cleanupWorktrees bool
	forceArchive       bool // Skip pre-archive verification checks
	keepSandbox        bool // Preserve sandbox directory for debugging
	includeSupervisors bool // Include supervisor sessions in bulk archive
)

var archiveCmd = &cobra.Command{
	Use:   "archive [session-name]",
	Short: "Archive a Claude session or multiple sessions",
	Long: `Archive a Claude session by marking it as archived.

Archived sessions:
  • Hidden from 'agm session list' (use --all flag to see them)
  • Files are NOT deleted (only metadata updated)
  • Cannot be resumed until restored

Session state determines how archiving works:

  STOPPED sessions (no active tmux session):
    • Archive immediately without any confirmation prompt
    • Do NOT use --async (error if included)
    • agm session archive my-old-session

  ACTIVE sessions (tmux session still running):
    • MUST use --async flag (error if omitted)
    • Spawns a background reaper process to handle graceful shutdown
    • agm session archive --async my-active-session

Error cases:
  • Active session without --async:  "session is active; use --async to archive an active session"
  • Stopped session with --async:    "--async should only be used for active sessions; omit --async for stopped sessions"

To restore an archived session:
  1. Run: agm session list --all
  2. Find session ID
  3. Use: agm session unarchive <session-id>

Examples:
  # Archive a stopped session (no --async needed)
  agm session archive my-old-session

  # Archive an active session (--async required)
  agm session archive --async my-active-session

  # Archive all inactive sessions older than 30 days (preview only)
  agm session archive --all --older-than=30d --dry-run

  # Archive all inactive sessions older than 30 days
  agm session archive --all --older-than=30d

  # Archive all inactive sessions (be careful!)
  agm session archive --all

  # List all sessions including archived
  agm session list --all

  # Archive by tmux session name
  agm session archive claude-5

  # Archive by session ID
  agm session archive session-abc123`,
	Args: cobra.MaximumNArgs(1),
	RunE: archiveSession,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Only complete first argument
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// Get Dolt adapter
		adapter, err := getStorage()
		if err != nil {
			// Fail gracefully - return empty list if can't connect to Dolt
			return []string{}, cobra.ShellCompDirectiveNoFileComp
		}
		defer adapter.Close()

		// List sessions from Dolt (exclude archived)
		filter := &dolt.SessionFilter{
			ExcludeArchived: true,
		}
		sessions, err := adapter.ListSessions(filter)
		if err != nil {
			// Fail gracefully - return empty list if query fails
			return []string{}, cobra.ShellCompDirectiveNoFileComp
		}

		// Build suggestions from non-archived sessions
		var suggestions []string
		for _, m := range sessions {
			// Add tmux name (primary identifier)
			if m.Tmux.SessionName != "" {
				suggestions = append(suggestions, m.Tmux.SessionName)
			}

			// Add manifest name (secondary identifier, if different)
			if m.Name != "" && m.Name != m.Tmux.SessionName {
				suggestions = append(suggestions, m.Name)
			}
		}

		return suggestions, cobra.ShellCompDirectiveNoFileComp
	},
}

func archiveSession(cmd *cobra.Command, args []string) (retErr error) {
	// Audit trail: log archive lifecycle events
	defer func() {
		sessionName := ""
		if len(args) > 0 {
			sessionName = args[0]
		}
		logCommandAudit("session.archive", sessionName, archiveAuditArgs(), retErr)
	}()

	if archiveAll {
		if len(args) > 0 {
			return fmt.Errorf("cannot specify session name with --all flag")
		}
		if asyncArchive {
			return fmt.Errorf("--async flag is not compatible with --all")
		}
		return archiveBulk()
	}

	// Single session archive mode
	if len(args) == 0 {
		return fmt.Errorf("session name required (or use --all for bulk archive)")
	}

	sessionName := args[0]

	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt: %w", err)
	}
	defer cleanup()

	getResult, getErr := ops.GetSession(opCtx, &ops.GetSessionRequest{
		Identifier: sessionName,
	})
	if getErr != nil {
		ui.PrintSessionNotFoundError(sessionName, "Dolt storage")
		return getErr
	}

	if handled, err := handleAlreadyArchivedOrAsync(sessionName, getResult); handled {
		return err
	}

	// Session is stopped - archive via ops layer
	fmt.Printf("Archiving session: %s\n", ui.Bold(getResult.Session.Name))
	fmt.Printf("  Session ID: %s\n", getResult.Session.ID)
	if getResult.Session.Project != "" {
		fmt.Printf("  Project: %s\n", getResult.Session.Project)
	}

	archiveResult, archiveErr := ops.ArchiveSession(opCtx, &ops.ArchiveSessionRequest{
		Identifier:  sessionName,
		Force:       forceArchive,
		KeepSandbox: keepSandbox,
	})
	if archiveErr != nil {
		return handleError(archiveErr)
	}

	ui.PrintSuccess(fmt.Sprintf("Archived session: %s", sessionName))
	fmt.Printf("\nThe session is now hidden from 'agm session list'.\n")
	fmt.Printf("Use 'agm session list --all' to see archived sessions.\n")
	fmt.Printf("\nTo restore: agm session unarchive %s\n", sessionName)

	reportPostCleanup(archiveResult.PostCleanup)
	reportSessionCleanup(runSessionCleanup(sessionName, opCtx))

	// Best-effort cleanup of stale additionalDirectories in Claude settings
	runSettingsCleanup()

	// Clean up merged worktrees if requested (legacy flag, additional to automatic cleanup)
	if cleanupWorktrees && getResult.Session.WorkingDirectory != "" {
		cleanupMergedWorktrees(getResult.Session.WorkingDirectory)
	}

	return nil
}

// archiveAuditArgs builds the audit arg map for the archive command from the
// global archive flags.
func archiveAuditArgs() map[string]string {
	auditArgs := map[string]string{
		"async": fmt.Sprintf("%v", asyncArchive),
		"force": fmt.Sprintf("%v", forceArchive),
	}
	if archiveAll {
		auditArgs["bulk"] = "true"
		if olderThan != "" {
			auditArgs["older_than"] = olderThan
		}
		if dryRun {
			auditArgs["dry_run"] = "true"
		}
	}
	return auditArgs
}

// handleAlreadyArchivedOrAsync handles three early-exit cases for archive:
// session is already archived (no-op), session is active without --async
// (error), or --async is set (spawn reaper). Returns (handled, err) — when
// handled is true the caller should propagate err immediately.
func handleAlreadyArchivedOrAsync(sessionName string, getResult *ops.GetSessionResult) (bool, error) {
	if getResult.Session.Lifecycle == "archived" {
		msg := fmt.Sprintf("Session '%s' is already archived", sessionName)
		ui.PrintWarning(msg)
		fmt.Println("\nTo restore this session:")
		fmt.Println("  1. Use: agm session list --all")
		fmt.Println("  2. Find the session and note its ID")
		fmt.Println("  3. Use: agm session unarchive <session-id>")
		return true, nil
	}
	isActive := getResult.Session.Status == "active"
	if isActive && !asyncArchive {
		return true, fmt.Errorf("session '%s' is active; use --async to archive an active session", sessionName)
	}
	if !isActive && asyncArchive {
		return true, fmt.Errorf("--async should only be used for active sessions; omit --async for stopped sessions")
	}
	if asyncArchive {
		return true, spawnReaper(sessionName)
	}
	return false, nil
}

// reportPostCleanup prints the per-step cleanup results from ops.ArchiveSession.
func reportPostCleanup(pc *ops.CleanupResult) {
	if pc == nil {
		return
	}
	if pc.WorktreesRemoved > 0 {
		fmt.Printf("Removed %d worktree(s)\n", pc.WorktreesRemoved)
	}
	if pc.WorktreesPruned {
		fmt.Printf("Pruned orphaned worktree references\n")
	}
	if pc.BranchDeleted {
		fmt.Printf("Deleted session branch\n")
	}
	if pc.SandboxBranchDeleted {
		fmt.Printf("Deleted sandbox branch\n")
	}
	if pc.SandboxRemoved {
		fmt.Printf("Removed sandbox directory\n")
	}
}

// reportSessionCleanup prints results from runSessionCleanup if non-nil.
func reportSessionCleanup(cleanupResult *cleanup.Result) {
	if cleanupResult == nil {
		return
	}
	if cleanupResult.WorktreesRemoved > 0 {
		fmt.Printf("Cleaned up %d worktree(s)\n", cleanupResult.WorktreesRemoved)
	}
	if cleanupResult.BranchesDeleted > 0 {
		fmt.Printf("Deleted %d branch(es)\n", cleanupResult.BranchesDeleted)
	}
	if cleanupResult.TmpFilesRemoved > 0 {
		fmt.Printf("Removed %d tmp file(s)\n", cleanupResult.TmpFilesRemoved)
	}
}

// cleanupMergedWorktrees removes worktrees whose branches have been merged into
// main, printing per-result messages to stderr.
func cleanupMergedWorktrees(workingDirectory string) {
	results, err := gitpkg.RemoveMergedWorktrees(workingDirectory, "main")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: worktree cleanup failed: %v\n", err)
	}
	for _, r := range results {
		if r.Removed {
			fmt.Fprintf(os.Stderr, "Cleaned up merged worktree: %s\n", r.Branch)
		} else if r.Err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not clean worktree %s: %v\n", r.Branch, r.Err)
		}
	}
}

// parseDuration parses duration strings like "30d", "7d", "1w", "24h"
func parseDuration(s string) (time.Duration, error) {
	// Handle day suffix (e.g., "30d")
	if len(s) >= 2 && s[len(s)-1] == 'd' {
		days := s[:len(s)-1]
		d, err := time.ParseDuration(days + "h")
		if err != nil {
			return 0, fmt.Errorf("invalid duration format: %s", s)
		}
		return d * 24, nil
	}

	// Handle week suffix (e.g., "1w")
	if len(s) >= 2 && s[len(s)-1] == 'w' {
		weeks := s[:len(s)-1]
		d, err := time.ParseDuration(weeks + "h")
		if err != nil {
			return 0, fmt.Errorf("invalid duration format: %s", s)
		}
		return d * 24 * 7, nil
	}

	// Try standard time.ParseDuration for hours, minutes, etc.
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration format (use 30d, 7d, 1w, or 24h): %s", s)
	}
	return d, nil
}

// archiveBulk archives multiple sessions based on filters
func archiveBulk() error {
	// Parse duration filter if specified
	var maxAge time.Duration
	if olderThan != "" {
		var err error
		maxAge, err = parseDuration(olderThan)
		if err != nil {
			return err
		}
	}

	// Get Dolt storage adapter
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer adapter.Close()

	allManifests, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		ui.PrintError(err,
			"Failed to list manifests from Dolt",
			"  • Check Dolt server is running\n"+
				"  • Verify database connection")
		return err
	}

	if len(allManifests) == 0 {
		ui.PrintWarning("No sessions found")
		return nil
	}

	activeSessions := collectActiveTmuxNames()
	now := time.Now()
	candidates, skipped := selectArchiveCandidates(allManifests, activeSessions, maxAge, now)

	if len(candidates) == 0 {
		fmt.Println("No sessions match the criteria for archival.")
		if len(skipped) > 0 {
			fmt.Printf("\nSkipped %d active session(s):\n", len(skipped))
			for _, s := range skipped {
				fmt.Printf("  - %s\n", s)
			}
		}
		return nil
	}

	printArchivePreview(candidates, skipped, now)

	if dryRun {
		ui.PrintSuccess("Dry run completed - no sessions were archived")
		fmt.Println("\nTo perform the archive, run without --dry-run flag")
		return nil
	}

	confirmed, err := confirmBulkArchive(len(candidates))
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Println("Cancelled.")
		return nil
	}

	// Build OpContext for cleanup operations
	opCtx := &ops.OpContext{
		Storage: adapter,
		Tmux:    tmuxClient,
	}

	successCount, failCount := bulkArchiveCandidates(adapter, candidates, opCtx)

	// Report results
	fmt.Println()
	if successCount > 0 {
		ui.PrintSuccess(fmt.Sprintf("Archived %d session(s)", successCount))
		fmt.Println("\nArchived sessions remain in their original locations with lifecycle: archived")
	}
	if failCount > 0 {
		ui.PrintWarning(fmt.Sprintf("Failed to archive %d session(s)", failCount))
	}

	fmt.Printf("\nUse 'agm session list --all' to see archived sessions.\n")
	fmt.Printf("To restore: edit manifest.yaml and change lifecycle from 'archived' to ''\n")

	return nil
}

// printArchivePreview prints the per-candidate preview block and the skipped
// list (if any) for archiveBulk.
func printArchivePreview(candidates []*manifest.Manifest, skipped []string, now time.Time) {
	fmt.Printf("Found %d session(s) to archive:\n\n", len(candidates))
	for _, m := range candidates {
		age := now.Sub(m.UpdatedAt)
		daysAgo := int(age.Hours() / 24)
		fmt.Printf("  • %s\n", ui.Bold(m.Name))
		fmt.Printf("    ID: %s\n", m.SessionID)
		if m.Context.Project != "" {
			fmt.Printf("    Project: %s\n", m.Context.Project)
		}
		fmt.Printf("    Last activity: %s (%d days ago)\n", m.UpdatedAt.Format("2006-01-02 15:04:05"), daysAgo)
		fmt.Println()
	}
	if len(skipped) > 0 {
		fmt.Printf("Skipped %d active session(s):\n", len(skipped))
		for _, s := range skipped {
			fmt.Printf("  - %s\n", s)
		}
		fmt.Println()
	}
}

// confirmBulkArchive prompts the user (or auto-confirms in test mode) before
// proceeding with the bulk archive.
func confirmBulkArchive(count int) (bool, error) {
	if os.Getenv("ENGRAM_TEST_MODE") == "1" {
		return true, nil
	}
	var confirmed bool
	err := huh.NewConfirm().
		Title(fmt.Sprintf("Archive %d session(s)?", count)).
		Description("This will mark sessions as archived (in-place, no files deleted).").
		Affirmative("Yes").
		Negative("No").
		Value(&confirmed).
		WithTheme(ui.GetTheme()).
		Run()
	if err != nil {
		ui.PrintError(err,
			"Failed to read confirmation prompt",
			"  • Check terminal is interactive (TTY)")
		return false, err
	}
	return confirmed, nil
}

// collectActiveTmuxNames returns the set of active tmux session names from
// tmuxClient (or an empty set if the client is unset or query fails).
func collectActiveTmuxNames() map[string]bool {
	active := make(map[string]bool)
	if tmuxClient == nil {
		return active
	}
	list, err := tmuxClient.ListSessions()
	if err != nil {
		return active
	}
	for _, name := range list {
		active[name] = true
	}
	return active
}

// selectArchiveCandidates filters allManifests into the set eligible for bulk
// archive given the active tmux names, age cutoff, and `now` reference time.
// Returns (candidates, skipped) where skipped is a list of human-readable
// reasons for each filtered-out session that the caller wants to surface.
func selectArchiveCandidates(allManifests []*manifest.Manifest, activeSessions map[string]bool, maxAge time.Duration, now time.Time) ([]*manifest.Manifest, []string) {
	var candidates []*manifest.Manifest
	var skipped []string
	for _, m := range allManifests {
		if m.Lifecycle == manifest.LifecycleArchived {
			continue
		}
		if activeSessions[m.Tmux.SessionName] {
			skipped = append(skipped, fmt.Sprintf("%s (active)", m.Name))
			continue
		}
		if !includeSupervisors && ops.IsSupervisorSession(m.Name) {
			skipped = append(skipped, fmt.Sprintf("%s (supervisor)", m.Name))
			continue
		}
		if m.UpdatedAt.IsZero() {
			ui.PrintWarning(fmt.Sprintf("Session '%s' has empty updated_at, skipping", m.Name))
			continue
		}
		if maxAge > 0 && now.Sub(m.UpdatedAt) < maxAge {
			continue
		}
		candidates = append(candidates, m)
	}
	return candidates, skipped
}

// bulkArchiveCandidates writes the lifecycle update for each candidate session
// and runs best-effort cleanup. Returns (successCount, failCount).
func bulkArchiveCandidates(adapter *dolt.Adapter, candidates []*manifest.Manifest, opCtx *ops.OpContext) (int, int) {
	var successCount, failCount int
	for _, m := range candidates {
		m.Lifecycle = manifest.LifecycleArchived
		m.UpdatedAt = time.Now()

		if err := adapter.UpdateSession(m); err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to update session in Dolt for %s: %v", m.Name, err))
			failCount++
			continue
		}

		sandboxPath := ""
		if m.Sandbox != nil {
			sandboxPath = m.Sandbox.MergedPath
		}
		repoPath := m.Context.Project
		postCleanup := ops.CleanupAfterArchive(
			m.SessionID, m.Name,
			m.WorkingDirectory, repoPath, sandboxPath, m.Name,
			false,
		)
		if postCleanup.BranchDeleted {
			fmt.Printf("  Deleted branch: %s\n", m.Name)
		}
		if postCleanup.SandboxBranchDeleted {
			fmt.Printf("  Deleted sandbox branch: agm/%s\n", m.SessionID)
		}

		cleanupResult := runSessionCleanup(m.Name, opCtx)
		if cleanupResult != nil {
			if cleanupResult.WorktreesRemoved > 0 {
				fmt.Printf("  Cleaned up %d worktree(s)\n", cleanupResult.WorktreesRemoved)
			}
			if cleanupResult.BranchesDeleted > 0 {
				fmt.Printf("  Deleted %d branch(es)\n", cleanupResult.BranchesDeleted)
			}
		}
		successCount++
	}
	return successCount, failCount
}

// spawnReaper spawns a detached agm-reaper process for async archival
// The reaper will wait for Claude prompt, send /exit, and archive the session
func spawnReaper(sessionName string) error {
	// Find agm-reaper binary (should be in same directory as agm)
	agmPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	reaperPath := filepath.Join(filepath.Dir(agmPath), "agm-reaper")

	// Create log file path with sanitized session name to prevent path traversal
	// This must happen before binary check so error messages include sanitized path
	// Handle both forward slashes and backslashes for cross-platform security
	sanitized := sessionName
	// Remove directory components with forward slashes
	if idx := strings.LastIndex(sanitized, "/"); idx != -1 {
		sanitized = sanitized[idx+1:]
	}
	// Remove directory components with backslashes (Windows-style paths)
	if idx := strings.LastIndex(sanitized, "\\"); idx != -1 {
		sanitized = sanitized[idx+1:]
	}
	// Use filepath.Base as final cleanup for any platform-specific separators
	sanitized = filepath.Base(sanitized)
	logFile := filepath.Join(os.TempDir(), fmt.Sprintf("agm-reaper-%s.log", sanitized))

	// Check if reaper binary exists
	if _, err := os.Stat(reaperPath); err != nil {
		ui.PrintError(err,
			"agm-reaper binary not found",
			fmt.Sprintf("  • Expected location: %s\n"+
				"  • Log file: %s\n"+
				"  • Build reaper: cd ~/src/ai-tools/agm && make build\n"+
				"  • Or install: make install\n"+
				"  • Or use synchronous archive: agm session archive %s (without --async)",
				reaperPath, logFile, sessionName))
		return fmt.Errorf("agm-reaper binary not found (log: %s): %w", logFile, err)
	}

	// Get sessions directory from config
	sessionsDir := cfg.SessionsDir

	// Build command with detachment
	cmd := exec.Command(reaperPath, "--session", sessionName, "--log-file", logFile, "--sessions-dir", sessionsDir)

	// Detach process from parent using setsid
	// This ensures the reaper survives even if the parent shell exits
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Create new session (detach from terminal)
	}

	// Redirect stdout/stderr to /dev/null (all logging goes to file)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	// Start process without waiting
	if err := cmd.Start(); err != nil {
		ui.PrintError(err,
			"Failed to spawn reaper process",
			fmt.Sprintf("  • Command: %s --session %s --log-file %s --sessions-dir %s\n"+
				"  • Check permissions: ls -l %s\n"+
				"  • Verify binary is executable: chmod +x %s\n"+
				"  • Test manually: %s --help",
				reaperPath, sessionName, logFile, sessionsDir, reaperPath, reaperPath, reaperPath))
		return fmt.Errorf("failed to start reaper: %w", err)
	}

	// Don't wait for process - it's detached
	pid := cmd.Process.Pid

	// Release process resources immediately to prevent zombie process
	// This is safe because the process is fully detached via setsid
	if err := cmd.Process.Release(); err != nil {
		// Log warning but don't fail - process is already running
		fmt.Fprintf(os.Stderr, "Warning: failed to release process resources: %v\n", err)
	}

	// Report success
	ui.PrintSuccess("Async archive started")
	fmt.Printf("\nReaper process spawned:\n")
	fmt.Printf("  PID: %d\n", pid)
	fmt.Printf("  Session: %s\n", sessionName)
	fmt.Printf("  Log file: %s\n", logFile)
	fmt.Printf("\nThe reaper will:\n")
	fmt.Printf("  1. Wait for Claude to return to prompt (smart detection, not fixed interval)\n")
	fmt.Printf("  2. Send /exit command\n")
	fmt.Printf("  3. Wait for pane to close\n")
	fmt.Printf("  4. Archive the session\n")
	fmt.Printf("\nMonitor progress: tail -f %s\n", logFile)

	return nil
}

// runSessionCleanup performs best-effort cleanup of session resources.
// Returns nil if the storage backend doesn't support worktree tracking.
func runSessionCleanup(sessionName string, opCtx *ops.OpContext) *cleanup.Result {
	// Type-assert to get concrete dolt.Adapter for worktree operations
	adapter, ok := opCtx.Storage.(*dolt.Adapter)
	if !ok {
		return nil
	}
	store := &cleanup.DoltWorktreeStore{Adapter: adapter}
	return cleanup.SessionResources(
		context.Background(),
		sessionName,
		store,
		cleanup.RealGitOps{},
		slog.Default(),
	)
}

// runSettingsCleanup runs configure-claude-settings cleanup-dirs as best-effort
// post-archive maintenance. Errors are logged but do not fail the archive.
func runSettingsCleanup() {
	binPath, err := findConfigureBinary()
	if err != nil {
		return // silently skip if binary not found
	}

	cmd := exec.Command(binPath, "cleanup-dirs")
	output, err := cmd.Output()
	if err != nil {
		return // best-effort
	}
	// Only print if something was actually cleaned
	out := strings.TrimSpace(string(output))
	if out != "" && !strings.HasPrefix(out, "No stale") {
		fmt.Printf("\n%s\n", out)
	}
}

func init() {
	archiveCmd.Flags().BoolVar(&asyncArchive, "async", false,
		"Archive an active session asynchronously (required for active sessions, not valid for stopped sessions)")
	archiveCmd.Flags().BoolVar(&archiveAll, "all", false,
		"Archive all inactive sessions (use with --older-than for filtering)")
	archiveCmd.Flags().StringVar(&olderThan, "older-than", "",
		"Archive sessions inactive for N days (e.g., 30d, 7d, 1w, 24h)")
	archiveCmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"Preview sessions to be archived without executing")
	archiveCmd.Flags().BoolVar(&cleanupWorktrees, "cleanup-worktrees", false,
		"Clean up merged git worktrees after archiving")
	archiveCmd.Flags().BoolVarP(&forceArchive, "force", "f", false,
		"Skip pre-archive verification checks (uncommitted changes, unmerged branch)")
	archiveCmd.Flags().BoolVar(&keepSandbox, "keep-sandbox", false,
		"Preserve sandbox directory for debugging instead of removing it")
	archiveCmd.Flags().BoolVar(&includeSupervisors, "include-supervisors", false,
		"Include supervisor sessions (orchestrator, overseer, meta-*) in bulk archive")
	sessionCmd.AddCommand(archiveCmd)
}
