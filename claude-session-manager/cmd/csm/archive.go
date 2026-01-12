package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/discovery"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/session"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var (
	forceArchive bool
	asyncArchive bool // Spawn background reaper for async archival
	archiveAll   bool
	olderThan    string
	dryRun       bool
)

var archiveCmd = &cobra.Command{
	Use:   "archive [session-name]",
	Short: "Archive a Claude session or multiple sessions",
	Long: `Archive a Claude session by marking it as archived.

Archived sessions:
  • Hidden from 'csm list' (use --all flag to see them)
  • Files are NOT deleted (only metadata updated)
  • Cannot be resumed until restored
  • Automatic backup created before archiving

This command will:
  1. Find the session by name, tmux name, or session ID
  2. Check if session is currently active in tmux
  3. Prompt for confirmation (unless --force is used)
  4. Update the manifest Lifecycle field to "archived"
  5. Create automatic backup of the manifest

To restore an archived session:
  1. Run: csm list --all
  2. Find session ID
  3. Edit: ~/sessions/session-<ID>/manifest.yaml
  4. Change: lifecycle: "archived" to lifecycle: ""
  5. Save and session will appear in csm list

Examples:
  # Archive single session with confirmation prompt
  csm archive my-old-session

  # Archive without confirmation (automation/scripts)
  csm archive my-old-session --force

  # Archive all inactive sessions older than 30 days (preview only)
  csm archive --all --older-than=30d --dry-run

  # Archive all inactive sessions older than 30 days
  csm archive --all --older-than=30d

  # Archive all inactive sessions (be careful!)
  csm archive --all

  # List all sessions including archived
  csm list --all

  # Archive by tmux session name
  csm archive claude-5

  # Archive by session ID
  csm archive session-abc123`,
	Args: cobra.MaximumNArgs(1),
	RunE: archiveSession,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Only complete first argument
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// List all manifests
		manifests, err := manifest.List(cfg.SessionsDir)
		if err != nil {
			return []string{}, cobra.ShellCompDirectiveNoFileComp
		}

		// Filter out archived sessions (can't archive what's already archived)
		filtered := make([]*manifest.Manifest, 0, len(manifests))
		for _, m := range manifests {
			if m.Lifecycle != manifest.LifecycleArchived {
				filtered = append(filtered, m)
			}
		}
		manifests = filtered

		// Get tmux mapping
		tmuxMapping, _ := discovery.GetTmuxMapping(cfg.SessionsDir)

		// Build suggestions from non-archived sessions
		var suggestions []string
		for _, m := range manifests {
			// Add tmux name
			if tmuxName := tmuxMapping[m.SessionID]; tmuxName != "" {
				suggestions = append(suggestions, tmuxName)
			}

			// Add manifest name (if different)
			if m.Name != "" && m.Name != tmuxMapping[m.SessionID] {
				suggestions = append(suggestions, m.Name)
			}
		}

		return suggestions, cobra.ShellCompDirectiveNoFileComp
	},
}

