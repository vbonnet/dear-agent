package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/cli"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/config"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/fuzzy"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/session"
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
)

var rootCmd = &cobra.Command{
	Use:   "csm [session-name]",
	Short: "Claude Session Manager - Smart session resume or create",
	Long: `csm (Claude Session Manager) helps you manage Claude AI sessions
with smart resume/create behavior and interactive prompts.

When no session name is provided:
  • If sessions exist in current directory → Shows interactive picker
  • If no sessions exist → Prompts to create new session

When session name is provided:
  • Exact match found → Resumes that session
  • Fuzzy matches found → Shows "did you mean" prompt
  • No match found → Offers to create new session

Examples:
  csm                    # Smart picker or create
  csm my-session         # Resume or create "my-session"
  csm new                # Create new session (interactive form)
  csm list               # List all sessions
  csm fix                # Fix UUID associations

Global Flags:
  -C, --directory <path>    Working directory (default: current directory)`,
	RunE: runDefaultCommand,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
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
			fmt.Fprintf(os.Stderr, "csm %s (%s)\n", Version, executable)
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
		// This allows multiple CSM commands to run concurrently (e.g., csm list while csm resume)
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
		// No global lock cleanup needed - using fine-grained locks instead
		// (tmux.AcquireTmuxLock and manifest.AcquireLock)
		return nil
	},
}

func init() {
	// Check for CSM_DEBUG environment variable
	// Flag will override this if explicitly set
	debugDefault := false
	if os.Getenv("CSM_DEBUG") == "true" || os.Getenv("CSM_DEBUG") == "1" {
		debugDefault = true
	}

	rootCmd.PersistentFlags().StringVarP(&directory, "directory", "C", "", "Working directory")
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.config/csm/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&sessionsDir, "sessions-dir", "", "sessions directory (default: ~/sessions)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", debugDefault, "enable debug logging (shorthand for --log-level debug, env: CSM_DEBUG)")
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
		configPath = filepath.Join(home, ".config", "csm", "config.yaml")
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

	// Case 1: No session name provided - smart behavior
	if len(args) == 0 {
		return handleNoArgs(matchingSessions, projectDir, uiCfg)
	}

	// Case 2: Session name provided - try to find it
	sessionName := args[0]
	return handleNamedSession(sessionName, manifests, matchingSessions, projectDir, uiCfg)
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

func handleNamedSession(name string, allSessions, matchingSessions []*manifest.Manifest, projectDir string, uiCfg *ui.Config) error {
	// Try exact match first
	for _, m := range allSessions {
		if m.Name == name {
			fmt.Printf("Resuming session: %s\n", m.Name)
			return performResume(m)
		}
	}

	// No exact match - try fuzzy matching
	var candidates []string
	for _, m := range allSessions {
		candidates = append(candidates, m.Name)
	}

	fuzzyMatches := fuzzy.FindSimilar(name, candidates, 0.6)

	if len(fuzzyMatches) > 0 {
		// Found fuzzy matches - show "did you mean"
		choice, err := ui.DidYouMean(name, fuzzyMatches, uiCfg)
		if err != nil {
			return err
		}

		if choice == "" {
			// User chose "create new"
			return runNewSessionFlow(&name, uiCfg)
		}

		// User selected a fuzzy match - resume it
		for _, m := range allSessions {
			if m.Name == choice {
				fmt.Printf("Resuming session: %s\n", m.Name)
				return performResume(m)
			}
		}
	}

	// No matches at all - offer to create new
	fmt.Printf("Session '%s' not found.\n", name)
	confirmed, err := ui.ConfirmCreate(name, projectDir, uiCfg)
	if err != nil {
		return err
	}

	if !confirmed {
		fmt.Println("Cancelled.")
		return nil
	}

	return runNewSessionFlow(&name, uiCfg)
}

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

// ExecuteWithDeps executes the CSM CLI with injected dependencies.
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
