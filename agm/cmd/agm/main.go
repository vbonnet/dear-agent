package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/vbonnet/dear-agent/agm/internal/cli"
	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/freshness"
	"github.com/vbonnet/dear-agent/agm/internal/harnessexec"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	"github.com/vbonnet/dear-agent/internal/telemetry"
	"github.com/vbonnet/dear-agent/internal/telemetry/usage"
	"github.com/vbonnet/dear-agent/pkg/otelsetup"
	"github.com/vbonnet/dear-agent/pkg/workspace"

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
	forceAgent       bool                  // --agent: force agent output mode ON
	forceNoAgent     bool                  // --no-agent: force agent output mode OFF
	detailedMode     bool                  // --detailed: re-enable IDs/full paths/hints in agent-mode
	outputMode       OutputMode            // resolved once in PersistentPreRunE
	tmuxClient       session.TmuxInterface // Injected dependency for testing
	usageTracker     *usage.Tracker
	commandStartTime time.Time
	auditLogger      *ops.AuditLogger
)

// stdoutIsTTY reports whether stdout is an interactive terminal. It is a package
// variable so tests can override it to exercise the header-gating logic without a
// real TTY.
var stdoutIsTTY = func() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// OutputMode is the unified agent-vs-human output mode. It is resolved exactly
// once in PersistentPreRunE and consulted by the output helpers (header,
// printResult, color/glyphs) so that agent-mode detection lives in one place
// instead of being re-derived from env/TTY checks scattered across the codebase.
type OutputMode int

const (
	// ModeHuman is the interactive terminal experience: text output, version
	// header, color, and success suggestions.
	ModeHuman OutputMode = iota
	// ModeAgent is the machine-facing experience: JSON output, no version
	// header, no color/glyphs, usage silenced, success suggestions dropped.
	ModeAgent
)

// resolveOutputMode applies the agent-mode precedence (highest first):
//
//  1. --agent flag      → force agent mode ON
//  2. --no-agent flag   → force agent mode OFF
//  3. AGM_AGENT=1 env   → agent mode ON
//  4. stdout not a TTY  → agent mode ON (auto)
//  5. default           → human mode (TTY)
func resolveOutputMode(forceAgent, forceNoAgent bool) OutputMode {
	switch {
	case forceAgent:
		return ModeAgent
	case forceNoAgent:
		return ModeHuman
	case os.Getenv("AGM_AGENT") == "1":
		return ModeAgent
	case !stdoutIsTTY():
		return ModeAgent
	default:
		return ModeHuman
	}
}

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

Exit Codes (for agents branching on $?):
  0  success
  1  generic / unknown error
  2  auth failure (unauthenticated / missing API key / permission denied)
  3  bad input (invalid args, validation failure)
  4  state conflict (archived, already-running, confirmation required)
  5  not found (session / resource does not exist)

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

		// Resolve the unified output mode exactly once, here, from flags + env +
		// TTY. Everything downstream (header, JSON routing, color/glyphs) consults
		// outputMode rather than re-deriving agent detection on its own.
		outputMode = resolveOutputMode(forceAgent, forceNoAgent)

		// Agent mode defaults to JSON output. An explicit --output flag always
		// wins (e.g. `--agent -o text` still yields text).
		if outputMode == ModeAgent && !cmd.Flags().Changed("output") {
			outputFormat = "json"
		}

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
		// Suppressed in agent mode to save tokens.
		if cmd.Name() != "version" && cmd.Name() != "status-line" && outputMode == ModeHuman {
			executable, err := os.Executable()
			if err != nil {
				executable = "unknown"
			}
			fmt.Fprintf(os.Stderr, "agm %s (%s)\n", Version, executable)
		}

		// Project UI settings from the same validated config snapshot.
		projectUIConfig(cfg, noColor, screenReader, outputMode)

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

// projectUIConfig installs presentation settings from the already validated
// shared snapshot without rereading configuration or mutating cfg.
func projectUIConfig(cfg *config.Config, noColor, screenReader bool, mode OutputMode) {
	uiCfg := cfg.UISettings
	if noColor {
		uiCfg.UI.NoColor = true
	}
	if screenReader {
		uiCfg.UI.ScreenReader = true
	}
	if mode == ModeAgent {
		uiCfg.UI.NoColor = true
	}
	ui.SetGlobalConfig(&uiCfg)
}

