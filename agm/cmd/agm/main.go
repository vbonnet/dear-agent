package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/backend"
	"github.com/vbonnet/dear-agent/agm/internal/cli"
	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/freshness"
	"github.com/vbonnet/dear-agent/agm/internal/manager"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/telemetry/usage"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	"github.com/vbonnet/dear-agent/pkg/otelsetup"
	"github.com/vbonnet/dear-agent/pkg/workspace"

	// Import backends to trigger registration
	_ "github.com/vbonnet/dear-agent/agm/internal/backend"
	// Import manager backends to trigger registration
	_ "github.com/vbonnet/dear-agent/agm/internal/manager/tmuxbackend"
	// Import workflows to trigger registration
	_ "github.com/vbonnet/dear-agent/agm/internal/workflow/deepresearch"
)

var (
	cfg              *config.Config
	cfgFile          string
	sessionsDir      string
	logLevel         string
	debugMode        bool
	directory        string
	timeout          time.Duration
	skipHealthCheck  bool
	noColor          bool
	screenReader     bool
	workspaceFlag    string
	listCommandsJSON bool
	outputFormat     string                // "text" (default), "json"
	fieldsFlag       []string              // field mask for JSON output
	tmuxClient       session.TmuxInterface // Injected dependency for testing
	managerBackend   manager.Backend       // New abstraction layer (nil = legacy path)
	usageTracker     *usage.Tracker
	commandStartTime time.Time
	auditLogger      *ops.AuditLogger
)

var rootCmd = &cobra.Command{
	Use:   "agm",
	Short: "Agent Gateway Manager - Multi-AI session management",
	Long: `agm (Agent Gateway Manager) helps you manage AI agent sessions
(Claude, Gemini, GPT) with explicit session commands.

When no arguments are provided:
  • If sessions exist in current directory → Shows interactive picker
  • If no sessions exist → Prompts to create new session

Session operations require explicit subcommands:
  • Use 'agm session resume <name>' to resume a session
  • Use 'agm session new <name>' to create a new session
  • Use 'agm session list' to list all sessions

Examples:
  agm                            # Smart picker or create (interactive)
  agm session resume my-session  # Resume existing session
  agm session new my-session     # Create new session
  agm session list               # List all sessions
  agm session archive my-session # Archive a session
  agm admin fix-uuid             # Fix UUID associations

Global Flags:
  -C, --directory <path>    Working directory (default: current directory)`,
	Args: cobra.ArbitraryArgs, // Allow any arguments to reach runDefaultCommand
	RunE: runDefaultCommand,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Handle --list-commands-json early (before normal execution)
		if listCommandsJSON {
			return printCommandsJSON(cmd.Root())
		}

		// Record command start time for usage tracking
		commandStartTime = time.Now()

		// Initialize audit logger (best-effort, don't fail command on audit errors)
		if al, err := ops.NewAuditLogger(""); err == nil {
			auditLogger = al
		}

		// Load configuration first
		var err error
		cfg, err = loadConfigWithFlags()
		if err != nil {
			return err
		}

		// Print header (version and binary location) for all commands except version and status-line
		// status-line is excluded because it's designed for machine parsing (tmux status bar)
		if cmd.Name() != "version" && cmd.Name() != "status-line" {
			executable, err := os.Executable()
			if err != nil {
				executable = "unknown"
			}
			fmt.Fprintf(os.Stderr, "agm %s (%s)\n", Version, executable)
		}

		// Load UI config and apply flag overrides
		uiCfg := ui.LoadConfig()
		if noColor {
			uiCfg.UI.NoColor = true
		}
		if screenReader {
			uiCfg.UI.ScreenReader = true
		}
		ui.SetGlobalConfig(uiCfg)

		// Set global timeout for tmux commands
		if cfg.Timeout.Enabled {
			tmux.SetTimeout(cfg.Timeout.TmuxCommands)
		}

		// NOTE: Global command lock removed in favor of fine-grained locks:
		// - Tmux operations use tmux.AcquireTmuxLock() (in internal/tmux/lock.go)
		// - Manifest operations use manifest.AcquireLock() (in internal/manifest/lock.go)
		// This allows multiple AGM commands to run concurrently (e.g., agm session list while agm my-session)
		// while still preventing race conditions in tmux server updates and manifest modifications.

		// Resolve working directory from -C flag
		if directory != "" {
			absPath, err := filepath.Abs(directory)
			if err != nil {
				return fmt.Errorf("failed to resolve directory: %w", err)
			}
			cli.SetProjectDirectory(absPath)
		} else {
			// Use current working directory if -C not specified
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}
			cli.SetProjectDirectory(cwd)
		}

		// Check binary freshness for mutation commands
		cmdPath := cmd.CommandPath()
		if strings.HasPrefix(cmdPath, "agm send") ||
			strings.HasPrefix(cmdPath, "agm session new") ||
			strings.HasPrefix(cmdPath, "agm session resume") {
			if repoPath, err := freshness.FindRepoPath(); err == nil {
				result := freshness.Check(repoPath, GitCommit)
				if result.Stale {
					fmt.Fprintf(os.Stderr, "\n⚠ WARNING: agm binary is stale\n")
					fmt.Fprintf(os.Stderr, "  Binary commit: %s\n", result.BinaryCommit)
					fmt.Fprintf(os.Stderr, "  Repo HEAD:     %s\n", result.RepoHEAD)
					fmt.Fprintf(os.Stderr, "  Run: make -C %s install\n\n", result.RepoPath)
				}
			}
		}

		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		duration := time.Since(commandStartTime).Milliseconds()

		// Track usage after command completes
		if usageTracker != nil {
			if err := usageTracker.TrackSync(usage.Event{
				Command:  cmd.CommandPath(),
				Args:     args,
				Duration: duration,
				Success:  true,
			}); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to track usage: %v\n", err)
			}
		}

		// Write audit trail entry (skip if command already logged its own enriched event)
		auditHandledMu.Lock()
		handled := auditHandled
		auditHandledMu.Unlock()

		if !handled && auditLogger != nil {
			event := ops.AuditEvent{
				Command:    cmd.CommandPath(),
				User:       os.Getenv("AGM_SESSION_NAME"),
				Result:     "success",
				DurationMs: duration,
			}
			if err := auditLogger.Log(event); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to write audit log: %v\n", err)
			}
		}

		// No global lock cleanup needed - using fine-grained locks instead
		// (tmux.AcquireTmuxLock and manifest.AcquireLock)
		return nil
	},
}

