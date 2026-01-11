package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/session"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system health and configuration",
	Long: `Verify that Claude, tmux, and all sessions are healthy.

Detects:
- Duplicate session directories (old vs new naming format)
- Sessions sharing the same Claude UUID
- Sessions with empty/missing Claude UUIDs
- Orphaned session directories
- Invalid manifest files

Examples:
  csm doctor    # Run health checks`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(ui.Blue("=== Claude Session Manager Health Check ===\n"))

		allHealthy := true

		// Check Claude installation (verify history.jsonl exists)
		homeDir, _ := os.UserHomeDir()
		historyPath := filepath.Join(homeDir, ".claude", "history.jsonl")
		if _, err := os.Stat(historyPath); err != nil {
			ui.PrintError(err, "Claude history not found", "  • Install Claude from https://claude.com\n  • Run Claude at least once")
			allHealthy = false
		} else {
			ui.PrintSuccess("Claude history found")
		}

		// Check tmux installation
		tmuxVersion, err := tmux.Version()
		if err != nil {
			ui.PrintError(err, "tmux not found or not working", "  • Install tmux: sudo apt install tmux")
			allHealthy = false
		} else {
			ui.PrintSuccess(fmt.Sprintf("tmux installed: %s", tmuxVersion))
		}

		// Check tmux socket
		socketInfo, err := tmux.GetSocketInfo()
		if err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to check socket: %v", err))
		} else {
			if socketInfo.Exists {
				if socketInfo.Accessible {
					ui.PrintSuccess(fmt.Sprintf("tmux socket active: %s", socketInfo.Path))
				} else if socketInfo.IsStale {
					ui.PrintWarning(fmt.Sprintf("tmux socket is stale: %s", socketInfo.Path))
					fmt.Println("  • Run 'csm new' to start a session (stale socket will be cleaned)")
					allHealthy = false
				}
			} else {
				ui.PrintSuccess(fmt.Sprintf("tmux socket ready: %s (will be created on first use)", socketInfo.Path))
			}
		}

		// Check user lingering (session persistence)
		lingerStatus, err := tmux.CheckLingering()
		if err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to check lingering: %v", err))
		} else {
			if lingerStatus.LoginctlExists {
				if lingerStatus.Enabled {
					ui.PrintSuccess(fmt.Sprintf("User lingering enabled (sessions persist after logout)"))
				} else {
					ui.PrintWarning("User lingering DISABLED - sessions will be killed on logout")
					fmt.Printf("  • Fix: Run 'loginctl enable-linger %s'\n", lingerStatus.Username)
					fmt.Println("  • This prevents systemd from killing tmux when you disconnect")
					allHealthy = false
				}
			} else {
				ui.PrintSuccess("Lingering check skipped (systemd not available)")
			}
		}

		// Check sessions directory
		manifests, err := manifest.List(cfg.SessionsDir)
		if err != nil {
			ui.PrintWarning(fmt.Sprintf("Sessions directory not found: %s", cfg.SessionsDir))
			ui.PrintSuccess("Run 'csm sync' to create manifests")
		} else {
			ui.PrintSuccess(fmt.Sprintf("Found %d session manifests", len(manifests)))

			// === NEW DIAGNOSTICS ===

			// 1. Check for duplicate session directories (old vs new format)
			fmt.Println(ui.Blue("\n--- Checking for duplicate session directories ---"))
			duplicates := detectDuplicateSessionDirs(cfg.SessionsDir)
			if len(duplicates) > 0 {
				ui.PrintWarning(fmt.Sprintf("Found %d duplicate session directories", len(duplicates)))
				for _, dup := range duplicates {
					fmt.Printf("  • '%s' has both:\n", dup.SessionName)
					fmt.Printf("    - Old format: %s\n", dup.OldFormat)
					fmt.Printf("    - New format: %s\n", dup.NewFormat)
				}
				fmt.Println("\n  Recommendation: Archive old format directories")
				fmt.Printf("    mkdir -p %s/.archive-old-format\n", cfg.SessionsDir)
				fmt.Printf("    mv %s/claude-*-session %s/.archive-old-format/\n", cfg.SessionsDir, cfg.SessionsDir)
				allHealthy = false
			} else {
				ui.PrintSuccess("No duplicate session directories found")
			}

			// 2. Check for sessions sharing the same Claude UUID
			fmt.Println(ui.Blue("\n--- Checking for duplicate Claude UUIDs ---"))
			uuidMap := make(map[string][]string) // UUID -> list of session names
			emptyUUIDs := []string{}

			for _, m := range manifests {
				if m.Claude.UUID == "" {
					emptyUUIDs = append(emptyUUIDs, m.Name)
				} else {
					uuidMap[m.Claude.UUID] = append(uuidMap[m.Claude.UUID], m.Name)
				}
			}

			// Find duplicate UUIDs
			duplicateUUIDs := make(map[string][]string)
			for uuid, sessions := range uuidMap {
				if len(sessions) > 1 {
					duplicateUUIDs[uuid] = sessions
				}
			}

			if len(duplicateUUIDs) > 0 {
				ui.PrintWarning(fmt.Sprintf("Found %d Claude UUID(s) shared by multiple sessions", len(duplicateUUIDs)))
				for uuid, sessions := range duplicateUUIDs {
					fmt.Printf("  • UUID %s... is shared by %d sessions:\n", uuid[:8], len(sessions))
					for _, sessName := range sessions {
						fmt.Printf("    - %s\n", sessName)
					}
				}
				fmt.Println("\n  Recommendation: Each session should have a unique Claude UUID")
				fmt.Println("    Use 'csm associate <session-name>' to assign correct UUIDs")
				allHealthy = false
			} else {
				ui.PrintSuccess("No duplicate Claude UUIDs found")
			}

			// 3. Check for sessions with empty Claude UUIDs
			if len(emptyUUIDs) > 0 {
				ui.PrintWarning(fmt.Sprintf("Found %d session(s) with empty Claude UUID", len(emptyUUIDs)))
				for _, sessName := range emptyUUIDs {
					fmt.Printf("  • %s\n", sessName)
				}
				fmt.Println("\n  Recommendation: Associate each session with its Claude conversation")
				fmt.Println("    Use 'csm associate <session-name>' to link")
				allHealthy = false
			} else {
				ui.PrintSuccess("All sessions have Claude UUIDs")
			}

			// 4. Check health of each session
			fmt.Println(ui.Blue("\n--- Checking session health ---"))
			unhealthyCount := 0
			for _, m := range manifests {
				health, err := session.CheckHealth(m)
				if err != nil {
					ui.PrintError(err, fmt.Sprintf("Failed to check health of %s", m.SessionID), "")
					unhealthyCount++
					continue
				}

				if !health.IsHealthy() {
					ui.PrintWarning(fmt.Sprintf("Unhealthy session: %s", m.SessionID))
					fmt.Println(health.Summary())
					unhealthyCount++
				}
			}

			if unhealthyCount > 0 {
				ui.PrintWarning(fmt.Sprintf("%d unhealthy sessions found", unhealthyCount))
				allHealthy = false
			} else if len(manifests) > 0 {
				ui.PrintSuccess("All sessions are healthy")
			}
		}

		// Overall status
		fmt.Println()
		if allHealthy {
			ui.PrintSuccess("✓ System is healthy")
		} else {
			ui.PrintWarning("⚠ Some issues found - see recommendations above")
			return fmt.Errorf("health check failed")
		}

		return nil
	},
}

