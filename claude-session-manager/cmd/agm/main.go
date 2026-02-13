package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/cli"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/config"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/session"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/telemetry/usage"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"

	// Import workflows to trigger registration
	_ "github.com/vbonnet/ai-tools/claude-session-manager/internal/workflow/deep_research"
)

var (
	cfg               *config.Config
	cfgFile           string
	sessionsDir       string
	logLevel          string
	debugMode         bool
	directory         string
	timeout           time.Duration
	skipHealthCheck   bool
	noColor           bool
	screenReader      bool
	globalHealthCheck *tmux.HealthChecker
	tmuxClient        session.TmuxInterface // Injected dependency for testing
	usageTracker      *usage.Tracker
	commandStartTime  time.Time
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
		// Record command start time for usage tracking
		commandStartTime = time.Now()

		// Load configuration first
		var err error
		cfg, err = loadConfigWithFlags()
		if err != nil {
			return err
		}

		// Print header (version and binary location) for all commands except version
		if cmd.Name() != "version" {
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

		// Initialize health checker
		if cfg.HealthCheck.Enabled && !skipHealthCheck {
			globalHealthCheck = tmux.NewHealthChecker(
				cfg.HealthCheck.CacheDuration,
				cfg.HealthCheck.ProbeTimeout,
			)
		}

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
		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		// Track usage after command completes
		if usageTracker != nil {
			duration := time.Since(commandStartTime).Milliseconds()
			_ = usageTracker.TrackSync(usage.Event{
				Command:  cmd.CommandPath(),
				Args:     args,
				Duration: duration,
				Success:  true,
			})
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
	debugDefault := false
	if os.Getenv("AGM_DEBUG") == "true" || os.Getenv("AGM_DEBUG") == "1" {
		debugDefault = true
	}

	rootCmd.PersistentFlags().StringVarP(&directory, "directory", "C", "", "Working directory")
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.config/agm/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&sessionsDir, "sessions-dir", "", "sessions directory (default: ~/sessions)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", debugDefault, "enable debug logging (shorthand for --log-level debug, env: AGM_DEBUG)")
	rootCmd.PersistentFlags().DurationVar(&timeout, "timeout", 0, "tmux command timeout (overrides config)")
	rootCmd.PersistentFlags().BoolVar(&skipHealthCheck, "skip-health-check", false, "skip health check")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output (WCAG AA compliance)")
	rootCmd.PersistentFlags().BoolVar(&screenReader, "screen-reader", false, "use text symbols instead of Unicode (for screen readers)")
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

	return cfg, nil
}

func runDefaultCommand(cmd *cobra.Command, args []string) error {
	uiCfg := ui.LoadConfig()

	// Get current working directory
	projectDir := cli.GetProjectDirectory()

	// List all sessions
	manifests, err := manifest.List(cfg.SessionsDir)
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
		return runNewSessionFlow(nil, uiCfg)
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

func runNewSessionFlow(suggestedName *string, uiCfg *ui.Config) error {
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
	if err := ExecuteWithDeps(session.NewRealTmux()); err != nil {
		os.Exit(1)
	}
}