func init() {
	// Initialize usage tracker
	var err error
	usageTracker, err = usage.New("")
	if err != nil {
		usageTracker = nil // Don't fail if tracker can't be initialized
	}

	// Check for AGM_DEBUG environment variable
	// Flag will override this if explicitly set
	debugDefault := os.Getenv("AGM_DEBUG") == "true" || os.Getenv("AGM_DEBUG") == "1"

	rootCmd.PersistentFlags().StringVarP(&directory, "directory", "C", "", "Working directory")
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.config/agm/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&sessionsDir, "sessions-dir", "", "sessions directory (default: ~/.claude/sessions)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", debugDefault, "enable debug logging (shorthand for --log-level debug, env: AGM_DEBUG)")
	rootCmd.PersistentFlags().DurationVar(&timeout, "timeout", 0, "tmux command timeout (overrides config)")
	rootCmd.PersistentFlags().BoolVar(&skipHealthCheck, "skip-health-check", false, "skip health check")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output (WCAG AA compliance)")
	rootCmd.PersistentFlags().BoolVar(&screenReader, "screen-reader", false, "use text symbols instead of Unicode (for screen readers)")
	rootCmd.PersistentFlags().StringVar(&workspaceFlag, "workspace", "", "explicit workspace name")
	rootCmd.PersistentFlags().BoolVar(&listCommandsJSON, "list-commands-json", false, "output all commands as JSON (agent discovery API)")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "text", "output format: text, json")
	rootCmd.PersistentFlags().StringSliceVar(&fieldsFlag, "fields", nil, "comma-separated field mask for JSON output (e.g., --fields id,name,status)")
}

// CommandInfo represents a command for JSON output
type CommandInfo struct {
	Name        string        `json:"name"`
	Use         string        `json:"use"`
	Short       string        `json:"short"`
	Long        string        `json:"long,omitempty"`
	Subcommands []CommandInfo `json:"subcommands,omitempty"`
}

// printCommandsJSON outputs all commands in JSON format for agent discovery
func printCommandsJSON(cmd *cobra.Command) error {
	info := buildCommandInfo(cmd)
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal commands to JSON: %w", err)
	}
	fmt.Println(string(data))
	os.Exit(0)
	return nil
}