func archiveSession(cmd *cobra.Command, args []string) error {
	// Handle bulk archive mode
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

	// If --async flag is set, spawn reaper instead of archiving now
	if asyncArchive {
		return spawnReaper(sessionName)
	}

	sessionsDir := cfg.SessionsDir

	// Resolve session identifier to manifest
	m, manifestPath, err := session.ResolveIdentifier(sessionName, sessionsDir)
	if err != nil {
		ui.PrintSessionNotFoundError(sessionName, sessionsDir)
		return err
	}

	// Check if already archived
	if m.Lifecycle == manifest.LifecycleArchived {
		msg := fmt.Sprintf("Session '%s' is already archived", sessionName)
		ui.PrintWarning(msg)
		fmt.Printf("\nManifest: %s\n", manifestPath)
		fmt.Println("\nTo restore this session:")
		fmt.Println("  1. Edit the manifest file above")
		fmt.Println("  2. Change lifecycle: \"archived\" to lifecycle: \"\"")
		fmt.Println("  3. Session will appear in csm list")
		return nil
	}

	// Check if session is active (unless --force)
	if !forceArchive {
		tmux := session.NewRealTmux()
		isActive, err := tmux.HasSession(m.Tmux.SessionName)
		if err != nil {
			// Ignore error - if we can't check, assume not active
			isActive = false
		}
		if isActive {
			ui.PrintActiveSessionError(sessionName, m.Tmux.SessionName)
			return fmt.Errorf("cannot archive active session")
		}
	}

	// Show confirmation prompt (unless --force)
	if !forceArchive {
		fmt.Printf("Archive session: %s\n", ui.Bold(m.Name))
		fmt.Printf("  Location: %s\n", manifestPath)
		if m.Context.Project != "" {
			fmt.Printf("  Project: %s\n", m.Context.Project)
		}

		// Show status
		tmux := session.NewRealTmux()
		status := "stopped"
		isActive, err := tmux.HasSession(m.Tmux.SessionName)
		if err == nil && isActive {
			status = "active"
		}
		fmt.Printf("  Status: %s\n", status)

		fmt.Println("\nThis will mark the session as archived.")
		fmt.Println("Files will NOT be deleted.")
		fmt.Println()

		var confirmed bool
		err = huh.NewConfirm().
			Title("Archive this session?").
			Affirmative("Yes").
			Negative("No").
			Value(&confirmed).
			Run()
		if err != nil {
			ui.PrintError(err,
				"Failed to read confirmation prompt",
				"  • Use --force flag to skip confirmation: csm archive "+sessionName+" --force\n"+
					"  • Check terminal is interactive (TTY)\n"+
					"  • Try running outside tmux/screen if inside")
			return err
		}

		if !confirmed {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Update lifecycle field
	m.Lifecycle = manifest.LifecycleArchived

	// Write manifest (automatic backup + UpdatedAt)
	if err := manifest.Write(manifestPath, m); err != nil {
		ui.PrintManifestWriteError(err)
		return err
	}

	// Move session directory to .archive-old-format/
	sessionDir := filepath.Dir(manifestPath)
	archiveBaseDir := filepath.Join(sessionsDir, ".archive-old-format")
	archiveTargetDir := filepath.Join(archiveBaseDir, filepath.Base(sessionDir))

	// Create archive directory if it doesn't exist
	if err := os.MkdirAll(archiveBaseDir, 0700); err != nil {
		ui.PrintError(err,
			"Failed to create archive directory",
			fmt.Sprintf("  • Directory: %s\n"+
				"  • Check permissions: ls -ld %s\n"+
				"  • Check disk space: df -h %s\n"+
				"  • Verify parent directory writable",
				archiveBaseDir, sessionsDir, sessionsDir))
		return err
	}

	// Check for conflict and auto-rename if needed
	originalTargetName := filepath.Base(archiveTargetDir)
	if _, err := os.Stat(archiveTargetDir); err == nil {
		// Conflict detected: target already exists
		timestamp := time.Now().Format("20060102T150405Z")
		archiveTargetDir = archiveTargetDir + "-" + timestamp

		ui.PrintWarning(fmt.Sprintf("Archive '%s' already exists", originalTargetName))
		fmt.Printf("Renaming to: %s\n", filepath.Base(archiveTargetDir))
	}

	// Move session directory to archive
	if err := os.Rename(sessionDir, archiveTargetDir); err != nil {
		ui.PrintError(err,
			"Failed to move session to archive",
			fmt.Sprintf("  • From: %s\n"+
				"  • To: %s\n"+
				"  • Check permissions: ls -ld %s\n"+
				"  • Verify source exists: ls -d %s\n"+
				"  • Check disk space: df -h %s",
				sessionDir, archiveTargetDir, archiveBaseDir, sessionDir, archiveBaseDir))
		return err
	}

	// Report success
	ui.PrintSuccess(fmt.Sprintf("Archived session: %s", sessionName))
	fmt.Printf("\nSession moved to: %s\n", archiveTargetDir)
	fmt.Printf("\nThe session is now hidden from 'csm list'.\n")
	fmt.Printf("Use 'csm list --all' to see archived sessions.\n")

	return nil
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
	sessionsDir := cfg.SessionsDir

	// Parse duration filter if specified
	var maxAge time.Duration
	if olderThan != "" {
		var err error
		maxAge, err = parseDuration(olderThan)
		if err != nil {
			return err
		}
	}

	// Get all manifests
	allManifests, err := manifest.List(sessionsDir)
	if err != nil {
		ui.PrintError(err,
			"Failed to list manifests",
			"  • Check sessions directory permissions: ls -ld "+sessionsDir+"\n"+
				"  • Verify directory structure: ls -la "+sessionsDir)
		return err
	}

	if len(allManifests) == 0 {
		ui.PrintWarning("No sessions found")
		return nil
	}

	// Get active tmux sessions for filtering
	tmux := session.NewRealTmux()
	activeSessions := make(map[string]bool)
	if !forceArchive {
		activeList, err := tmux.ListSessions()
		if err == nil {
			for _, name := range activeList {
				activeSessions[name] = true
			}
		}
	}

	// Filter sessions to archive
	type candidateSession struct {
		manifest *manifest.Manifest
		path     string
	}
	var candidates []candidateSession
	var skipped []string

	now := time.Now()
	for _, m := range allManifests {
		// Skip already archived
		if m.Lifecycle == manifest.LifecycleArchived {
			continue
		}

		// Skip active sessions (unless --force)
		if !forceArchive && activeSessions[m.Tmux.SessionName] {
			skipped = append(skipped, fmt.Sprintf("%s (active)", m.Name))
			continue
		}

		// Check updated_at timestamp
		if m.UpdatedAt.IsZero() {
			ui.PrintWarning(fmt.Sprintf("Session '%s' has empty updated_at, skipping", m.Name))
			continue
		}

		// Apply age filter if specified
		if maxAge > 0 {
			age := now.Sub(m.UpdatedAt)
			if age < maxAge {
				continue
			}
		}

		// Find manifest path
		sessionDir := filepath.Join(sessionsDir, fmt.Sprintf("session-%s", m.SessionID))
		manifestPath := filepath.Join(sessionDir, "manifest.yaml")

		// Also check archive directory
		if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
			archiveDir := filepath.Join(sessionsDir, ".archive-old-format", fmt.Sprintf("session-%s", m.SessionID))
			if _, err := os.Stat(archiveDir); err == nil {
				sessionDir = archiveDir
				manifestPath = filepath.Join(archiveDir, "manifest.yaml")
			}
		}

		candidates = append(candidates, candidateSession{
			manifest: m,
			path:     manifestPath,
		})
	}

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

	// Display preview
	fmt.Printf("Found %d session(s) to archive:\n\n", len(candidates))
	for _, c := range candidates {
		age := now.Sub(c.manifest.UpdatedAt)
		daysAgo := int(age.Hours() / 24)
		fmt.Printf("  • %s\n", ui.Bold(c.manifest.Name))
		fmt.Printf("    ID: %s\n", c.manifest.SessionID)
		if c.manifest.Context.Project != "" {
			fmt.Printf("    Project: %s\n", c.manifest.Context.Project)
		}
		fmt.Printf("    Last activity: %s (%d days ago)\n", c.manifest.UpdatedAt.Format("2006-01-02 15:04:05"), daysAgo)
		fmt.Printf("    Path: %s\n", filepath.Dir(c.path))
		fmt.Println()
	}

	if len(skipped) > 0 {
		fmt.Printf("Skipped %d active session(s):\n", len(skipped))
		for _, s := range skipped {
			fmt.Printf("  - %s\n", s)
		}
		fmt.Println()
	}

	// Dry run mode
	if dryRun {
		ui.PrintSuccess("Dry run completed - no sessions were archived")
		fmt.Println("\nTo perform the archive, run without --dry-run flag")
		return nil
	}

	// Confirmation prompt (unless --force)
	if !forceArchive {
		var confirmed bool
		err = huh.NewConfirm().
			Title(fmt.Sprintf("Archive %d session(s)?", len(candidates))).
			Description("This will mark sessions as archived and move them to .archive-old-format/").
			Affirmative("Yes").
			Negative("No").
			Value(&confirmed).
			Run()
		if err != nil {
			ui.PrintError(err,
				"Failed to read confirmation prompt",
				"  • Use --force flag to skip confirmation\n"+
					"  • Check terminal is interactive (TTY)")
			return err
		}

		if !confirmed {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Perform archival
	var successCount, failCount int
	for _, c := range candidates {
		// Update lifecycle field
		c.manifest.Lifecycle = manifest.LifecycleArchived

		// Write manifest (automatic backup + UpdatedAt)
		if err := manifest.Write(c.path, c.manifest); err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to update manifest for %s: %v", c.manifest.Name, err))
			failCount++
			continue
		}

		// Move session directory to .archive-old-format/
		sessionDir := filepath.Dir(c.path)
		archiveBaseDir := filepath.Join(sessionsDir, ".archive-old-format")
		archiveTargetDir := filepath.Join(archiveBaseDir, filepath.Base(sessionDir))

		// Create archive directory if it doesn't exist
		if err := os.MkdirAll(archiveBaseDir, 0700); err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to create archive directory for %s: %v", c.manifest.Name, err))
			failCount++
			continue
		}

		// Check for conflict and auto-rename if needed
		if _, err := os.Stat(archiveTargetDir); err == nil {
			timestamp := time.Now().Format("20060102T150405Z")
			archiveTargetDir = archiveTargetDir + "-" + timestamp
		}

		// Move session directory to archive
		if err := os.Rename(sessionDir, archiveTargetDir); err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to move %s to archive: %v", c.manifest.Name, err))
			failCount++
			continue
		}

		successCount++
	}

	// Report results
	fmt.Println()
	if successCount > 0 {
		ui.PrintSuccess(fmt.Sprintf("Archived %d session(s)", successCount))
	}
	if failCount > 0 {
		ui.PrintWarning(fmt.Sprintf("Failed to archive %d session(s)", failCount))
	}

	fmt.Printf("\nSessions moved to: %s/.archive-old-format/\n", sessionsDir)
	fmt.Printf("Use 'csm list --all' to see archived sessions.\n")

	return nil
}

