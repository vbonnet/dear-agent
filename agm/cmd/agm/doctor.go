package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/freshness"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/orphan"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	"github.com/vbonnet/dear-agent/agm/internal/validate"
)

var (
	validateFlag   bool
	applyFixesFlag bool
	jsonFormat     bool
	doctorTestMode bool
)

// getDoctorSessionsDir returns the sessions directory based on test mode
func getDoctorSessionsDir() string {
	// Test mode overrides config
	if doctorTestMode {
		homeDir, _ := os.UserHomeDir()
		return homeDir + "/sessions-test"
	}
	if cfg != nil && cfg.SessionsDir != "" {
		return cfg.SessionsDir
	}
	// Default to ~/sessions
	homeDir, _ := os.UserHomeDir()
	return homeDir + "/sessions"
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system health and configuration",
	Long: `Verify that Claude, tmux, and all sessions are healthy.

USAGE MODES:

1. Quick Health Check (DEFAULT - Fast, ~1-5 seconds)
   Run: agm admin doctor

   Performs structural checks:
   - Required binaries in PATH (agm, agm-reaper, agm-mcp-server)
   - Hook files installed with agm-/engram- prefixes
   - Hook registrations in settings.json
   - AGM config validity
   - Go version minimum
   - Claude and tmux installation status
   - Duplicate session directories (old vs new naming format)
   - Sessions sharing the same Claude UUID
   - Sessions with empty/missing Claude UUIDs
   - Orphaned session directories
   - Invalid manifest files

   Use when: You want a quick overview of system health

2. Deep Validation (OPTIONAL - Slower, ~5-30 seconds per session)
   Run: agm admin doctor --validate

   Performs structural checks PLUS functional testing:
   - Tests actual session resumability by attempting resume
   - Classifies resume errors and suggests fixes
   - Auto-fixes issues with --apply-fixes flag

   Use when: Debugging session resume failures or preparing for production

WHICH MODE TO USE:
- Daily health checks: Use default mode (fast)
- Troubleshooting resume issues: Use --validate mode (thorough)
- CI/CD pipelines: Use --validate --json for automated testing

Examples:
  agm admin doctor                             # Quick health check (structural only)
  agm admin doctor --validate                  # Thorough validation (structural + functional)
  agm admin doctor --validate --apply-fixes    # Test and auto-fix issues
  agm admin doctor --validate --json           # JSON output for scripting
  agm admin doctor --test                      # Check test sessions in ~/sessions-test/`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(ui.Blue("=== Claude Session Manager Health Check ===\n"))

		allHealthy := true

		// Get sessions directory (test mode or production)
		sessionsDir := getDoctorSessionsDir()

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
					fmt.Println("  • Run 'agm session new' to start a session (stale socket will be cleaned)")
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

		// Check binary freshness
		fmt.Println(ui.Blue("\n--- Checking binary freshness ---"))
		repoPath, repoErr := freshness.FindRepoPath()
		if repoErr != nil {
			ui.PrintWarning(fmt.Sprintf("Could not locate source repo: %v", repoErr))
		} else {
			freshnessResult := freshness.Check(repoPath, GitCommit)
			if freshnessResult.Error != nil {
				ui.PrintWarning(fmt.Sprintf("Could not check binary freshness: %v", freshnessResult.Error))
			} else if freshnessResult.Stale {
				ui.PrintWarning("Binary is stale!")
				fmt.Printf("  Binary commit: %s\n", freshnessResult.BinaryCommit)
				fmt.Printf("  Repo HEAD:     %s\n", freshnessResult.RepoHEAD)
				fmt.Printf("  Fix: make -C %s install\n", freshnessResult.RepoPath)
				allHealthy = false
			} else {
				ui.PrintSuccess(fmt.Sprintf("Binary is up to date (commit %s)", freshnessResult.BinaryCommit))
			}
		}

		// Run installation checks (binaries, hooks, settings, config, Go version)
		if !runInstallChecks() {
			allHealthy = false
		}

		// Check sessions from Dolt
		adapter, err := getStorage()
		if err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to connect to Dolt storage: %v", err))
			ui.PrintSuccess("Run 'agm admin sync' to sync sessions")
			return nil
		}
		defer adapter.Close()

		manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
		if err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to list sessions from Dolt: %v", err))
			ui.PrintSuccess("Run 'agm admin sync' to sync sessions")
		} else {
			ui.PrintSuccess(fmt.Sprintf("Found %d session manifests", len(manifests)))

			// If --validate flag is set, run functional validation
			if validateFlag {
				return runValidation(manifests, applyFixesFlag, jsonFormat)
			}

			// === NEW DIAGNOSTICS ===

			// 1. Check for duplicate session directories (old vs new format)
			fmt.Println(ui.Blue("\n--- Checking for duplicate session directories ---"))
			duplicates := detectDuplicateSessionDirs(sessionsDir)
			if len(duplicates) > 0 {
				ui.PrintWarning(fmt.Sprintf("Found %d duplicate session directories", len(duplicates)))
				for _, dup := range duplicates {
					fmt.Printf("  • '%s' has both:\n", dup.SessionName)
					fmt.Printf("    - Old format: %s\n", dup.OldFormat)
					fmt.Printf("    - New format: %s\n", dup.NewFormat)
				}
				fmt.Println("\n  Recommendation: Archive old format directories")
				fmt.Printf("    mkdir -p %s/.archive-old-format\n", sessionsDir)
				fmt.Printf("    mv %s/claude-*-session %s/.archive-old-format/\n", sessionsDir, sessionsDir)
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
				fmt.Println("    Use 'agm session associate <session-name>' to assign correct UUIDs")
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
				fmt.Println("    Use 'agm session associate <session-name>' to link")
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
					manifestPath := filepath.Join(sessionsDir, m.SessionID, "manifest.yaml")
					ui.PrintError(err,
						fmt.Sprintf("Failed to check health of session %s", m.SessionID),
						"  • Check manifest file: cat "+manifestPath+"\n"+
							"  • Verify manifest format: agm session list --format=json | grep "+m.SessionID+"\n"+
							"  • Try manual resume: agm session resume "+m.Name)
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

			// 5. Check for orphaned conversations
			fmt.Println(ui.Blue("\n--- Checking for orphaned conversations ---"))
			orphanReport, err := orphan.DetectOrphans(sessionsDir, "", adapter)
			if err != nil {
				ui.PrintError(err,
					"Failed to detect orphaned conversations",
					"  • Check Claude history: ~/.claude/history.jsonl\n"+
						"  • Verify sessions directory: "+sessionsDir)
				allHealthy = false
			} else {
				if orphanReport.TotalOrphans > 0 {
					ui.PrintWarning(fmt.Sprintf("Found %d orphaned session(s)", orphanReport.TotalOrphans))

					// Show workspace breakdown
					if len(orphanReport.ByWorkspace) > 0 {
						for workspace, count := range orphanReport.ByWorkspace {
							wsName := workspace
							if wsName == "" {
								wsName = "(no workspace)"
							}
							fmt.Printf("  • %s: %d orphan(s)\n", wsName, count)
						}
					}

					fmt.Println("\n  Recommendation: Import orphaned sessions to restore AGM tracking")
					fmt.Println("    Run: agm admin find-orphans --auto-import")
					fmt.Println("\n  Impact: Risk of lost work if sessions are not recovered")
					allHealthy = false
				} else {
					ui.PrintSuccess("No orphaned sessions found")
				}
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

func runValidation(manifests []*manifest.Manifest, fix bool, json bool) error {
	if len(manifests) == 0 {
		ui.PrintWarning("No sessions found to validate")
		return nil
	}

	fmt.Println(ui.Blue("\n=== Testing Session Resumability ===\n"))

	opts := &validate.Options{
		AutoFix:           fix,
		JSONOutput:        json,
		TimeoutPerSession: 15, // 15 seconds per session
	}

	report, err := validate.RunValidation(manifests, opts)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Output results
	if json {
		validate.PrintJSON(report)
	} else {
		validate.PrintText(report)
	}

	// Return error if any sessions failed
	if report.Failed > 0 {
		return fmt.Errorf("validation failed: %d/%d sessions cannot resume",
			report.Failed, report.TotalSessions)
	}

	return nil
}

func init() {
	adminCmd.AddCommand(doctorCmd)
	doctorCmd.Flags().BoolVar(&validateFlag, "validate", false,
		"Test actual session resumability")
	doctorCmd.Flags().BoolVar(&applyFixesFlag, "apply-fixes", false,
		"Apply suggested fixes (requires --validate)")
	doctorCmd.Flags().BoolVar(&jsonFormat, "json", false,
		"Output results as JSON")
	doctorCmd.Flags().BoolVar(&doctorTestMode, "test", false,
		"Check test sessions in ~/sessions-test/ (isolated from production)")
}