// buildCommandInfo recursively builds command info structure
func buildCommandInfo(cmd *cobra.Command) CommandInfo {
	info := CommandInfo{
		Name:  cmd.Name(),
		Use:   cmd.Use,
		Short: cmd.Short,
		Long:  cmd.Long,
	}

	// Add subcommands recursively
	for _, subCmd := range cmd.Commands() {
		// Skip hidden commands
		if !subCmd.IsAvailableCommand() || subCmd.Hidden {
			continue
		}
		info.Subcommands = append(info.Subcommands, buildCommandInfo(subCmd))
	}

	return info
}

func loadConfigWithFlags() (*config.Config, error) {
	// Load config file or defaults
	configPath := cfgFile
	if configPath == "" {
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".config", "agm", "config.yaml")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}

	// Override with flags
	if sessionsDir != "" {
		cfg.SessionsDir = sessionsDir
	}
	// --debug flag takes precedence over --log-level
	if debugMode {
		cfg.LogLevel = "debug"
	} else if logLevel != "" {
		cfg.LogLevel = logLevel
	}
	if timeout > 0 {
		cfg.Timeout.TmuxCommands = timeout
	}
	if skipHealthCheck {
		cfg.HealthCheck.Enabled = false
	}

	// Workspace detection: Auto-detect workspace from current directory
	// to enable workspace-specific session storage.
	//
	// Detection is skipped if:
	// - SessionsDir is explicitly set via flag/config/env (user override)
	// - Workspace is already set in config (explicit workspace selection)
	//
	// Detection flow (see detectWorkspace for details):
	// 1. Load workspace config from ~/.agm/config.yaml (or custom path)
	// 2. Run 6-priority detection algorithm (flag > env > auto-detect > default > interactive > error)
	// 3. If successful: Set cfg.Workspace and override SessionsDir to {workspace_root}/.agm/sessions
	// 4. If failed: Fall back to default ~/sessions (backward compatible)
	if sessionsDir == "" && cfg.Workspace == "" {
		detectWorkspace(cfg, workspaceFlag)
	}

	// Centralized storage support: Create symlink if centralized mode is enabled
	// This ensures transparent redirection from ~/.agm to centralized storage location
	// (e.g., ~/src/ws/oss/repos/engram-research/.agm)
	if cfg.Storage.Mode == "centralized" {
		if err := config.EnsureSymlinkBootstrap(cfg); err != nil {
			// Log warning but don't fail - allow AGM to continue in degraded mode
			fmt.Fprintf(os.Stderr, "Warning: Failed to setup centralized storage symlink: %v\n", err)
			fmt.Fprintf(os.Stderr, "Continuing with dotfile mode. Run 'agm storage verify' for details.\n")
		}
	}

	return cfg, nil
}

