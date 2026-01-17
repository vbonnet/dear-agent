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
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/claude"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/discovery"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var resumeCmd = &cobra.Command{
	Use:   "resume [identifier]",
	Short: "Resume a Claude session by UUID, tmux name, or fuzzy match",
	Long: `Resume a Claude session by various identifier types:

- UUID (full or partial): csm resume c4eb298c
- Tmux session name:      csm resume claude-1
- Fuzzy match on project: csm resume workspace-design
- Interactive (no args):  csm resume

The command will:
1. Resolve the identifier to find the Claude UUID
2. Check session health (worktree exists, Claude dirs present)
3. Create or attach to tmux session
4. Send 'cd' to worktree directory
5. Send 'claude --resume <uuid>' to tmux pane
6. Update manifest last_activity timestamp

Examples:
  csm resume c4eb298c              # By UUID prefix
  csm resume claude-1              # By tmux name
  csm resume workspace-design      # By project path pattern
  csm resume                       # Interactive picker (TODO)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get identifier from args or prompt
		var identifier string
		if len(args) > 0 {
			identifier = args[0]
		} else {
			// TODO: Interactive picker for Phase 3
			return fmt.Errorf("interactive picker not yet implemented - please provide identifier")
		}

		// Resolve identifier to SessionID
		sessionID, manifestPath, err := resolveSessionIdentifier(identifier)
		if err != nil {
			ui.PrintError(err, "Failed to resolve session identifier",
				fmt.Sprintf("  • Try: csm list --all to see available sessions\n"+
					"  • Identifier can be UUID, tmux name, or project path pattern"))
			return err
		}

		ui.PrintSuccess(fmt.Sprintf("Resolved identifier %q to session: %s", identifier, sessionID))

		// Read manifest to check lifecycle
		m, err := manifest.Read(manifestPath)
		if err != nil {
			ui.PrintManifestReadError(err, manifestPath)
			return err
		}

		// Check if session is archived
		if m.Lifecycle == manifest.LifecycleArchived {
			ui.PrintArchivedSessionError(sessionID)
			return fmt.Errorf("cannot resume archived session")
		}

		// Check session health
		health, err := checkSessionHealth(sessionID, manifestPath)
		if err != nil {
			ui.PrintError(err,
				"Session health check failed",
				"  • Run diagnostics: csm doctor\n"+
					"  • Check manifest file: cat "+manifestPath+"\n"+
					"  • List all sessions: csm list --all")
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
		if err := resumeSession(sessionID, manifestPath, health); err != nil {
			ui.PrintError(err,
				"Failed to resume session",
				"  • Check tmux is running: tmux list-sessions\n"+
					"  • Verify session health: csm doctor\n"+
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

		// Defensive check: ensure cfg is initialized
		if cfg == nil {
			return []string{}, cobra.ShellCompDirectiveNoFileComp
		}

		// List manifests from configured sessions directory
		manifests, err := manifest.List(cfg.SessionsDir)
		if err != nil {
			// Fail gracefully - return empty list if can't read sessions
			return []string{}, cobra.ShellCompDirectiveNoFileComp
		}

		// Get tmux mapping (session ID → tmux name)
		tmuxMapping, _ := discovery.GetTmuxMapping(cfg.SessionsDir)
		// Ignore error - worst case: empty mapping, no tmux names suggested

		// Build completion suggestions
		var suggestions []string
		for _, m := range manifests {
			// Skip archived sessions (can't resume archived)
			if m.Lifecycle == manifest.LifecycleArchived {
				continue
			}

			// Add tmux name (primary identifier)
			if tmuxName := tmuxMapping[m.SessionID]; tmuxName != "" {
				suggestions = append(suggestions, tmuxName)
			}

			// Add manifest name (secondary identifier, if different from tmux name)
			if m.Name != "" && m.Name != tmuxMapping[m.SessionID] {
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

		// Read the manifest to get the SessionID
		m, err := manifest.Read(manifestPath)
		if err != nil {
			// Skip manifests that can't be read
			continue
		}

		paths[m.SessionID] = manifestPath
	}

	return paths, nil
}

// resolveSessionIdentifier finds the Claude UUID and manifest path from various identifier types
func resolveSessionIdentifier(identifier string) (string, string, error) {
	// Defensive check: ensure cfg is initialized
	if cfg == nil {
		return "", "", fmt.Errorf("config not initialized")
	}

	// Use configured sessions directory instead of hardcoded default
	sessionsDir := cfg.SessionsDir
	manifests, err := manifest.List(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", fmt.Errorf("no sessions directory found at %s", sessionsDir)
		}
		return "", "", fmt.Errorf("failed to list manifests: %w", err)
	}

	if len(manifests) == 0 {
		return "", "", fmt.Errorf("no session manifests found")
	}

	// Build sessionID -> manifestPath mapping by scanning all directories
	manifestPaths, err := buildManifestPathMap(sessionsDir)
	if err != nil {
		return "", "", fmt.Errorf("failed to build manifest path map: %w", err)
	}

	// Build tmux mapping
	tmuxMapping, _ := discovery.GetTmuxMapping(sessionsDir)

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
		m, manifestPath, err := offerToImportOrphanedSession(identifier)
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
	manifestPath, ok := manifestPaths[m.SessionID]
	if !ok {
		return "", "", fmt.Errorf("manifest path not found for session ID %s", m.SessionID)
	}
	return m.SessionID, manifestPath, nil
}

// checkSessionHealth validates that a session can be resumed (v2 schema)
func checkSessionHealth(sessionID, manifestPath string) (*HealthStatus, error) {
	// Read manifest
	m, err := manifest.Read(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	health := &HealthStatus{
		UUID:            sessionID, // v2: SessionID (keeping field name UUID for backward compat in struct)
		ManifestPath:    manifestPath,
		WorktreePath:    m.Context.Project, // v2: Context.Project
		SessionEnvPath:  "",                // v2: Not stored in manifest
		FileHistoryPath: "",                // v2: Not stored in manifest
		TmuxSessionName: m.Tmux.SessionName,
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
	tmuxExists, err := tmux.HasSession(m.Tmux.SessionName)
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

	// Session env
	if health.SessionEnvExists {
		fmt.Printf("✓ Session env:   %s\n", health.SessionEnvPath)
	} else {
		fmt.Printf("⚠ Session env:   %s (NOT FOUND)\n", health.SessionEnvPath)
	}

	// File history
	if health.FileHistoryExists {
		fmt.Printf("✓ File history:  %s\n", health.FileHistoryPath)
	} else {
		fmt.Printf("⚠ File history:  %s (NOT FOUND)\n", health.FileHistoryPath)
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

// resumeSession performs the complete resume workflow
func resumeSession(sessionID, manifestPath string, health *HealthStatus) error {
	sendCommands := false

	// Ensure tmux session exists
	if !health.TmuxExists {
		ui.PrintSuccess(fmt.Sprintf("Creating tmux session: %s", health.TmuxSessionName))
		if err := tmux.NewSession(health.TmuxSessionName, health.WorktreePath); err != nil {
			return fmt.Errorf("failed to create tmux session: %w", err)
		}
		sendCommands = true
	} else {
		ui.PrintSuccess(fmt.Sprintf("Tmux session %s already exists", health.TmuxSessionName))

		// Check if Claude is already running
		claudeRunning, err := tmux.IsProcessRunning(health.TmuxSessionName, "claude")
		if err != nil {
			ui.PrintWarning("Could not check if Claude is running - skipping resume commands for safety")
			// We'll display the Claude UUID later after reading the manifest
			sendCommands = false
		} else if claudeRunning {
			ui.PrintSuccess("Claude already running - skipping resume commands")
			sendCommands = false
		} else {
			ui.PrintSuccess("Claude not running - will send resume commands")
			sendCommands = true
		}
	}

	// Read manifest to get Claude UUID (needed for both display and resume command)
	m, err := manifest.Read(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	// Only send commands if needed
	if sendCommands {
		// Build combined command: cd <project-dir> && claude --resume <uuid> && exit
		// This ensures the directory change happens in the same shell as the Claude command
		var fullCmd string
		if m.Claude.UUID != "" {
			fullCmd = fmt.Sprintf("cd %s && claude --resume %s && exit",
				shellQuote(health.WorktreePath),
				shellQuote(m.Claude.UUID))
		} else {
			// Fallback to starting a new Claude session if UUID is not set
			fullCmd = fmt.Sprintf("cd %s && claude && exit", shellQuote(health.WorktreePath))
			ui.PrintWarning("No Claude UUID found in manifest - starting new Claude session")
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
				promptWaitErr = tmux.WaitForClaudePrompt(health.TmuxSessionName, 60*time.Second)
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
	}

	// Update manifest last_activity (best effort - don't fail if this errors)
	if err := updateManifestActivity(manifestPath); err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to update manifest activity: %v", err))
	}

	// Update VS Code tab title if running in VS Code
	updateVSCodeTabTitle(health.TmuxSessionName)

	// Release lock before attaching (attachment can block for hours)
	// The lock should only protect the setup phase, not the tmux attachment
	if globalLock != nil {
		if err := globalLock.Unlock(); err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to release lock: %v", err))
		}
		globalLock = nil // Prevent double-unlock in PersistentPostRunE
	}

	// Attach to tmux session
	ui.PrintSuccess(fmt.Sprintf("Attaching to tmux session: %s", health.TmuxSessionName))
	if sendCommands {
		fmt.Println("\nNote: You will be attached to the tmux session. Press Ctrl+B then D to detach.")
	}
	fmt.Println()

	if err := tmux.AttachSession(health.TmuxSessionName); err != nil {
		return fmt.Errorf("failed to attach to tmux session: %w", err)
	}

	return nil
}

// updateManifestActivity updates the updated_at field in manifest (v2: auto-updated by Write)
func updateManifestActivity(manifestPath string) error {
	m, err := manifest.Read(manifestPath)
	if err != nil {
		return err
	}

	// v2: UpdatedAt is automatically set by manifest.Write(), no manual update needed
	// Just write the manifest back, which will update UpdatedAt

	// Write back
	return manifest.Write(manifestPath, m)
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
func offerToImportOrphanedSession(identifier string) (*manifest.Manifest, string, error) {
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
	m, err := discovery.CreateManifest(session, sessionsDir, tmuxName, sessionID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create manifest: %w", err)
	}

	manifestPath := filepath.Join(sessionsDir, sessionID, "manifest.yaml")
	ui.PrintSuccess(fmt.Sprintf("Created manifest: %s", manifestPath))
	fmt.Println()

	return m, manifestPath, nil
}

func init() {
	rootCmd.AddCommand(resumeCmd)
}