func init() {
	// Silence the cobra Usage block on errors to save ~324 tokens per failed command.
	// SilenceErrors stays false so errors still print.
	rootCmd.SilenceUsage = true

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
	rootCmd.PersistentFlags().BoolVar(&forceAgent, "agent", false, "force agent output mode (JSON, no header/color); overrides TTY auto-detection")
	rootCmd.PersistentFlags().BoolVar(&forceNoAgent, "no-agent", false, "force human output mode (text, header, color); overrides AGM_AGENT and non-TTY auto-detection")
	rootCmd.PersistentFlags().BoolVar(&detailedMode, "detailed", false, "in agent-mode, re-enable IDs, full paths, and verbose hints (inverse of terse default)")
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
	if err := printJSON(info); err != nil {
		return fmt.Errorf("failed to marshal commands to JSON: %w", err)
	}
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
	// Pass an explicit source through unchanged so config.Load can distinguish it
	// from the ordinarily absent canonical default.
	cfg, err := config.Load(cfgFile)
	if err != nil {
		// The strict loader's errors (unknown field, malformed value, extra
		// document, missing explicitly selected file) are all repairable
		// configuration problems, not internal failures. Classify them as bad
		// input (exit 3) so agent callers can branch on $? without parsing
		// stderr, instead of falling through to the generic exit code 1.
		return nil, &exitError{code: ExitBadInput, msg: err.Error()}
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
	if !newCmd.Flags().Changed("sandbox-provider") {
		sandboxProvider = cfg.Sandbox.Provider
	}
	cfg.Sandbox.Provider = sandboxProvider

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
			return nil, fmt.Errorf("setup centralized storage: %w", err)
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
	uiCfg := ui.GetGlobalConfig()

	// Get current working directory
	projectDir := cli.GetProjectDirectory()

	// Get Dolt adapter
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer func() { _ = adapter.Close() }()

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
		return handleNoArgs(cmd.Context(), adapter, matchingSessions, projectDir, uiCfg)
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

func handleNoArgs(ctx context.Context, adapter *dolt.Adapter, matchingSessions []*manifest.Manifest, projectDir string, uiCfg *ui.Config) error {
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
		return performResume(ctx, adapter, matchingSessions[0])
	}

	// Multiple sessions - show picker
	return showSessionPicker(ctx, adapter, matchingSessions, uiCfg)
}

// handleNamedSession removed - 'agm <name>' shortcut no longer supported
// Use 'agm session resume <name>' or 'agm session new <name>' instead

func showSessionPicker(ctx context.Context, adapter *dolt.Adapter, sessions []*manifest.Manifest, uiCfg *ui.Config) error {
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
	return performResume(ctx, adapter, selected.Manifest)
}

// performResume runs the full resume workflow for an already-selected session.
// The bare `agm` default command resolved the session from the current directory
// (or the interactive picker), so we delegate to the same resumeResolvedSession
// helper that backs `agm session resume` rather than reimplementing the workflow.
func performResume(ctx context.Context, adapter *dolt.Adapter, m *manifest.Manifest) error {
	// Reconstruct the manifest path the same way resolveSessionIdentifier does,
	// so resumeResolvedSession can update last-activity and auto-commit.
	manifestPath := filepath.Join(cfg.SessionsDir, m.SessionID, "manifest.yaml")
	return resumeResolvedSession(ctx, adapter, m.SessionID, manifestPath)
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
	return executeWithSignalContext(context.Background(), rootCmd.ExecuteContext, os.Interrupt, syscall.SIGTERM)
}

func executeWithSignalContext(parent context.Context, execute func(context.Context) error, signals ...os.Signal) error {
	ctx, stop := signal.NotifyContext(parent, signals...)
	defer stop()
	return execute(ctx)
}

func main() {
	if len(os.Args) > 1 && harnessexec.IsProtocol(os.Args[1]) {
		if err := harnessexec.Run(os.Args[1], os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "agm private harness executor: %v\n", err)
			os.Exit(1)
		}
		return
	}
	exitCode := run()
	os.Exit(exitCode)
}

func run() int {
	// Bound telemetry teardown so a wedged or partially-implemented collector
	// (e.g. one missing MetricsService) can never delay or hang process exit —
	// telemetry is fail-open, never load-bearing (ce-5zbg).
	const telemetryShutdownTimeout = 5 * time.Second
	shutdown := otelsetup.InitTracer("agm")
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), telemetryShutdownTimeout)
		defer cancel()
		_ = shutdown(ctx) //nolint:errcheck // best-effort flush; exit must not block on telemetry
	}()

	// Metrics counterpart to the tracer above (agent.tasks.*, agent.tokens.*,
	// agent.stall.*). No-op until OTEL_EXPORTER_OTLP_ENDPOINT is set.
	if _, err := telemetry.InitMeter("agm"); err == nil {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), telemetryShutdownTimeout)
			defer cancel()
			_ = telemetry.Shutdown(ctx)
		}()
	}

	// Tmux is AGM's one production local runtime. RealTmux adapts the tmux
	// process once at the composition root while ExecuteWithDeps preserves a
	// deterministic test seam.
	if err := ExecuteWithDeps(session.NewRealTmux()); err != nil {
		// Map the failure onto the exit-code taxonomy so agent consumers can
		// branch on $? (2=auth, 3=bad-input, 4=state-conflict, 5=not-found)
		// without parsing stderr. Unmapped errors fall through to 1.
		return exitCodeFromError(err)
	}
	return ExitOK
}