// detectWorkspace attempts to auto-detect workspace from current directory.
//
// This function implements workspace detection with robust error handling
// for common edge cases:
//
// Edge cases handled:
//  1. Missing workspace config file (~/.agm/config.yaml)
//     → Falls back to default ~/sessions (backward compatible)
//  2. Invalid or corrupted workspace config
//     → Logs warning and falls back to default
//  3. Current directory outside any workspace
//     → Falls back to default workspace or ~/sessions
//  4. Multiple nested workspaces (ambiguous path)
//     → Uses first match (engram detector walks up from deepest)
//  5. Disabled workspaces in config
//     → Skipped during detection (engram detector filters enabled only)
//  6. Non-existent current directory
//     → Falls back to default (filepath.Abs handles gracefully)
//
// Detection algorithm (from engram/core/pkg/workspace):
//
//	Priority 1: Explicit --workspace flag (highest priority)
//	Priority 2: WORKSPACE environment variable
//	Priority 3: Auto-detect from PWD (walk up directory tree)
//	Priority 4: Default workspace from config
//	Priority 5: Interactive prompt (disabled in AGM - non-interactive)
//	Priority 6: Error (falls back to ~/sessions in AGM)
//
// On success:
//   - Sets cfg.Workspace to detected workspace name
//   - Overrides cfg.SessionsDir to {workspace_root}/.agm/sessions
//
// On failure:
//   - Leaves cfg.Workspace empty
//   - Leaves cfg.SessionsDir at default ~/.claude/sessions
//   - No error returned (graceful degradation)
func detectWorkspace(cfg *config.Config, workspaceFlag string) {
	// Determine workspace config path (default: ~/.agm/config.yaml)
	workspaceConfigPath := cfg.WorkspaceConfigPath
	if workspaceConfigPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			// Can't determine home directory - skip workspace detection
			if debugMode {
				fmt.Fprintf(os.Stderr, "Warning: Failed to get home directory for workspace config: %v\n", err)
			}
			return
		}
		workspaceConfigPath = filepath.Join(home, ".agm", "config.yaml")
	}

	// Check if workspace config exists
	if _, err := os.Stat(workspaceConfigPath); os.IsNotExist(err) {
		// Config file doesn't exist - this is OK, use default sessions dir
		if debugMode {
			fmt.Fprintf(os.Stderr, "Info: No workspace config found at %s, using default sessions dir\n", workspaceConfigPath)
		}
		return
	}

	// Create workspace detector (non-interactive mode)
	detector, err := workspace.NewDetectorWithInteractive(workspaceConfigPath, false)
	if err != nil {
		// Config exists but is invalid/corrupted
		fmt.Fprintf(os.Stderr, "Warning: Failed to load workspace config from %s: %v\n", workspaceConfigPath, err)
		fmt.Fprintf(os.Stderr, "         Using default sessions directory. Fix config or remove it to clear this warning.\n")
		return
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		// Can't determine current directory - unusual but handle gracefully
		if debugMode {
			fmt.Fprintf(os.Stderr, "Warning: Failed to get current directory: %v\n", err)
		}
		return
	}

	// Attempt workspace detection
	ws, err := detector.Detect(cwd, workspaceFlag)
	if err != nil {
		// Detection failed - could be:
		// - No matching workspace for current directory
		// - Invalid workspace specified in --workspace flag
		// - No workspaces configured/enabled
		if workspaceFlag != "" {
			// Explicit flag provided but failed - this is an error worth showing
			fmt.Fprintf(os.Stderr, "Warning: Workspace '%s' not found or disabled: %v\n", workspaceFlag, err)
			fmt.Fprintf(os.Stderr, "         Using default sessions directory.\n")
		} else if debugMode {
			// Auto-detection failed silently (expected if not in workspace)
			fmt.Fprintf(os.Stderr, "Info: No workspace detected for %s: %v\n", cwd, err)
		}
		return
	}

	// Detection successful - configure workspace-specific sessions dir
	cfg.Workspace = ws.Name
	cfg.SessionsDir = workspaceSessionsDir(ws)

	if debugMode {
		fmt.Fprintf(os.Stderr, "Info: Detected workspace '%s' at %s\n", ws.Name, ws.Root)
		fmt.Fprintf(os.Stderr, "      Using sessions directory: %s\n", cfg.SessionsDir)
	}
}

// workspaceSessionsDir returns the sessions directory for a detected workspace.
// If OutputDir is explicitly configured (differs from Root), uses {OutputDir}/sessions.
// Otherwise uses the standard convention {Root}/.agm/sessions.
func workspaceSessionsDir(ws *workspace.Workspace) string {
	if ws.OutputDir != "" && ws.OutputDir != ws.Root {
		return filepath.Join(ws.OutputDir, "sessions")
	}
	return filepath.Join(ws.Root, ".agm", "sessions")
}

func runDefaultCommand(cmd *cobra.Command, args []string) error {
	uiCfg := ui.LoadConfig()

	// Get current working directory
	projectDir := cli.GetProjectDirectory()

	// Get Dolt adapter
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer adapter.Close()

	// List all sessions
	manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	// Filter to sessions matching current directory
	var matchingSessions []*manifest.Manifest
	for _, m := range manifests {
		absProjectPath, _ := filepath.Abs(m.Context.Project)
		if absProjectPath == projectDir {
			matchingSessions = append(matchingSessions, m)
		}
	}

	// Case 1: No arguments provided - smart picker behavior
	if len(args) == 0 {
		return handleNoArgs(matchingSessions, projectDir, uiCfg)
	}

	// Case 2: Arguments provided - this is an error (removed 'agm <name>' shortcut)
	sessionName := args[0]
	fmt.Fprintf(os.Stderr, "Error: Unknown command or argument: %q\n\n", sessionName)
	fmt.Fprintf(os.Stderr, "The 'agm <session-name>' shortcut has been removed to prevent command name collisions.\n\n")
	fmt.Fprintf(os.Stderr, "To resume a session, use:\n")
	fmt.Fprintf(os.Stderr, "  agm session resume %s\n\n", sessionName)
	fmt.Fprintf(os.Stderr, "To create a new session, use:\n")
	fmt.Fprintf(os.Stderr, "  agm session new %s\n\n", sessionName)
	fmt.Fprintf(os.Stderr, "To list all sessions, use:\n")
	fmt.Fprintf(os.Stderr, "  agm session list\n\n")
	fmt.Fprintf(os.Stderr, "Run 'agm --help' for more information.\n")
	return fmt.Errorf("unknown command: %q", sessionName)
}

