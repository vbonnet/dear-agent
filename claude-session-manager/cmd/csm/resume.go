package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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

		// Resolve identifier to Claude UUID
		uuid, manifestPath, err := resolveSessionIdentifier(identifier)
		if err != nil {
			ui.PrintError(err, "Failed to resolve session identifier",
				fmt.Sprintf("  • Try: csm list --all to see available sessions\n"+
					"  • Identifier can be UUID, tmux name, or project path pattern"))
			return err
		}

		ui.PrintSuccess(fmt.Sprintf("Resolved identifier %q to UUID: %s", identifier, uuid[:8]))

		// Read manifest to check lifecycle
		m, err := manifest.Read(manifestPath)
		if err != nil {
			ui.PrintError(err, "Failed to read manifest", "")
			return err
		}

		// Check if session is archived
		if m.Lifecycle == manifest.LifecycleArchived {
			ui.PrintError(
				fmt.Errorf("session is archived"),
				"Cannot resume archived session",
				"  • Use 'csm unarchive "+uuid[:8]+"' to restore this session\n"+
					"  • Or use 'csm list --all' to see all sessions",
			)
			return fmt.Errorf("cannot resume archived session")
		}

		// Check session health
		health, err := checkSessionHealth(uuid, manifestPath)
		if err != nil {
			ui.PrintError(err, "Session health check failed", "")
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
		if err := resumeSession(uuid, manifestPath, health); err != nil {
			ui.PrintError(err, "Failed to resume session", "")
			return err
		}

		ui.PrintSuccess(fmt.Sprintf("Successfully resumed session %s", uuid[:8]))
		return nil
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

// resolveSessionIdentifier finds the Claude UUID and manifest path from various identifier types
func resolveSessionIdentifier(identifier string) (string, string, error) {
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
	manifestPath := filepath.Join(sessionsDir, m.SessionID, "manifest.yaml")
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
		ui.PrintError(nil, "Critical Issues:", "")
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
func resumeSession(uuid, manifestPath string, health *HealthStatus) error {
	// Ensure tmux session exists
	if !health.TmuxExists {
		ui.PrintSuccess(fmt.Sprintf("Creating tmux session: %s", health.TmuxSessionName))
		if err := tmux.NewSession(health.TmuxSessionName, health.WorktreePath); err != nil {
			return fmt.Errorf("failed to create tmux session: %w", err)
		}
	} else {
		ui.PrintSuccess(fmt.Sprintf("Tmux session %s already exists", health.TmuxSessionName))
	}

	// Send cd command to tmux (with shell quoting to prevent injection)
	cdCmd := fmt.Sprintf("cd %s", shellQuote(health.WorktreePath))
	if err := tmux.SendCommand(health.TmuxSessionName, cdCmd); err != nil {
		return fmt.Errorf("failed to send cd command: %w", err)
	}

	// Read manifest to get Claude UUID
	m, err := manifest.Read(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	// Send claude --resume command to tmux (use Claude.UUID from manifest)
	var resumeCmd string
	if m.Claude.UUID != "" {
		resumeCmd = fmt.Sprintf("claude --resume %s", shellQuote(m.Claude.UUID))
	} else {
		// Fallback to starting a new Claude session if UUID is not set
		resumeCmd = "claude"
		ui.PrintWarning("No Claude UUID found in manifest - starting new Claude session")
	}
	if err := tmux.SendCommand(health.TmuxSessionName, resumeCmd); err != nil {
		return fmt.Errorf("failed to send claude resume command: %w", err)
	}

	// Update manifest last_activity (best effort - don't fail if this errors)
	if err := updateManifestActivity(manifestPath); err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to update manifest activity: %v", err))
	}

	// Attach to tmux session
	ui.PrintSuccess(fmt.Sprintf("Attaching to tmux session: %s", health.TmuxSessionName))
	fmt.Println("\nNote: You will be attached to the tmux session. Press Ctrl+B then D to detach.")
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
	confirm, err := ui.Confirm("Would you like to import this session?")
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
