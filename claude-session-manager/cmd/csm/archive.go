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
)

var archiveCmd = &cobra.Command{
	Use:   "archive <session-name>",
	Short: "Archive a Claude session",
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
  # Archive with confirmation prompt
  csm archive my-old-session

  # Archive without confirmation (automation/scripts)
  csm archive my-old-session --force

  # List all sessions including archived
  csm list --all

  # Archive by tmux session name
  csm archive claude-5

  # Archive by session ID
  csm archive session-abc123`,
	Args: cobra.ExactArgs(1),
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
	sessionName := args[0]

	// If --async flag is set, spawn reaper instead of archiving now
	if asyncArchive {
		return spawnReaper(sessionName)
	}

	sessionsDir := cfg.SessionsDir

	// Resolve session identifier to manifest
	m, manifestPath, err := session.ResolveIdentifier(sessionName, sessionsDir)
	if err != nil {
		ui.PrintError(err, "Session not found",
			fmt.Sprintf("  • Check session name with: csm list\n"+
				"  • Available sessions are in: %s", sessionsDir))
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
			ui.PrintError(
				fmt.Errorf("session is active"),
				fmt.Sprintf("Cannot archive active session '%s'", sessionName),
				fmt.Sprintf("The session is currently running in tmux.\n\n"+
					"To archive this session:\n"+
					"  1. Stop the tmux session first:\n"+
					"     tmux kill-session -t %s\n\n"+
					"  2. Then archive:\n"+
					"     csm archive %s\n\n"+
					"Or use --force to archive anyway:\n"+
					"  csm archive %s --force",
					m.Tmux.SessionName, sessionName, sessionName))
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
			ui.PrintError(err, "Failed to read confirmation", "")
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
		ui.PrintError(err, "Failed to write manifest",
			"  • Check file permissions\n"+
				"  • Verify disk space")
		return err
	}

	// Move session directory to .archive-old-format/
	sessionDir := filepath.Dir(manifestPath)
	archiveBaseDir := filepath.Join(sessionsDir, ".archive-old-format")
	archiveTargetDir := filepath.Join(archiveBaseDir, filepath.Base(sessionDir))

	// Create archive directory if it doesn't exist
	if err := os.MkdirAll(archiveBaseDir, 0700); err != nil {
		ui.PrintError(err, "Failed to create archive directory",
			fmt.Sprintf("  • Directory: %s\n"+
				"  • Check permissions and disk space", archiveBaseDir))
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
		ui.PrintError(err, "Failed to move session to archive",
			fmt.Sprintf("  • From: %s\n"+
				"  • To: %s\n"+
				"  • Check permissions and ensure target doesn't exist", sessionDir, archiveTargetDir))
		return err
	}

	// Report success
	ui.PrintSuccess(fmt.Sprintf("Archived session: %s", sessionName))
	fmt.Printf("\nSession moved to: %s\n", archiveTargetDir)
	fmt.Printf("\nThe session is now hidden from 'csm list'.\n")
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
		ui.PrintError(err, "csm-reaper binary not found",
			fmt.Sprintf("  • Expected location: %s\n"+
				"  • Ensure csm-reaper is built and installed alongside csm", reaperPath))
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
		ui.PrintError(err, "Failed to spawn reaper process",
			fmt.Sprintf("  • Command: %s --session %s --log-file %s\n"+
				"  • Check permissions and binary path", reaperPath, sessionName, logFile))
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
	rootCmd.AddCommand(archiveCmd)
}
