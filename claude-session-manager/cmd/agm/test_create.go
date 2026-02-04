package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"time"

	"github.com/charmbracelet/huh/spinner"
	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var (
	createJSON            bool
	createWorkingDir      string
	createSessionsDir     string
	createStartupTimeout  int
	createAddDirs         []string
	createSkipPermissions bool
	createPrompt          string
	createPromptFile      string
)

// sessionNameRegex validates session names (alphanumeric, hyphens, underscores only)
var sessionNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

var testCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new CSM test session in isolated tmux",
	Long: `Create a new test session with isolated tmux and CSM state.

The session name must contain only alphanumeric characters, hyphens, and underscores.

Test sessions are completely isolated from production CSM sessions:
  • Tmux session: csm-test-<name> (separate from production sessions)
  • Sessions directory: /tmp/csm-test-<name> (not ~/.claude-sessions)
  • Working directory: configurable (default: current directory)

Examples:
  # Create session with default settings
  csm test create my-test

  # Create with custom sessions directory
  csm test create my-test --sessions-dir /tmp/my-tests

  # Create with longer startup timeout
  csm test create my-test --startup-timeout 60

  # Add trusted directories to avoid permission prompts
  csm test create my-test --add-dir /tmp/test-project

  # Skip all permission prompts (use with caution)
  csm test create my-test --skip-permissions

  # Get JSON output for automation
  csm test create my-test --json`,
	Args: cobra.ExactArgs(1),
	RunE: runTestCreate,
}

func init() {
	testCreateCmd.Flags().BoolVar(
		&createJSON,
		"json",
		false,
		"Output as JSON for automation",
	)
	testCreateCmd.Flags().StringVar(
		&createWorkingDir,
		"working-dir",
		"",
		"Working directory for the session (default: current directory)",
	)
	testCreateCmd.Flags().StringVar(
		&createSessionsDir,
		"sessions-dir",
		"",
		"Directory for CSM session state (default: /tmp/csm-test-<name>)",
	)
	testCreateCmd.Flags().IntVar(
		&createStartupTimeout,
		"startup-timeout",
		30,
		"Timeout in seconds for Claude to start",
	)
	testCreateCmd.Flags().StringSliceVar(
		&createAddDirs,
		"add-dir",
		[]string{},
		"Additional directories to trust (passed to claude --add-dir)",
	)
	testCreateCmd.Flags().BoolVar(
		&createSkipPermissions,
		"skip-permissions",
		false,
		"Skip permission prompts (passes --dangerously-skip-permissions to claude)",
	)
	testCreateCmd.Flags().StringVar(
		&createPrompt,
		"prompt",
		"",
		"Prompt to send after session initialization",
	)
	testCreateCmd.Flags().StringVar(
		&createPromptFile,
		"prompt-file",
		"",
		"File containing prompt to send",
	)
	testCreateCmd.MarkFlagsMutuallyExclusive("prompt", "prompt-file")

	testCmd.AddCommand(testCreateCmd)
}

// TestSession represents a test session (for JSON output)
type TestSession struct {
	Name          string    `json:"name"`
	TmuxSession   string    `json:"tmux_session"`
	SessionsDir   string    `json:"sessions_dir"`
	WorkingDir    string    `json:"working_dir"`
	CreatedAt     time.Time `json:"created_at"`
	StartupTimeMs int64     `json:"startup_time_ms"`
}