// DuplicateSessionDir represents a session with both old and new format directories
type DuplicateSessionDir struct {
	SessionName string
	OldFormat   string // e.g., claude-1-session
	NewFormat   string // e.g., session-claude-1
}

// detectDuplicateSessionDirs finds sessions with both old and new format directories
func detectDuplicateSessionDirs(sessionsDir string) []DuplicateSessionDir {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil
	}

	// Map: session name -> directory formats
	sessionDirs := make(map[string]map[string]string) // name -> {format -> dirName}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := entry.Name()

		// Skip hidden directories and archives
		if strings.HasPrefix(dirName, ".") {
			continue
		}

		var sessionName string
		var format string

		// Check for old format: claude-N-session or <name>-session
		if strings.HasSuffix(dirName, "-session") {
			sessionName = strings.TrimSuffix(dirName, "-session")
			format = "old"
		} else if strings.HasPrefix(dirName, "session-") {
			// New format: session-<name>
			sessionName = strings.TrimPrefix(dirName, "session-")
			format = "new"
		} else {
			// Unknown format, skip
			continue
		}

		if sessionDirs[sessionName] == nil {
			sessionDirs[sessionName] = make(map[string]string)
		}
		sessionDirs[sessionName][format] = dirName
	}

	// Find duplicates (sessions with both old and new format)
	var duplicates []DuplicateSessionDir
	for sessionName, formats := range sessionDirs {
		if oldDir, hasOld := formats["old"]; hasOld {
			if newDir, hasNew := formats["new"]; hasNew {
				duplicates = append(duplicates, DuplicateSessionDir{
					SessionName: sessionName,
					OldFormat:   oldDir,
					NewFormat:   newDir,
				})
			}
		}
	}

	return duplicates
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
