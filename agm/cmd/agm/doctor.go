package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/config"
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

// getDoctorSessionsDir returns the sessions directory for compatibility
// callers that do not already hold a retained runtime HOME.
func getDoctorSessionsDir() (string, error) {
	if !doctorTestMode && cfg != nil && cfg.SessionsDir != "" {
		return cfg.SessionsDir, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve HOME for sessions directory: %w", err)
	}
	return getDoctorSessionsDirAtHome(homeDir), nil
}

func getDoctorSessionsDirAtHome(homeDir string) string {
	// Test mode overrides config
	if doctorTestMode {
		return filepath.Join(homeDir, "sessions-test")
	}
	if cfg != nil && cfg.SessionsDir != "" {
		return cfg.SessionsDir
	}
	// Default to ~/sessions
	return filepath.Join(homeDir, "sessions")
}

func resolveDoctorHomePath(snapshot *config.Config) (string, error) {
	authority, err := snapshot.RuntimeAuthority()
	if err != nil {
		return "", fmt.Errorf("doctor runtime authority: %w", err)
	}
	home, err := authority.Home()
	if err != nil {
		return "", fmt.Errorf("doctor HOME authority: %w", err)
	}
	path, err := home.Path()
	if err != nil {
		return "", fmt.Errorf("doctor HOME path: %w", err)
	}
	return path, nil
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
   - Per-harness health for every active harness in use (claude-code,
     codex-cli, agy, opencode-cli): CLI binary on PATH, auth/config, config dir
     Deprecated compatibility: gemini-cli

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
		homeDir, err := resolveDoctorHomePath(cfg)
		if err != nil {
			return err
		}

		// Get sessions directory (test mode or production)
		sessionsDir := getDoctorSessionsDirAtHome(homeDir)

		// Check Claude installation (verify history.jsonl exists)
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

		// Check tmux socket.
		//
		// The orphan check comes first (ce-7ep9): a tmux server whose socket
		// file was unlinked keeps running but is unreachable, so its sessions
		// are stranded. That state looks identical to "no socket yet" from the
		// filesystem alone, and reporting it as ready-for-first-use hides an
		// outage. Only the process table can tell the two apart.
		orphanedServer, orphanErr := tmux.DetectOrphanedServer()
		switch {
		case orphanErr != nil:
			ui.PrintWarning(fmt.Sprintf("Failed to check for orphaned tmux server: %v", orphanErr))
			// The socket check is a safety gate: an inconclusive process scan
			// cannot prove that no sessions are stranded, so it must not leave
			// the overall diagnosis green.
			allHealthy = false
		case orphanedServer != nil:
			ui.PrintError(
				fmt.Errorf("tmux server running but unreachable: pid(s) %v", orphanedServer.PIDs),
				fmt.Sprintf("orphaned tmux server on %s (socket file missing or unusable)", orphanedServer.SocketPath),
				fmt.Sprintf("  • Sessions on this server are stranded and cannot be recovered\n"+
					"  • Kill the orphaned server: kill %s\n"+
					"  • Then verify: agm session list",
					formatPIDs(orphanedServer.PIDs)))
			allHealthy = false
		default:
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
		}

		// Check user lingering (session persistence)
		lingerStatus, err := tmux.CheckLingering()
		if err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to check lingering: %v", err))
		} else {
			if lingerStatus.LoginctlExists {
				if lingerStatus.Enabled {
					ui.PrintSuccess("User lingering enabled (sessions persist after logout)")
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

			// Deployment verification: the running binary's embedded vcs.revision
			// must be an ancestor of origin/main. This catches the rolled-back
			// checkout case (a binary built from a commit not on trunk) that the
			// HEAD-equality freshness check above cannot see — the exact failure
			// that deadlocked the VROOM mesh on 2026-06-18 (ce-mb9g / retro B1).
			fmt.Println(ui.Blue("\n--- Verifying deployment (vcs ancestry vs origin/main) ---"))
			ancestry := freshness.VerifyAncestry(repoPath, GitCommit, "")
			switch {
			case ancestry.OK():
				ui.PrintSuccess(fmt.Sprintf("Deployed binary is on trunk (commit %s ∈ %s)", ancestry.BinaryCommit, ancestry.TrunkRef))
			case ancestry.FailLoud():
				ui.PrintWarning("Deployment verification FAILED — running binary is not on origin/main!")
				fmt.Printf("  %s\n", ancestry.Reason())
				fmt.Printf("  Binary commit: %s\n", ancestry.BinaryCommit)
				if ancestry.TrunkCommit != "" {
					fmt.Printf("  %s:    %s\n", ancestry.TrunkRef, ancestry.TrunkCommit)
				}
				fmt.Printf("  Fix: rebuild from a clean checkout of origin/main, e.g.\n")
				fmt.Printf("       git -C %s fetch origin && git -C %s checkout origin/main && make -C %s install\n", ancestry.RepoPath, ancestry.RepoPath, ancestry.RepoPath)
				allHealthy = false
			default: // AncestryIndeterminate — fail open
				ui.PrintWarning(fmt.Sprintf("Could not verify deployment ancestry: %v", ancestry.Reason()))
			}
		}

		// Check token-refresher binary freshness (vcs ancestry vs origin/main).
		fmt.Println(ui.Blue("\n--- Checking token-refresher freshness ---"))
		if !checkTokenRefresherFreshness() {
			allHealthy = false
		}

		// Run installation checks (binaries, hooks, settings, config, Go version)
		if !runInstallChecks(homeDir) {
			allHealthy = false
		}

		// Check sessions from Dolt
		adapter, err := getStorage()
		if err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to connect to Dolt storage: %v", err))
			ui.PrintSuccess("Run 'agm admin sync' to sync sessions")
			return nil
		}
		defer func() { _ = adapter.Close() }()

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

			// 6. Per-harness health (gated by the harness each session uses)
			fmt.Println(ui.Blue("\n--- Checking per-harness health ---"))
			if !checkHarnessHealth(manifests, homeDir) {
				allHealthy = false
			}

			// 7. Check for mesh-wide send-block incidents.
			// When the same guard violation blocks send.msg across N≥2 sessions
			// within the recent window, emit one consolidated warning rather than
			// letting each supervisor rediscover the block independently.
			// See: 2026-06-18 ghost-text outage (ce-q7am).
			fmt.Println(ui.Blue("\n--- Checking for mesh-wide send-block ---"))
			if !checkMeshWideSendBlock() {
				allHealthy = false
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
		switch {
		case strings.HasSuffix(dirName, "-session"):
			sessionName = strings.TrimSuffix(dirName, "-session")
			format = "old"
		case strings.HasPrefix(dirName, "session-"):
			// New format: session-<name>
			sessionName = strings.TrimPrefix(dirName, "session-")
			format = "new"
		default:
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

// checkHarnessHealth runs per-harness health checks for every harness in use
// across the session manifests. AGM's active parity harnesses are claude-code,
// codex-cli, agy, opencode-cli, and pi-cli; gemini-cli is deprecated compatibility.
// This surfaces a missing binary, missing
// auth/config, or absent config dir for whichever harnesses the sessions
// actually use — without privileging Claude.
//
// Sessions with an empty harness field are treated as claude-code (the
// historical default), and claude-code is always checked even when no session
// references it, so the Claude path is never skipped. Returns false (failing
// the overall health check) only when a harness with at least one live session
// is unhealthy; the always-on default check is informational.
func checkHarnessHealth(manifests []*manifest.Manifest, homeDir string) bool {
	inUse := map[string]int{}
	for _, m := range manifests {
		h := m.Harness
		if h == "" {
			h = "claude-code"
		}
		inUse[h]++
	}
	// Always verify the default harness, even with zero sessions.
	if _, ok := inUse["claude-code"]; !ok {
		inUse["claude-code"] = 0
	}

	names := make([]string, 0, len(inUse))
	for name := range inUse {
		names = append(names, name)
	}
	sort.Strings(names)

	healthy := true
	for _, name := range names {
		count := inUse[name]
		// A harness with no live sessions is informational only — never fail
		// the overall check on it.
		if !reportHarnessHealth(name, count, homeDir) && count > 0 {
			healthy = false
		}
	}
	return healthy
}

// reportHarnessHealth prints the health of a single harness and returns whether
// it is healthy. count is the number of live sessions using it (0 for the
// always-on default check).
func reportHarnessHealth(name string, count int, homeDir string) bool {
	hh := agent.CheckHarnessHealthAtHome(name, homeDir)

	var label string
	if count > 0 {
		label = fmt.Sprintf("%s (%d session(s))", name, count)
	} else {
		label = fmt.Sprintf("%s (default, no sessions)", name)
	}

	if hh.IsHealthy() {
		detail := fmt.Sprintf("binary '%s' on PATH, auth configured", hh.BinaryName)
		if hh.ConfigDir != "" && hh.ConfigDirFound {
			detail += fmt.Sprintf(", config dir %s", hh.ConfigDir)
		}
		ui.PrintSuccessWithDetail(label, "  • "+detail)
		return true
	}

	// Unhealthy: report which sub-check failed and how to fix it.
	ui.PrintWarning(fmt.Sprintf("Harness '%s' unhealthy", label))
	switch {
	case !hh.Known:
		fmt.Printf("  • Unknown harness — active harnesses: claude-code, codex-cli, agy, opencode-cli, pi-cli; deprecated: gemini-cli\n")
	case !hh.BinaryPresent:
		fmt.Printf("  • CLI binary '%s' not found on PATH — install it or migrate these sessions\n", hh.BinaryName)
	}
	// The unknown-harness line above already explains the failure; the auth
	// hint would just repeat "unknown harness".
	if hh.Known && !hh.AuthConfigured && hh.AuthHint != "" {
		fmt.Printf("  • %s\n", strings.ReplaceAll(hh.AuthHint, "\n", "\n    "))
	}
	if hh.ConfigDir != "" && !hh.ConfigDirFound {
		fmt.Printf("  • Config dir %s not found (harness may be uninitialized)\n", hh.ConfigDir)
	}
	return false
}

// checkTokenRefresherFreshness verifies that ~/go/bin/token-refresher is
// installed and its embedded vcs.revision is an ancestor of origin/main in the
// dear-agent source repository. Mirrors the ce-mb9g pattern used for the agm
// binary itself. Returns true when healthy or indeterminate (fail-open),
// false when a definitive problem is detected.
func checkTokenRefresherFreshness() bool {
	homeDir, _ := os.UserHomeDir()
	binPath := filepath.Join(homeDir, "go", "bin", "token-refresher")

	if _, err := os.Stat(binPath); err != nil {
		ui.PrintWarning("token-refresher not installed")
		fmt.Println("  Fix: make -C ~/src/dear-agent install-token-refresher")
		return false
	}

	rev, err := freshness.ReadBinaryVCSRevision(binPath)
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("Could not read token-refresher build info: %v", err))
		return true // fail open — can't distinguish infra issue from real problem
	}

	if rev == "" {
		ui.PrintWarning("token-refresher has no embedded vcs.revision — rebuild from a tagged commit")
		fmt.Println("  Fix: make -C ~/src/dear-agent install-token-refresher")
		return false
	}

	repoPath, err := freshness.FindDearAgentRepoPath()
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("Could not locate dear-agent source repo: %v — skipping ancestry check", err))
		ui.PrintSuccess(fmt.Sprintf("token-refresher installed (commit %s, ancestry unverified)", rev))
		return true // fail open
	}

	ancestry := freshness.VerifyAncestry(repoPath, rev, "")
	switch {
	case ancestry.OK():
		ui.PrintSuccess(fmt.Sprintf("token-refresher at %s (current, on %s)", ancestry.BinaryCommit, ancestry.TrunkRef))
		return true
	case ancestry.FailLoud():
		ui.PrintWarning("token-refresher is stale — embedded commit is NOT on origin/main")
		fmt.Printf("  %s\n", ancestry.Reason())
		fmt.Printf("  Binary commit: %s\n", ancestry.BinaryCommit)
		if ancestry.TrunkCommit != "" {
			fmt.Printf("  %s: %s\n", ancestry.TrunkRef, ancestry.TrunkCommit)
		}
		fmt.Printf("  Fix: make -C %s install-token-refresher\n", ancestry.RepoPath)
		return false
	default: // AncestryIndeterminate — fail open
		ui.PrintWarning(fmt.Sprintf("Could not verify token-refresher ancestry: %v", ancestry.Reason()))
		return true
	}
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

// formatPIDs renders PIDs space-separated so the printed remedy can be pasted
// straight into a `kill` invocation.
func formatPIDs(pids []int) string {
	parts := make([]string, len(pids))
	for i, p := range pids {
		parts[i] = strconv.Itoa(p)
	}
	return strings.Join(parts, " ")
}