// spawnReaper spawns a detached csm-reaper process for async archival
// The reaper will wait for Claude prompt, send /exit, and archive the session
func spawnReaper(sessionName string) error {
	// Find csm-reaper binary (should be in same directory as csm)
	csmPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	reaperPath := filepath.Join(filepath.Dir(csmPath), "csm-reaper")

	// Check if reaper binary exists
	if _, err := os.Stat(reaperPath); err != nil {
		ui.PrintError(err,
			"csm-reaper binary not found",
			fmt.Sprintf("  • Expected location: %s\n"+
				"  • Build reaper: cd ~/src/ws/oss/repos/ai-tools/main/claude-session-manager && go build -o %s ./cmd/csm-reaper\n"+
				"  • Or use synchronous archive: csm archive %s (without --async)",
				reaperPath, reaperPath, sessionName))
		return fmt.Errorf("csm-reaper binary not found: %w", err)
	}

	// Create log file path with sanitized session name to prevent path traversal
	// Replace any potentially dangerous characters with underscores
	sanitized := filepath.Base(sessionName) // Removes any directory components
	logFile := filepath.Join(os.TempDir(), fmt.Sprintf("csm-reaper-%s.log", sanitized))

	// Build command with detachment
	cmd := exec.Command(reaperPath, "--session", sessionName, "--log-file", logFile)

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
			fmt.Sprintf("  • Command: %s --session %s --log-file %s\n"+
				"  • Check permissions: ls -l %s\n"+
				"  • Verify binary is executable: chmod +x %s\n"+
				"  • Test manually: %s --help",
				reaperPath, sessionName, logFile, reaperPath, reaperPath, reaperPath))
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
	fmt.Printf("  1. Wait for Claude to return to prompt\n")
	fmt.Printf("  2. Send /exit command\n")
	fmt.Printf("  3. Wait for pane to close\n")
	fmt.Printf("  4. Archive the session\n")
	fmt.Printf("\nMonitor progress: tail -f %s\n", logFile)

	return nil
}

func init() {
	archiveCmd.Flags().BoolVarP(&forceArchive, "force", "f", false,
		"Skip confirmation prompt and active session check")
	archiveCmd.Flags().BoolVar(&asyncArchive, "async", false,
		"Spawn background reaper for async archival (used by /csm-exit)")
	archiveCmd.Flags().BoolVar(&archiveAll, "all", false,
		"Archive all inactive sessions (use with --older-than for filtering)")
	archiveCmd.Flags().StringVar(&olderThan, "older-than", "",
		"Archive sessions inactive for N days (e.g., 30d, 7d, 1w, 24h)")
	archiveCmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"Preview sessions to be archived without executing")
	rootCmd.AddCommand(archiveCmd)
}
