package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
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
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("failed to get home directory: %w", err)
	}

	sessionsDir := filepath.Join(homeDir, "sessions")
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

	// Strategy 1: UUID match (full or partial)
	for _, m := range manifests {
		if strings.HasPrefix(m.Claude.SessionID, identifier) || m.Claude.SessionID == identifier {
			matches = append(matches, m)
			matchType = "UUID"
		}
	}

	// Strategy 2: Tmux session name match
	if len(matches) == 0 {
		for uuid, tmuxName := range tmuxMapping {
			if tmuxName == identifier {
				// Find manifest with this UUID
				for _, m := range manifests {
					if m.Claude.SessionID == uuid {
						matches = append(matches, m)
						matchType = "tmux name"
						break
					}
				}
			}
		}
	}

	// Strategy 3: Fuzzy match on project path
	if len(matches) == 0 {
		for _, m := range manifests {
			if strings.Contains(m.Worktree.Path, identifier) {
				matches = append(matches, m)
				matchType = "project path"
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
		return "", "", fmt.Errorf("no sessions found matching %q", identifier)
	}

	if len(matches) > 1 {
		// Multiple matches - show user and ask to be more specific
		ui.PrintWarning(fmt.Sprintf("Multiple sessions matched %q by %s:", identifier, matchType))
		for i, m := range matches {
			tmuxName := tmuxMapping[m.Claude.SessionID]
			if tmuxName == "" {
				tmuxName = "-"
			}
			fmt.Printf("  %d. UUID: %s | Tmux: %s | Project: %s\n",
				i+1, m.Claude.SessionID[:8], tmuxName, m.Worktree.Path)
		}
		return "", "", fmt.Errorf("ambiguous identifier - please be more specific")
	}

	// Single match found
	m := matches[0]
	manifestPath := filepath.Join(sessionsDir, m.SessionID, "manifest.yaml")
	return m.Claude.SessionID, manifestPath, nil
}

// checkSessionHealth validates that a session can be resumed
func checkSessionHealth(uuid, manifestPath string) (*HealthStatus, error) {
	// Read manifest
	m, err := manifest.Read(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	health := &HealthStatus{
		UUID:            uuid,
		ManifestPath:    manifestPath,
		WorktreePath:    m.Worktree.Path,
		SessionEnvPath:  m.Claude.SessionEnvPath,
		FileHistoryPath: m.Claude.FileHistoryPath,
		TmuxSessionName: m.Tmux.SessionName,
		Issues:          []string{},
		Warnings:        []string{},
		CanResume:       true,
	}

	// Check worktree exists
	if _, err := os.Stat(m.Worktree.Path); os.IsNotExist(err) {
		health.WorktreeExists = false
		health.Issues = append(health.Issues,
			fmt.Sprintf("Worktree directory not found: %s", m.Worktree.Path))
		health.CanResume = false
	} else {
		health.WorktreeExists = true
	}

	// Check session-env exists
	if _, err := os.Stat(m.Claude.SessionEnvPath); os.IsNotExist(err) {
		health.SessionEnvExists = false
		health.Warnings = append(health.Warnings,
			fmt.Sprintf("Session env directory not found: %s", m.Claude.SessionEnvPath))
	} else {
		health.SessionEnvExists = true
	}

	// Check file-history exists (optional - just a warning)
	if _, err := os.Stat(m.Claude.FileHistoryPath); os.IsNotExist(err) {
		health.FileHistoryExists = false
		health.Warnings = append(health.Warnings,
			fmt.Sprintf("File history directory not found: %s", m.Claude.FileHistoryPath))
	} else {
		health.FileHistoryExists = true
	}

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

	// Send cd command to tmux
	cdCmd := fmt.Sprintf("cd %s", health.WorktreePath)
	if err := tmux.SendCommand(health.TmuxSessionName, cdCmd); err != nil {
		return fmt.Errorf("failed to send cd command: %w", err)
	}

	// Send claude --resume command to tmux
	resumeCmd := fmt.Sprintf("claude --resume %s", uuid)
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

// updateManifestActivity updates the last_activity field in manifest
func updateManifestActivity(manifestPath string) error {
	m, err := manifest.Read(manifestPath)
	if err != nil {
		return err
	}

	// Update last activity
	now := time.Now()
	m.LastActivity = now
	m.Claude.LastActivity = now

	// Write back
	return manifest.Write(manifestPath, m)
}

func init() {
	rootCmd.AddCommand(resumeCmd)
}
