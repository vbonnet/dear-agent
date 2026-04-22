package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/cmd/engram/internal/cli"
	"github.com/vbonnet/dear-agent/internal/telemetry/usage"
)

// Version information - set via ldflags at build time
var (
	version = "dev"     // Version (e.g., "v0.1.0-prototype")
	commit  = "unknown" // Git commit SHA
	date    = "unknown" // Build date
	builtBy = "unknown" // Builder (e.g., "goreleaser", "manual")
)

// Usage tracker for CLI analytics
var usageTracker *usage.Tracker
var commandStartTime time.Time

// Global flags
var (
	workspaceFlag    string
	listCommandsJSON bool
)

var rootCmd = &cobra.Command{
	Use:   "engram",
	Short: "Engram - Persistent learning for AI coding agents",
	Long: `Engram is a platform that enables AI coding agents to learn from experience
and improve over time through memory traces (engrams) stored as markdown files.

The platform supports multiple AI agents (Claude Code, Cursor, Windsurf) and provides
plugin-based extensibility for custom workflows.

USAGE
  engram <command> [flags]

CORE COMMANDS
  init       - Initialize Engram workspace
  doctor     - Run health checks and diagnostics
  index      - Manage engram indexes
  retrieve   - Search and retrieve relevant engrams
  plugin     - Manage plugins
  tokens     - Estimate token counts for engram files

DOCUMENTATION COMMANDS
  backfill-spec         - Generate SPEC.md from codebase
  backfill-architecture - Generate ARCHITECTURE.md from codebase
  backfill-adrs         - Generate ADRs from git history
  review-spec           - Validate SPEC.md quality (LLM-as-judge)
  review-architecture   - Validate ARCHITECTURE.md quality (LLM-as-judge)
  review-adr            - Validate ADR file quality (LLM-as-judge)

EXAMPLES
  # Initialize workspace
  $ engram init

  # Run health checks
  $ engram doctor

  # Rebuild engram indexes
  $ engram index rebuild

  # Search for engrams
  $ engram retrieve "error handling patterns"

  # List installed plugins
  $ engram plugin list

  # Estimate tokens for engram files
  $ engram tokens estimate "**/*.ai.md"
  $ engram tokens estimate file.md --json

  # Backfill documentation for existing projects
  $ engram backfill-spec --project-dir ~/my-project
  $ engram review-spec --file ~/my-project/SPEC.md

  # Generate shell completion script
  $ engram completion bash > /etc/bash_completion.d/engram

LEARN MORE
  Use 'engram <command> --help' for detailed help on each command.
  Documentation: https://github.com/vbonnet/dear-agent/engram`,
	Version: getVersionInfo(),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Handle --list-commands-json early (before normal execution)
		if listCommandsJSON {
			return printCommandsJSON(cmd.Root())
		}

		// Record command start time for duration tracking
		commandStartTime = time.Now()

		// Print header to stderr for all commands except version
		if cmd.Name() != "version" {
			executable, err := os.Executable()
			if err != nil {
				executable = "unknown"
			}
			fmt.Fprintf(os.Stderr, "engram %s (%s)\n", version, executable)
		}
		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		// Track usage after command completes (sync to ensure write before exit)
		if usageTracker != nil {
			duration := time.Since(commandStartTime).Milliseconds()
			_ = usageTracker.TrackSync(usage.Event{
				Command:  cmd.CommandPath(),
				Args:     args,
				Duration: duration,
				Success:  true,
			})
			// Ignore errors - don't want tracking failures to break CLI
		}
		return nil
	},
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Initialize usage tracker (silent fail if it errors)
	var err error
	usageTracker, err = usage.New("")
	if err != nil {
		// Don't fail if tracker can't be initialized
		// CLI should work even if usage tracking fails
		usageTracker = nil
	}

	// Global flags
	rootCmd.PersistentFlags().StringVar(&workspaceFlag, "workspace", "", "explicit workspace name (overrides auto-detection)")
	rootCmd.PersistentFlags().BoolVar(&listCommandsJSON, "list-commands-json", false, "output all commands as JSON (agent discovery API)")
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

// getVersionInfo returns formatted version information
func getVersionInfo() string {
	executable, err := os.Executable()
	if err != nil {
		executable = "unknown"
	}
	return fmt.Sprintf("%s\nBinary: %s\nCommit: %s\nBuilt: %s by %s\nGo: %s %s/%s",
		version, executable, commit, date, builtBy, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

// completionCmd represents the completion command
var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion script",
	Long: `Generate auto-completion script for your shell.

INSTALLATION

Bash:
  $ source <(engram completion bash)

  # To load completions for every session, add to ~/.bashrc:
  $ echo 'source <(engram completion bash)' >> ~/.bashrc

Zsh:
  $ source <(engram completion zsh)

  # To load completions for every session, add to ~/.zshrc:
  $ echo 'source <(engram completion zsh)' >> ~/.zshrc

Fish:
  $ engram completion fish | source

  # To load completions for every session:
  $ engram completion fish > ~/.config/fish/completions/engram.fish

PowerShell:
  PS> engram completion powershell | Out-String | Invoke-Expression

  # To load completions for every session, add to your PowerShell profile:
  PS> engram completion powershell | Out-File -Append $PROFILE

EXAMPLES
  # Generate bash completion
  $ engram completion bash > /etc/bash_completion.d/engram

  # Test zsh completion (without installing)
  $ source <(engram completion zsh)

  # Install for fish
  $ engram completion fish > ~/.config/fish/completions/engram.fish
`,
	ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
	Args:      cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return cmd.Root().GenBashCompletion(os.Stdout)
		case "zsh":
			return cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			return cmd.Root().GenFishCompletion(os.Stdout, true)
		case "powershell":
			return cmd.Root().GenPowerShellCompletion(os.Stdout)
		default:
			return cli.InvalidInputError("shell", args[0], "bash|zsh|fish|powershell")
		}
	},
}

func init() {
	// Add completion command to root
	rootCmd.AddCommand(completionCmd)
}