func handleNoArgs(matchingSessions []*manifest.Manifest, projectDir string, uiCfg *ui.Config) error {
	if len(matchingSessions) == 0 {
		// No sessions - offer to create new
		fmt.Println("No sessions found in current directory.")
		confirmed, err := ui.ConfirmCreate("", projectDir, uiCfg)
		if err != nil {
			return err
		}

		if !confirmed {
			fmt.Println("Cancelled.")
			return nil
		}

		// Launch interactive form for new session
		return runNewSessionFlow(nil)
	}

	if len(matchingSessions) == 1 {
		// Single session - resume it directly
		fmt.Printf("Resuming session: %s\n", matchingSessions[0].Name)
		return performResume(matchingSessions[0])
	}

	// Multiple sessions - show picker
	return showSessionPicker(matchingSessions, uiCfg)
}

// handleNamedSession removed - 'agm <name>' shortcut no longer supported
// Use 'agm session resume <name>' or 'agm session new <name>' instead

func showSessionPicker(sessions []*manifest.Manifest, uiCfg *ui.Config) error {
	// Convert to UI sessions with status
	uiSessions := make([]*ui.Session, len(sessions))

	// Batch compute statuses for efficiency (use injected tmuxClient)
	statuses := session.ComputeStatusBatch(sessions, tmuxClient)

	for i, m := range sessions {
		uiSessions[i] = &ui.Session{
			Manifest:  m,
			Status:    statuses[m.Name],
			UpdatedAt: m.UpdatedAt,
		}
	}

	// Show interactive picker
	selected, err := ui.SessionPicker(uiSessions, uiCfg)
	if err != nil {
		return err
	}

	fmt.Printf("Resuming session: %s\n", selected.Name)
	return performResume(selected.Manifest)
}

func performResume(m *manifest.Manifest) error {
	// TODO: Implement actual resume logic
	// This will integrate with tmux and claude CLI
	fmt.Printf("  Project: %s\n", m.Context.Project)
	fmt.Printf("  Status: %s\n", session.ComputeStatus(m, tmuxClient))
	if m.Claude.UUID != "" {
		fmt.Printf("  UUID: %s\n", m.Claude.UUID)
	}
	fmt.Println("\n[Resume logic placeholder - full implementation in next iteration]")
	return nil
}

func runNewSessionFlow(suggestedName *string) error {
	// TODO: Implement new session flow
	// This will show the interactive form we built
	if suggestedName != nil {
		fmt.Printf("Creating new session: %s\n", *suggestedName)
	} else {
		fmt.Println("Creating new session...")
	}
	fmt.Println("\n[New session flow placeholder - full implementation in next iteration]")
	return nil
}

// ExecuteWithDeps executes the AGM CLI with injected dependencies.
// This function is used for testing to inject mock implementations.
//
// Parameters:
//
//	tmux - TmuxInterface implementation (use session.NewRealTmux() for production)
//
// Returns:
//
//	error - Command execution error (nil on success)
func ExecuteWithDeps(tmux session.TmuxInterface) error {
	tmuxClient = tmux
	return rootCmd.Execute()
}

func main() {
	exitCode := run()
	os.Exit(exitCode)
}

func run() int {
	shutdown := otelsetup.InitTracer("agm")
	defer shutdown(context.Background()) //nolint:errcheck

	// Use backend adapter to support multiple backends
	// The backend is selected via AGM_SESSION_BACKEND env var (defaults to tmux)
	adapter, err := backend.GetDefaultBackendAdapter()
	if err != nil {
		// Fallback to tmux if backend initialization fails
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize backend, falling back to tmux: %v\n", err)
		adapter = backend.NewBackendAdapter(backend.NewTmuxBackend())
	}

	// Initialize the new manager backend abstraction layer
	mgr, mgrErr := manager.GetDefault("")
	if mgrErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: manager backend unavailable: %v\n", mgrErr)
	}
	managerBackend = mgr

	if err := ExecuteWithDeps(adapter); err != nil {
		return 1
	}
	return 0
}
