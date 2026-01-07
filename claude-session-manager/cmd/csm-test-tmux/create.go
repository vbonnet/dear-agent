package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/testutil/output"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/testutil/session"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/testutil/tmux"
)

var (
	createWorkingDir     string
	createSessionsDir    string
	createStartupTimeout int
	createAddDirs        []string
	createSkipPermissions bool
)

var createCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new CSM test session in isolated tmux",
	Long: `Create a new test session with isolated tmux and CSM state.

The session name must contain only alphanumeric characters, hyphens, and underscores.

Examples:
  # Create session with default settings
  csm-test-tmux create my-test

  # Create with custom sessions directory
  csm-test-tmux create my-test --sessions-dir /tmp/my-tests

  # Create with longer startup timeout
  csm-test-tmux create my-test --startup-timeout 60

  # Add trusted directories to avoid permission prompts
  csm-test-tmux create my-test --add-dir /tmp/test-project

  # Skip all permission prompts (use with caution)
  csm-test-tmux create my-test --skip-permissions

  # Get JSON output for AI agents
  csm-test-tmux create my-test --format json`,
	Args: cobra.ExactArgs(1),
	RunE: runCreate,
}

func init() {
	createCmd.Flags().StringVar(
		&createWorkingDir,
		"working-dir",
		"",
		"Working directory for the session (default: current directory)",
	)
	createCmd.Flags().StringVar(
		&createSessionsDir,
		"sessions-dir",
		"",
		"Directory for CSM session state (default: /tmp/csm-test-<name>)",
	)
	createCmd.Flags().IntVar(
		&createStartupTimeout,
		"startup-timeout",
		30,
		"Timeout in seconds for Claude to start",
	)
	createCmd.Flags().StringSliceVar(
		&createAddDirs,
		"add-dir",
		[]string{},
		"Additional directories to trust (passed to claude --add-dir)",
	)
	createCmd.Flags().BoolVar(
		&createSkipPermissions,
		"skip-permissions",
		false,
		"Skip permission prompts (passes --dangerously-skip-permissions to claude)",
	)

	rootCmd.AddCommand(createCmd)
}

func runCreate(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Set defaults
	if createWorkingDir == "" {
		var err error
		createWorkingDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	if createSessionsDir == "" {
		createSessionsDir = fmt.Sprintf("/tmp/csm-test-%s", name)
	}

	// Create session manager
	tmuxClient := tmux.New()
	mgr := session.New(tmuxClient)

	// Create session
	opts := session.CreateOptions{
		Name:            name,
		WorkingDir:      createWorkingDir,
		SessionsDir:     createSessionsDir,
		StartupTimeout:  time.Duration(createStartupTimeout) * time.Second,
		AdditionalDirs:  createAddDirs,
		SkipPermissions: createSkipPermissions,
	}

	sess, err := mgr.Create(opts)
	if err != nil {
		return formatError(err)
	}

	// Format output
	return printOutput(sess)
}

// formatError formats an error using the configured formatter
func formatError(err error) error {
	formatter := output.Format(formatFlag)
	formatted, fmtErr := formatter.FormatError(err)
	if fmtErr != nil {
		// Fallback to plain error if formatting fails
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	fmt.Fprint(os.Stderr, formatted)
	return err
}

// printOutput prints data using the configured formatter
func printOutput(data interface{}) error {
	formatter := output.Format(formatFlag)
	formatted, err := formatter.Format(data)
	if err != nil {
		return fmt.Errorf("failed to format output: %w", err)
	}
	fmt.Println(formatted)
	return nil
}
