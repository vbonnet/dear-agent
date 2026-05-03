package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/statusline"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

var (
	statusLineFormat  string
	statusLineSession string
	statusLineJSON    bool
)

var statusLineCmd = &cobra.Command{
	Use:   "status-line",
	Short: "Generate tmux status line for current session",
	Long: `Generate tmux status line with agent icon, state, context usage, and git status.

This command is designed to be called from tmux status-right or status-left
configuration to display rich session information in the status bar.

The status line includes:
  • Agent icon (🤖 Claude, ✨ Gemini, 🧠 GPT, 💻 OpenCode)
  • Session state with color coding (DONE|WORKING|USER_PROMPT|COMPACTING|OFFLINE)
  • Context usage percentage with warning colors (green < 70%, yellow < 85%, red >= 85%)
  • Git branch name and uncommitted file count
  • Session name

Template variables available:
  {{.AgentIcon}}       - Icon for agent type (🤖✨🧠💻)
  {{.AgentType}}       - Agent name (claude, gemini, gpt, opencode)
  {{.State}}           - Session state (DONE, WORKING, etc.)
  {{.StateColor}}      - Tmux color code for state (green, blue, yellow, etc.)
  {{.ContextPercent}}  - Context usage percentage (0-100)
  {{.ContextColor}}    - Tmux color code for context (green, yellow, red)
  {{.Branch}}          - Git branch name
  {{.Uncommitted}}     - Number of uncommitted files
  {{.SessionName}}     - Session name
  {{.Workspace}}       - Workspace name

Output formats:
  • template (default) - Render using template from --format flag or config
  • json - Output raw JSON data for custom processing

Examples:
  # Auto-detect current tmux session and use default template
  agm session status-line

  # Use custom template
  agm session status-line -f '{{.AgentIcon}} {{.State}} | {{.ContextPercent}}%'

  # Explicit session name
  agm session status-line -s my-session

  # JSON output for custom parsing
  agm session status-line --json

  # Tmux configuration example (add to ~/.tmux.conf)
  # Use #{session_name} so each session gets its own status data
  set -g status-right '#(agm session status-line -s "#{session_name}")'
  set -g status-interval 10`,
	RunE: runStatusLine,
}

func init() {
	statusLineCmd.Flags().StringVarP(
		&statusLineFormat,
		"format",
		"f",
		"",
		"Template string (overrides config default)",
	)

	statusLineCmd.Flags().StringVarP(
		&statusLineSession,
		"session",
		"s",
		"",
		"Session name (auto-detected from tmux if not specified)",
	)

	statusLineCmd.Flags().BoolVar(
		&statusLineJSON,
		"json",
		false,
		"Output JSON instead of formatted template",
	)

	sessionCmd.AddCommand(statusLineCmd)
}

func runStatusLine(cmd *cobra.Command, args []string) error {
	// Determine session name (explicit flag or auto-detect from tmux)
	sessionName := statusLineSession
	if sessionName == "" {
		var err error
		sessionName, err = autoDetectTmuxSession()
		if err != nil {
			// Not in tmux - show minimal message instead of error
			fmt.Print("[Not in tmux]")
			return nil //nolint:nilerr // intentional: caller signals via separate bool/optional
		}
	}

	// Find manifest by session name
	m, err := findManifestBySession(sessionName)
	if err != nil {
		// Session not found - this is a regular tmux session, not AGM-managed
		// Show minimal indicator instead of erroring (prevents tmux status line from disappearing)
		fmt.Printf("[%s]", sessionName)
		return nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	// Collect status line data
	data, err := session.CollectStatusLineData(sessionName, m)
	if err != nil {
		// Collection failed - show session name with error indicator
		fmt.Printf("[%s ⚠️]", sessionName)
		return nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	// Output based on format flag
	if statusLineJSON {
		return outputStatusLineJSON(data)
	}

	return outputTemplate(data)
}

// autoDetectTmuxSession detects the current tmux session name
func autoDetectTmuxSession() (string, error) {
	// Use the tmux package's GetCurrentSessionName function
	sessionName, err := tmux.GetCurrentSessionName()
	if err != nil {
		return "", fmt.Errorf("not in tmux session or failed to detect: %w", err)
	}

	return sessionName, nil
}

// findManifestBySession finds a manifest by tmux session name or AGM session name
func findManifestBySession(sessionName string) (*manifest.Manifest, error) {
	// Get Dolt adapter (AGM has migrated from YAML to Dolt database)
	adapter, err := getStorage()
	if err != nil {
		// Dolt not available - gracefully degrade (don't error)
		return nil, fmt.Errorf("dolt not available: %w", err)
	}
	defer adapter.Close()

	// List all sessions and find the one with matching tmux session name
	// This is not the most efficient but works for status line use case
	sessions, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	// Search for session by tmux session name or AGM session name.
	// Prefer entries with a Claude UUID (duplicate entries without UUID
	// can be created by session restarts or hook re-registration).
	var bestMatch *manifest.Manifest
	for _, m := range sessions {
		if m.Tmux.SessionName == sessionName || m.Name == sessionName {
			if bestMatch == nil || (bestMatch.Claude.UUID == "" && m.Claude.UUID != "") {
				bestMatch = m
			}
		}
	}
	if bestMatch != nil {
		return bestMatch, nil
	}

	return nil, fmt.Errorf("session not found: %s", sessionName)
}

// outputStatusLineJSON outputs status line data as JSON
func outputStatusLineJSON(data *session.StatusLineData) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}
	return nil
}

// outputTemplate renders and outputs the template
func outputTemplate(data *session.StatusLineData) error {
	// Determine template to use (flag > config > default)
	templateStr := statusLineFormat
	if templateStr == "" {
		templateStr = cfg.StatusLine.DefaultFormat
	}
	if templateStr == "" {
		// Fallback to default template from formatter package
		templateStr = statusline.DefaultTemplate()
	}

	// Create formatter and render
	formatter, err := statusline.NewFormatter(templateStr)
	if err != nil {
		return fmt.Errorf("failed to create formatter: %w", err)
	}

	output, err := formatter.Format(data)
	if err != nil {
		return fmt.Errorf("failed to format status line: %w", err)
	}

	// Print to stdout (for tmux to capture)
	fmt.Print(output)
	return nil
}