func runTestCreate(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Validate session name
	if !sessionNameRegex.MatchString(name) {
		return fmt.Errorf("session name '%s' contains invalid characters.\nUse only letters, numbers, hyphens, and underscores.\nExample: my-test-1, test_session, mytest123", name)
	}

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

	tmuxName := fmt.Sprintf("csm-test-%s", name)

	// Check for collision
	exists, err := tmux.HasSession(tmuxName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if exists {
		return fmt.Errorf("session '%s' already exists.\n\nSuggestions:\n  • Cleanup existing: csm test cleanup %s\n  • Use different name: csm test create %s-2", name, name, name)
	}

	// Create session with or without spinner
	var sess *TestSession
	var createErr error

	if !createJSON {
		// Human mode - use spinner
		spinErr := spinner.New().
			Title(fmt.Sprintf("Creating test session '%s'...", name)).
			Accessible(true).
			Action(func() {
				sess, createErr = createTestSession(name, tmuxName, createWorkingDir, createSessionsDir, createStartupTimeout)
			}).
			Run()
		if spinErr != nil {
			return fmt.Errorf("spinner error: %w", spinErr)
		}

		// Ensure clean line after spinner
		fmt.Println()

		if createErr != nil {
			return createErr
		}

		// Print human-readable output
		ui.PrintSuccess(fmt.Sprintf("Test session created: %s", name))
		fmt.Printf("  Tmux: %s\n", tmuxName)
		fmt.Printf("  Sessions: %s\n", createSessionsDir)
		fmt.Printf("  Startup: %.2fs\n", float64(sess.StartupTimeMs)/1000.0)
	} else {
		// Automation mode - direct call
		sess, createErr = createTestSession(name, tmuxName, createWorkingDir, createSessionsDir, createStartupTimeout)
		if createErr != nil {
			return createErr
		}

		// Print JSON output
		jsonBytes, err := json.MarshalIndent(sess, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(jsonBytes))
	}

	return nil
}

// createTestSession creates a test session with Claude started
func createTestSession(name, tmuxName, workingDir, sessionsDir string, timeoutSec int) (*TestSession, error) {
	startTime := time.Now()

	// Track cleanup state
	var cleanupDone bool
	defer func() {
		if !cleanupDone {
			// Best-effort cleanup on failure
			_ = exec.Command("tmux", "kill-session", "-t", tmuxName).Run()
			_ = os.RemoveAll(sessionsDir)
		}
	}()

	// Step 1: Create sessions directory
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create sessions directory: %w\n\nSuggestions:\n  • Check write permissions for: %s\n  • Verify parent directory exists", err, sessionsDir)
	}

	// Step 2: Create tmux session
	if err := tmux.NewSession(tmuxName, workingDir); err != nil {
		return nil, fmt.Errorf("failed to create tmux session: %w\n\nSuggestions:\n  • Check if tmux is installed: tmux -V\n  • Verify working directory exists: %s", err, workingDir)
	}

	// Step 3: Build Claude command with flags
	claudeCmd := "claude"
	for _, dir := range createAddDirs {
		claudeCmd += fmt.Sprintf(" --add-dir '%s'", dir)
	}
	if createSkipPermissions {
		claudeCmd += " --dangerously-skip-permissions"
	}

	// Step 4: Start Claude in tmux session
	sendCmd := exec.Command("tmux", "send-keys", "-t", tmuxName, claudeCmd, "C-m")
	if err := sendCmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to start Claude: %w\n\nSuggestions:\n  • Check if claude is installed: which claude\n  • Verify claude is in PATH", err)
	}

	// Step 5: Wait for Claude prompt
	timeout := time.Duration(timeoutSec) * time.Second
	if err := tmux.WaitForPromptSimple(tmuxName, timeout); err != nil {
		return nil, fmt.Errorf("Claude startup timeout after %ds: %w\n\nSuggestions:\n  • Increase timeout: --startup-timeout %d\n  • Check Claude is working: claude --version\n  • View session output: tmux attach -t %s", timeoutSec, err, timeoutSec+30, tmuxName)
	}

	// Step 6: Send prompt if provided via --prompt or --prompt-file flags
	if createPrompt != "" {
		if err := tmux.SendPromptLiteral(tmuxName, createPrompt); err != nil {
			// Non-fatal - log warning and continue
			fmt.Printf("Warning: failed to send prompt: %v\n", err)
			fmt.Println("  • You can manually enter the prompt in the session")
		}
	} else if createPromptFile != "" {
		if err := tmux.SendPromptFromFile(tmuxName, createPromptFile); err != nil {
			// Non-fatal - log warning and continue
			fmt.Printf("Warning: failed to send prompt from file: %v\n", err)
			fmt.Println("  • You can manually enter the prompt in the session")
		}
	}

	startupTime := time.Since(startTime)
	cleanupDone = true // Disable cleanup defer

	return &TestSession{
		Name:          name,
		TmuxSession:   tmuxName,
		SessionsDir:   sessionsDir,
		WorkingDir:    workingDir,
		CreatedAt:     time.Now(),
		StartupTimeMs: startupTime.Milliseconds(),
	}, nil
}
