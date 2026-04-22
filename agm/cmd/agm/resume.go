package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/claude"
	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/discovery"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/state"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	uuidpkg "github.com/vbonnet/dear-agent/agm/internal/uuid"
)

var (
	resumeDetached    bool
	resumeForceParent bool
	resumePrompt      string
	resumePromptFile  string
)

var resumeCmd = &cobra.Command{
	Use:   "resume [identifier]",
	Short: "Resume a Claude session by UUID, tmux name, or fuzzy match",
	Long: `Resume a Claude session by various identifier types:

- UUID (full or partial): agmresume c4eb298c
- Tmux session name:      agmresume claude-1
- Fuzzy match on project: agmresume workspace-design
- Interactive (no args):  agmresume

The command will:
1. Resolve the identifier to find the Claude UUID
2. Check session health (worktree exists, Claude dirs present)
3. Create or attach to tmux session
4. Send 'cd' to worktree directory
5. Send 'claude --resume <uuid>' to tmux pane
6. Update manifest last_activity timestamp

Flags:
  --detached     Create/resume session without attaching (session runs in background)
  --prompt       Send a prompt to the session after resume (useful for crash recovery)
  --prompt-file  Send file contents as prompt after resume (useful for crash recovery)

Examples:
  agmresume c4eb298c              # By UUID prefix
  agmresume claude-1              # By tmux name
  agmresume workspace-design      # By project path pattern
  agmresume orchestrator --detached  # Resume without attaching
  agmresume worker-1 --prompt "continue working on the auth module"  # Resume and inject prompt
  agmresume worker-1 --detached --prompt "pick up where you left off"  # Background resume with prompt
  agmresume worker-1 --prompt-file /path/to/recovery.txt  # Resume with prompt from file
  agmresume                       # Interactive picker (TODO)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get identifier from args or prompt
		var identifier string
		if len(args) > 0 {
			identifier = args[0]
		} else {
			// TODO: Interactive picker for Phase 3
			return fmt.Errorf("interactive picker not yet implemented - please provide identifier")
		}

		// Get Dolt storage adapter
		adapter, err := getStorage()
		if err != nil {
			return fmt.Errorf("failed to connect to Dolt storage: %w", err)
		}
		defer adapter.Close()

		// Resolve identifier to SessionID
		sessionID, manifestPath, err := resolveSessionIdentifier(adapter, identifier)
		if err != nil {
			ui.PrintError(err, "Failed to resolve session identifier",
				fmt.Sprintf("  • Try: agmlist --all to see available sessions\n"+
					"  • Identifier can be UUID, tmux name, or project path pattern"))
			return err
		}

		ui.PrintSuccess(fmt.Sprintf("Resolved identifier %q to session: %s", identifier, sessionID))

		// Read manifest from Dolt to check lifecycle
		m, err := adapter.GetSession(sessionID)
		if err != nil {
			ui.PrintError(err, "Failed to read session from Dolt",
				"  • Session may not exist in database\n"+
					"  • Try: agm session list --all")
			return err
		}

		// Auto-detect harness from manifest
		harnessName := m.Harness
		if harnessName == "" {
			harnessName = "claude-code" // Default for backward compatibility
		}

		// Warn if harness unavailable
		if err := agent.ValidateHarnessAvailability(harnessName); err != nil {
			ui.PrintWarning(fmt.Sprintf("⚠️  %s", err.Error()))
		}

		fmt.Printf("Using harness: %s\n", harnessName)

		// Check if session is archived
		if m.Lifecycle == manifest.LifecycleArchived {
			ui.PrintArchivedSessionError(sessionID)
			return fmt.Errorf("cannot resume archived session")
		}

		// Check session health
		health, err := checkSessionHealth(adapter, sessionID, manifestPath)
		if err != nil {
			ui.PrintError(err,
				"Session health check failed",
				"  • Run diagnostics: agmdoctor\n"+
					"  • List all sessions: agmlist --all")
			return err
		}

		// Display health status
		displayHealthStatus(health)

		// If critical issues, abort
		if !health.CanResume {
			ui.PrintError(
				fmt.Errorf("session cannot be resumed"),
				"Critical issues prevent resuming this session",
				"  • Fix the issues above and try again",
			)
			return fmt.Errorf("session health check failed")
		}

		// Resume the session
		if err := resumeSession(adapter, sessionID, manifestPath, harnessName, health); err != nil {
			ui.PrintError(err,
				"Failed to resume session",
				"  • Check tmux is running: tmux list-sessions\n"+
					"  • Verify session health: agmdoctor\n"+
					"  • Try manual attach: tmux attach -t "+health.TmuxSessionName)
			return err
		}

		ui.PrintSuccess(fmt.Sprintf("Successfully resumed session %s", sessionID))
		return nil
	},
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Only complete first argument (session identifier)
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

		// List sessions from Dolt (exclude archived sessions from completion)
		filter := &dolt.SessionFilter{
			ExcludeArchived: true,
		}
		sessions, err := adapter.ListSessions(filter)
		if err != nil {
			// Fail gracefully - return empty list if query fails
			return []string{}, cobra.ShellCompDirectiveNoFileComp
		}

		// Build completion suggestions
		var suggestions []string
		for _, m := range sessions {
			// Add tmux name (primary identifier)
			if m.Tmux.SessionName != "" {
				suggestions = append(suggestions, m.Tmux.SessionName)
			}

			// Add manifest name (secondary identifier, if different from tmux name)
			if m.Name != "" && m.Name != m.Tmux.SessionName {
				suggestions = append(suggestions, m.Name)
			}
		}

		// Return suggestions with NoFileComp directive (prevent file completion)
		return suggestions, cobra.ShellCompDirectiveNoFileComp
	},
}

// HealthStatus represents session health check results
type HealthStatus struct {
	UUID              string
	ManifestPath      string
	WorktreeExists    bool
	WorktreePath      string
	SessionEnvExists  bool
	SessionEnvPath    string
	FileHistoryExists bool
	FileHistoryPath   string
	TmuxSessionName   string
	TmuxExists        bool
	CanResume         bool
	Issues            []string
	Warnings          []string
}

// buildManifestPathMap scans the sessions directory and builds a map from SessionID to manifest file path
// This handles legacy directory naming where the directory name doesn't match the SessionID
func buildManifestPathMap(sessionsDir string) (map[string]string, error) {
	paths := make(map[string]string)

	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Skip archive directory
		if entry.Name() == ".archive-old-format" {
			continue
		}

		manifestPath := filepath.Join(sessionsDir, entry.Name(), "manifest.yaml")
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			continue
		}

		// Extract SessionID from directory name
		// Expected format: session-<sessionID> or <sessionID>
		sessionID := entry.Name()
		if strings.HasPrefix(sessionID, "session-") {
			sessionID = strings.TrimPrefix(sessionID, "session-")
		}

		paths[sessionID] = manifestPath
	}

	return paths, nil
}

// resolveSessionIdentifier finds the Claude UUID and manifest path from various identifier types
func resolveSessionIdentifier(adapter *dolt.Adapter, identifier string) (string, string, error) {
	// Defensive check: ensure cfg is initialized
	if cfg == nil {
		return "", "", fmt.Errorf("config not initialized")
	}

	// Use configured sessions directory instead of hardcoded default
	sessionsDir := cfg.SessionsDir

	// List manifests from Dolt (exclude archived sessions)
	manifests, err := adapter.ListSessions(&dolt.SessionFilter{
		ExcludeArchived: true,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to list sessions from Dolt: %w", err)
	}

	if len(manifests) == 0 {
		return "", "", fmt.Errorf("no session manifests found")
	}

	// Build sessionID -> manifestPath mapping
	// For Dolt-backed sessions, construct synthetic paths even if YAML doesn't exist
	manifestPaths := make(map[string]string)
	for _, m := range manifests {
		// Construct expected path based on session ID
		manifestPath := filepath.Join(sessionsDir, m.SessionID, "manifest.yaml")
		manifestPaths[m.SessionID] = manifestPath
	}

	// Build tmux mapping using Dolt adapter
	tmuxMapping, _ := discovery.GetTmuxMappingWithAdapter(sessionsDir, adapter)

	// Try matching strategies in order
	var matches []*manifest.Manifest
	var matchType string

	// Strategy 1: SessionID match (full or partial - v2: SessionID is top-level)
	for _, m := range manifests {
		if strings.HasPrefix(m.SessionID, identifier) || m.SessionID == identifier {
			matches = append(matches, m)
			matchType = "session ID"
		}
	}

	// Strategy 2: Tmux session name match
	if len(matches) == 0 {
		for sessionID, tmuxName := range tmuxMapping {
			if tmuxName == identifier {
				// Find manifest with this SessionID
				for _, m := range manifests {
					if m.SessionID == sessionID {
						matches = append(matches, m)
						matchType = "tmux name"
						break
					}
				}
			}
		}
	}

	// Strategy 3: Fuzzy match on project path (v2: Context.Project)
	if len(matches) == 0 {
		for _, m := range manifests {
			if strings.Contains(m.Context.Project, identifier) {
				matches = append(matches, m)
				matchType = "project path"
			}
		}
	}

	// Strategy 4: Match on manifest Name (v2 field)
	if len(matches) == 0 {
		for _, m := range manifests {
			if m.Name == identifier {
				matches = append(matches, m)
				matchType = "manifest name"
			}
		}
	}

	// Strategy 4: Fuzzy match on session ID
	if len(matches) == 0 {
		for _, m := range manifests {
			if strings.Contains(m.SessionID, identifier) {
				matches = append(matches, m)
				matchType = "session ID"
			}
		}
	}

	// Handle results
	if len(matches) == 0 {
		// No manifest found - try to find orphaned session in history and offer to import
		m, manifestPath, err := offerToImportOrphanedSession(adapter, identifier)
		if err == nil {
			// Successfully imported (v2: SessionID is top-level)
			return m.SessionID, manifestPath, nil
		}
		// Fall through to original error (orphaned session not found or user declined)
		return "", "", fmt.Errorf("no sessions found matching %q", identifier)
	}

	if len(matches) > 1 {
		// Multiple matches - show user and ask to be more specific
		ui.PrintWarning(fmt.Sprintf("Multiple sessions matched %q by %s:", identifier, matchType))
		for i, m := range matches {
			tmuxName := tmuxMapping[m.SessionID]
			if tmuxName == "" {
				tmuxName = "-"
			}
			fmt.Printf("  %d. ID: %s | Tmux: %s | Project: %s\n",
				i+1, m.SessionID, tmuxName, m.Context.Project)
		}
		return "", "", fmt.Errorf("ambiguous identifier - please be more specific")
	}

	// Single match found
	m := matches[0]

	// Check if matched session has children (execution sessions)
	// If so, prefer the most recent non-archived child over the parent
	// UNLESS --force-parent flag is set
	if !resumeForceParent {
		children, err := adapter.GetChildren(m.SessionID)
		if err == nil && len(children) > 0 {
			// Find most recent non-archived child
			var mostRecentChild *manifest.Manifest
			for _, child := range children {
				if child.Lifecycle != manifest.LifecycleArchived {
					if mostRecentChild == nil || child.UpdatedAt.After(mostRecentChild.UpdatedAt) {
						mostRecentChild = child
					}
				}
			}

			// Prefer child over parent (execution over planning)
			if mostRecentChild != nil {
				ui.PrintSuccess(fmt.Sprintf(
					"Found planning session '%s' with execution session '%s'",
					m.Name, mostRecentChild.Name))
				fmt.Println(ui.Blue("  → Resuming execution session (use --force-parent to resume planning session)"))

				// Use child session instead
				m = mostRecentChild
			}
		}
	} else {
		// User explicitly requested parent session
		fmt.Println(ui.Yellow("  ⚠ Using --force-parent: Resuming planning session"))
	}

	manifestPath, ok := manifestPaths[m.SessionID]
	if !ok {
		return "", "", fmt.Errorf("manifest path not found for session ID %s", m.SessionID)
	}
	return m.SessionID, manifestPath, nil
}

// checkSessionHealth validates that a session can be resumed (v2 schema)
func checkSessionHealth(adapter *dolt.Adapter, sessionID, manifestPath string) (*HealthStatus, error) {
	// Read manifest from Dolt
	m, err := adapter.GetSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to read session from Dolt: %w", err)
	}

	// Determine tmux session name - use manifest value or fallback to session name
	tmuxSessionName := m.Tmux.SessionName
	if tmuxSessionName == "" {
		// Fallback: use sanitized session name if Tmux.SessionName is empty
		tmuxSessionName = sanitizeTmuxName(m.Name)
		if tmuxSessionName == "" {
			tmuxSessionName = "session" // Last resort fallback
		}
	}

	health := &HealthStatus{
		UUID:            sessionID, // v2: SessionID (keeping field name UUID for backward compat in struct)
		ManifestPath:    manifestPath,
		WorktreePath:    m.Context.Project, // v2: Context.Project
		SessionEnvPath:  "",                // v2: Not stored in manifest
		FileHistoryPath: "",                // v2: Not stored in manifest
		TmuxSessionName: tmuxSessionName,
		Issues:          []string{},
		Warnings:        []string{},
		CanResume:       true,
	}

	// Check working directory exists (v2: Context.Project)
	if _, err := os.Stat(m.Context.Project); os.IsNotExist(err) {
		health.WorktreeExists = false
		health.Issues = append(health.Issues,
			fmt.Sprintf("Working directory not found: %s", m.Context.Project))
		health.CanResume = false
	} else {
		health.WorktreeExists = true
	}

	// Session env and file history checks removed (not in v2 schema)
	health.SessionEnvExists = true  // Always true since not checked
	health.FileHistoryExists = true // Always true since not checked

	// Check tmux session exists
	tmuxExists, err := tmux.HasSession(tmuxSessionName)
	if err != nil {
		health.Warnings = append(health.Warnings,
			fmt.Sprintf("Failed to check tmux session: %v", err))
	}
	health.TmuxExists = tmuxExists

	return health, nil
}

// displayHealthStatus prints health check results
func displayHealthStatus(health *HealthStatus) {
	fmt.Println("\nSession Health Check:")
	fmt.Println("────────────────────────────────────────────────")

	// Worktree
	if health.WorktreeExists {
		fmt.Printf("✓ Worktree:      %s\n", health.WorktreePath)
	} else {
		fmt.Printf("✗ Worktree:      %s (NOT FOUND)\n", health.WorktreePath)
	}

	// Tmux
	if health.TmuxExists {
		fmt.Printf("✓ Tmux:          %s (EXISTS)\n", health.TmuxSessionName)
	} else {
		fmt.Printf("○ Tmux:          %s (will create)\n", health.TmuxSessionName)
	}

	fmt.Println()

	// Display issues
	if len(health.Issues) > 0 {
		fmt.Printf("\n%s Critical Issues:\n", ui.Red("✗"))
		for _, issue := range health.Issues {
			fmt.Printf("  • %s\n", issue)
		}
		fmt.Println()
	}

	// Display warnings
	if len(health.Warnings) > 0 {
		ui.PrintWarning("Warnings:")
		for _, warning := range health.Warnings {
			fmt.Printf("  • %s\n", warning)
		}
		fmt.Println()
	}
}

// shellQuote quotes a string for safe use in shell commands
// This prevents command injection by escaping special characters
func shellQuote(s string) string {
	// Simple but secure: wrap in single quotes and escape any single quotes
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// shouldSendResumeCommands determines whether resume should send commands to the tmux session.
// Fix (commit e7cacf8): NEVER send commands to existing tmux sessions.
// Sending resume commands to an active session injects text into the running agent.
// IsClaudeRunning detection was unreliable (process name varies), so the safe
// default is to always just attach when a session already exists.
func shouldSendResumeCommands(tmuxExists bool) bool {
	return !tmuxExists
}

// sendPostResumePrompt delivers a prompt to the session after it is ready.
// It reads the prompt from promptText (inline) or promptFile, then uses
// SendMultiLinePromptSafe which waits for the Claude prompt before sending.
func sendPostResumePrompt(sessionName, promptText, promptFile string) error {
	var message string
	if promptText != "" {
		message = promptText
	} else {
		content, err := os.ReadFile(promptFile)
		if err != nil {
			return fmt.Errorf("failed to read prompt file %s: %w", promptFile, err)
		}
		// Enforce size limit (10KB) consistent with send_msg
		const maxSize = 10 * 1024
		if len(content) > maxSize {
			return fmt.Errorf("prompt file too large: %d bytes (max 10KB)", len(content))
		}
		message = string(content)
	}

	ui.PrintSuccess("Sending post-resume prompt...")
	if err := tmux.SendMultiLinePromptSafe(sessionName, message, false); err != nil {
		return fmt.Errorf("failed to send prompt: %w", err)
	}
	return nil
}

// resumeSession performs the complete resume workflow
func resumeSession(adapter *dolt.Adapter, sessionID, manifestPath, harnessName string, health *HealthStatus) error {
	sendCommands := shouldSendResumeCommands(health.TmuxExists)

	// Ensure tmux session exists
	if !health.TmuxExists {
		ui.PrintSuccess(fmt.Sprintf("Creating tmux session: %s", health.TmuxSessionName))
		if err := tmux.NewSession(health.TmuxSessionName, health.WorktreePath); err != nil {
			return fmt.Errorf("failed to create tmux session: %w", err)
		}
	} else {
		ui.PrintSuccess(fmt.Sprintf("Attaching to existing tmux session: %s", health.TmuxSessionName))
	}

	// Read manifest from Dolt to get Claude UUID (needed for both display and resume command)
	m, err := adapter.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("failed to read session from Dolt: %w", err)
	}

	// Only send commands if needed
	if sendCommands {
		// Build agent-specific resume command
		var fullCmd string

		switch harnessName {
		case "opencode-cli":
			// OpenCode uses simple attach (server maintains session state)
			fullCmd = fmt.Sprintf("cd %s && opencode attach && exit", shellQuote(health.WorktreePath))

		case "claude-code":
			// Claude uses UUID-based resume
			resumeUUID := m.Claude.UUID

			// If UUID is not in Dolt, try uuid.Discover as fallback
			if resumeUUID == "" {
				findInManifests := func(name string) (*manifest.Manifest, error) {
					manifests, listErr := adapter.ListSessions(&dolt.SessionFilter{})
					if listErr != nil {
						return nil, listErr
					}
					for _, ms := range manifests {
						if ms.Tmux.SessionName == name || ms.Name == name {
							return ms, nil
						}
					}
					return nil, fmt.Errorf("no session found for: %s", name)
				}
				discoveredUUID, discoverErr := uuidpkg.Discover(health.TmuxSessionName, findInManifests, false)
				if discoverErr == nil && discoveredUUID != "" {
					resumeUUID = discoveredUUID
					ui.PrintSuccess(fmt.Sprintf("Discovered Claude UUID via fallback: %s", resumeUUID[:8]))
				}
			}

			if resumeUUID != "" {
				fullCmd = fmt.Sprintf("cd %s && claude --resume %s && exit",
					shellQuote(health.WorktreePath),
					shellQuote(resumeUUID))
			} else {
				// Fallback to starting a new Claude session if UUID is not set
				fullCmd = fmt.Sprintf("cd %s && claude && exit", shellQuote(health.WorktreePath))
				ui.PrintWarning("No Claude UUID found - starting new Claude session")
			}

		default:
			// Other harnesses - start without resume support
			fullCmd = fmt.Sprintf("cd %s && exit", shellQuote(health.WorktreePath))
			ui.PrintWarning(fmt.Sprintf("Harness '%s' does not support resume - starting in working directory", harnessName))
		}

		// Send combined command to tmux
		if err := tmux.SendCommand(health.TmuxSessionName, fullCmd); err != nil {
			return fmt.Errorf("failed to send resume command: %w", err)
		}

		// Wait for Claude process to appear first (quick check)
		var processWaitErr error
		spinErr := spinner.New().
			Title("Waiting for Claude process to start...").
			Accessible(true).
			Action(func() {
				processWaitErr = tmux.WaitForProcessReady(health.TmuxSessionName, "claude", 15*time.Second)
			}).
			Run()
		if spinErr != nil {
			return fmt.Errorf("spinner error: %w", spinErr)
		}

		// Ensure clean line after spinner
		fmt.Println()

		if processWaitErr != nil {
			ui.PrintWarning("Claude process is taking longer than expected")
			fmt.Println("  Continuing to wait for conversation to load...")
		} else {
			ui.PrintSuccess("Claude process started!")
		}

		// Wait for conversation to load (detect prompt)
		var promptWaitErr error
		spinErr = spinner.New().
			Title("Waiting for conversation to load...").
			Accessible(true).
			Action(func() {
				// Increased timeout to 60s for resume operations (conversation loading can be slow)
				promptWaitErr = tmux.WaitForPromptSimple(health.TmuxSessionName, 60*time.Second)
			}).
			Run()
		if spinErr != nil {
			return fmt.Errorf("spinner error: %w", spinErr)
		}

		// Ensure clean line after spinner
		fmt.Println()

		if promptWaitErr != nil {
			ui.PrintWarning("Conversation is taking longer than expected to load")
			fmt.Println("  Attaching now - conversation should appear shortly")
		} else {
			ui.PrintSuccess("Conversation loaded and ready!")
		}

		// Restore permission mode if saved and different from default
		if supportsPermissionMode(harnessName) && m.PermissionMode != "" && m.PermissionMode != "default" {
			shiftTabCount := calculateShiftTabCount(m.PermissionMode)

			if shiftTabCount > 0 {
				// Verify session is at idle prompt via capture-pane before sending mode keys.
				// Bug fix: S-Tab keys must not be sent blindly without state verification.
				canReceive := session.CheckSessionDelivery(health.TmuxSessionName)
				if canReceive != state.CanReceiveYes {
					ui.PrintWarning(fmt.Sprintf("Cannot restore permission mode: session not at idle prompt (state: %s)", canReceive))
				} else {
					ui.PrintSuccess(fmt.Sprintf("Restoring permission mode: %s", m.PermissionMode))

					for i := 0; i < shiftTabCount; i++ {
						if err := tmux.SendKeys(health.TmuxSessionName, "S-Tab"); err != nil {
							ui.PrintWarning(fmt.Sprintf("Failed to restore mode: %v", err))
							break
						}
						time.Sleep(100 * time.Millisecond) // Allow tmux to process
					}
				}
			}
		}
	}

	// Update manifest last_activity (best effort - don't fail if this errors)
	if err := updateManifestActivity(adapter, sessionID, manifestPath); err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to update manifest activity: %v", err))
	}

	// Update VS Code tab title if running in VS Code
	updateVSCodeTabTitle(health.TmuxSessionName)

	// Send post-resume prompt if --prompt or --prompt-file was specified.
	// This happens after Claude is ready (conversation loaded), before attach.
	// Works for both new sessions (sendCommands=true) and existing sessions.
	if resumePrompt != "" || resumePromptFile != "" {
		if err := sendPostResumePrompt(health.TmuxSessionName, resumePrompt, resumePromptFile); err != nil {
			// Non-fatal: warn but continue so the user can still attach and type manually
			ui.PrintWarning(fmt.Sprintf("Failed to send post-resume prompt: %v", err))
		} else {
			ui.PrintSuccess("Post-resume prompt delivered.")
		}
	}

	// NOTE: No need to release global lock before attach - using fine-grained locks
	// AttachSession never holds any lock, so it can block indefinitely without issues

	// Attach to tmux session (unless --detached)
	if !resumeDetached {
		socketPath := tmux.GetSocketPath()
		debug.Log("Attaching to tmux session: %s (socket: %s)", health.TmuxSessionName, socketPath)
		ui.PrintSuccess(fmt.Sprintf("Attaching to tmux session: %s", health.TmuxSessionName))
		if sendCommands {
			fmt.Println("\nNote: You will be attached to the tmux session. Press Ctrl+B then D to detach.")
		}
		fmt.Println()

		if err := tmux.AttachSession(health.TmuxSessionName); err != nil {
			return fmt.Errorf("failed to attach to tmux session: %w", err)
		}
	} else {
		ui.PrintSuccess(fmt.Sprintf("Session '%s' resumed (detached)", health.TmuxSessionName))
		fmt.Printf("  • To attach later: tmux attach -t %s\n", health.TmuxSessionName)
		fmt.Printf("  • To view logs: agm logs %s\n", sessionID)
	}

	return nil
}

// updateManifestActivity updates the updated_at field and state in manifest (v2: auto-updated by Write)
func updateManifestActivity(adapter *dolt.Adapter, sessionID, manifestPath string) error {
	m, err := adapter.GetSession(sessionID)
	if err != nil {
		return err
	}

	// v2: UpdatedAt is automatically updated when we call UpdateSession
	// Just write the manifest back to Dolt, which will update UpdatedAt

	// Write back to Dolt
	if err := adapter.UpdateSession(m); err != nil {
		return err
	}

	// Auto-commit manifest change if in git repo
	_ = git.CommitManifest(manifestPath, "resume", m.Name) // Errors logged internally

	return nil
}

// sanitizeTmuxName sanitizes a string for safe use as tmux session name
// Only allows alphanumeric, dash, and underscore
func sanitizeTmuxName(s string) string {
	var result strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' {
			result.WriteRune(r)
		} else if r == ' ' {
			result.WriteRune('-')
		}
	}
	return result.String()
}

// generateTmuxName generates a unique tmux session name from a project path
func generateTmuxName(project string, existingSessions []string) string {
	base := filepath.Base(project)
	base = sanitizeTmuxName(base)

	// Ensure base is not empty after sanitization
	if base == "" {
		base = "session"
	}

	name := fmt.Sprintf("claude-%s", base)

	// Check for conflicts with existing sessions
	conflict := false
	for _, existing := range existingSessions {
		if existing == name {
			conflict = true
			break
		}
	}

	if !conflict {
		return name
	}

	// Add numeric suffix if conflict
	for i := 2; i < 100; i++ {
		candidate := fmt.Sprintf("%s-%d", name, i)
		conflict = false
		for _, existing := range existingSessions {
			if existing == candidate {
				conflict = true
				break
			}
		}
		if !conflict {
			return candidate
		}
	}

	// Fallback to timestamp-based suffix
	return fmt.Sprintf("%s-%d", name, time.Now().Unix()%10000)
}

// offerToImportOrphanedSession checks history.jsonl for orphaned sessions
// and prompts user to import if found
func offerToImportOrphanedSession(adapter *dolt.Adapter, identifier string) (*manifest.Manifest, string, error) {
	// Defensive check: ensure cfg is initialized
	if cfg == nil {
		return nil, "", fmt.Errorf("config not initialized")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, "", err
	}

	historyPath := filepath.Join(homeDir, ".claude", "history.jsonl")
	// Use configured sessions directory instead of hardcoded default
	sessionsDir := cfg.SessionsDir

	// Parse history (best effort - don't fail loudly on errors)
	entries, _, err := claude.ParseHistory(historyPath)
	if err != nil {
		// Return error to trigger normal "not found" message
		return nil, "", err
	}

	// Deduplicate to sessions
	sessions := claude.Deduplicate(entries)

	// Match by UUID or project path (NOT tmux, since we have no mapping for orphaned sessions)
	var matches []claude.Session
	for _, s := range sessions {
		if strings.HasPrefix(s.UUID, identifier) ||
			strings.Contains(s.Project, identifier) {
			matches = append(matches, s)
		}
	}

	if len(matches) == 0 {
		return nil, "", fmt.Errorf("no orphaned sessions found")
	}

	// Handle multiple matches - ask user to be more specific
	var session *claude.Session
	if len(matches) > 1 {
		ui.PrintWarning(fmt.Sprintf("Found %d orphaned sessions matching %q:", len(matches), identifier))
		for i, s := range matches {
			fmt.Printf("  %d. UUID: %s | Project: %s | Messages: %d\n",
				i+1, s.UUID[:8], s.Project, s.MessageCount)
		}
		return nil, "", fmt.Errorf("multiple orphaned sessions found - please be more specific (use full UUID or project path)")
	}

	session = &matches[0]

	// Display session info
	fmt.Println()
	ui.PrintWarning(fmt.Sprintf("No manifest found for %q", identifier))
	fmt.Println()
	fmt.Println("However, I found a Claude session in history that matches:")
	fmt.Printf("  UUID:          %s\n", session.UUID)
	fmt.Printf("  Project:       %s\n", session.Project)
	fmt.Printf("  Messages:      %d\n", session.MessageCount)
	fmt.Printf("  Last Activity: %s\n", session.LastActivity.Format("2006-01-02 15:04"))

	// Get active tmux sessions to avoid name conflicts
	activeTmux, _ := tmux.ListSessions()

	// Generate unique tmux name
	tmuxName := generateTmuxName(session.Project, activeTmux)
	fmt.Printf("  Tmux:          %s (will create)\n", tmuxName)
	fmt.Println()

	// Confirm with user
	var confirm bool
	err = huh.NewConfirm().
		Title("Would you like to import this session?").
		Affirmative("Yes").
		Negative("No").
		Value(&confirm).
		WithTheme(ui.GetTheme()).
		Run()
	if err != nil || !confirm {
		return nil, "", fmt.Errorf("import declined by user")
	}

	// Ensure sessions dir exists
	if err := os.MkdirAll(sessionsDir, 0700); err != nil {
		return nil, "", fmt.Errorf("failed to create sessions directory: %w", err)
	}

	// Generate session ID from UUID prefix
	sessionID := fmt.Sprintf("session-%s", session.UUID[:8])

	// Create manifest
	m, err := discovery.CreateManifest(session, sessionsDir, tmuxName, sessionID, adapter)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create manifest: %w", err)
	}

	manifestPath := filepath.Join(sessionsDir, sessionID, "manifest.yaml")
	ui.PrintSuccess(fmt.Sprintf("Created manifest: %s", manifestPath))
	fmt.Println()

	return m, manifestPath, nil
}

// calculateShiftTabCount returns number of Shift+Tab presses needed to reach target mode
// Assumes Claude always starts in "default" mode
// Cycle order: default(0) -> auto(1) -> plan(2) -> default(0)
func calculateShiftTabCount(targetMode string) int {
	modeIndex := map[string]int{"default": 0, "auto": 1, "plan": 2}
	targetIdx, ok := modeIndex[targetMode]
	if !ok {
		return 0 // Invalid mode, stay in default
	}
	// Always starting from default(0) on fresh launch
	return targetIdx
}

// supportsPermissionMode checks if harness supports permission modes
func supportsPermissionMode(harness string) bool {
	// Currently only Claude Code supports permission modes
	return harness == "claude-code"
}

func init() {
	resumeCmd.Flags().BoolVar(&resumeDetached, "detached", false, "Resume session without attaching")
	resumeCmd.Flags().BoolVar(&resumeForceParent, "force-parent", false, "Resume planning session instead of execution session")
	resumeCmd.Flags().StringVar(&resumePrompt, "prompt", "", "Prompt to send to session after resume (for crash recovery)")
	resumeCmd.Flags().StringVar(&resumePromptFile, "prompt-file", "", "File containing prompt to send after resume (max 10KB)")
	resumeCmd.MarkFlagsMutuallyExclusive("prompt", "prompt-file")
	sessionCmd.AddCommand(resumeCmd)
}
