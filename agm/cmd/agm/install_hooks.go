package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

//go:embed hooks/*
var hooksFS embed.FS

var installHooksCmd = &cobra.Command{
	Use:   "install-hooks",
	Short: "Install Claude Code hooks for state tracking",
	Long: `Install Claude Code hooks that notify AGM of session state transitions.

This command copies hook scripts to ~/.claude/hooks/ and registers them in
~/.claude/settings.json so Claude Code invokes them automatically.

Hooks installed:
  • posttool-agm-state-notify         - Set state to THINKING after tool use
  • session-start/agm-state-ready     - Set state to READY on session start
  • session-start/agm-plan-continuity - Link execution sessions to planning parents
  • agm-pretool-test-session-guard        - Block test-* sessions without --test flag
  • pretool-agm-mode-tracker          - Track permission mode changes for persistence

These hooks enable accurate state detection with <1% false positive rate,
replacing the fragile tmux pane parsing method (37.5% false positive rate).

The agm-plan-continuity hook automatically detects when Claude Code creates
execution sessions via "Clear Context and Execute Plan" and links them to
their parent planning sessions for proper session resumption.

Examples:
  # Install hooks (copy files + register in settings.json)
  agm admin install-hooks

  # Verify hooks are installed
  ls -la ~/.claude/hooks/`,
	RunE: runInstallHooks,
}

func init() {
	adminCmd.AddCommand(installHooksCmd)
}

// hookRegistration describes how a hook should be registered in settings.json
type hookRegistration struct {
	Event   string // PreToolUse, PostToolUse, SessionStart, Stop
	Command string // command path (using ~ for home dir)
	Timeout int    // timeout in seconds (0 = no timeout field)
	Matcher string // optional matcher (empty = no matcher)
}

func runInstallHooks(cmd *cobra.Command, args []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	hooksDir := filepath.Join(homeDir, ".claude", "hooks")

	// Create hooks directory if it doesn't exist
	if err := os.MkdirAll(hooksDir, 0o700); err != nil {
		return fmt.Errorf("failed to create hooks directory: %w", err)
	}

	// Create session-start subdirectory
	sessionStartDir := filepath.Join(hooksDir, "session-start")
	if err := os.MkdirAll(sessionStartDir, 0o700); err != nil {
		return fmt.Errorf("failed to create session-start directory: %w", err)
	}

	// Hook files to install
	hooks := map[string]string{
		"hooks/posttool-agm-state-notify":         filepath.Join(hooksDir, "posttool-agm-state-notify"),
		"hooks/session-start-agm-state-ready":     filepath.Join(sessionStartDir, "agm-state-ready"),
		"hooks/session-start-agm-plan-continuity": filepath.Join(sessionStartDir, "agm-plan-continuity"),
		"hooks/agm-pretool-test-session-guard":    filepath.Join(hooksDir, "agm-pretool-test-session-guard"),
		"hooks/pretool-agm-mode-tracker":          filepath.Join(hooksDir, "pretool-agm-mode-tracker"),
		"hooks/stop-agm-resource-cleanup":         filepath.Join(hooksDir, "stop-agm-resource-cleanup"),
	}

	installed := 0
	for srcPath, destPath := range hooks {
		// Read hook content from embedded filesystem
		content, err := hooksFS.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("failed to read embedded hook %s: %w", srcPath, err)
		}

		// Conflict detection: warn if file exists and is not from AGM
		if existing, err := os.ReadFile(destPath); err == nil {
			// Check if existing file differs (not our hook)
			if string(existing) != string(content) {
				fmt.Printf("⚠ Conflict: %s exists with different content, overwriting\n", destPath)
			}
		}

		// Write hook file (must be executable)
		err = os.WriteFile(destPath, content, 0o700) //#nosec G306 -- executable hook
		if err != nil {
			return fmt.Errorf("failed to write hook %s: %w", destPath, err)
		}

		fmt.Printf("✓ Installed: %s\n", destPath)
		installed++
	}

	fmt.Printf("\nSuccessfully installed %d hook files to %s\n", installed, hooksDir)

	// Register hooks in settings.json
	registrations := []hookRegistration{
		{
			Event:   "PostToolUse",
			Command: "~/.claude/hooks/posttool-agm-state-notify",
			Timeout: 5,
		},
		{
			Event:   "PreToolUse",
			Command: "~/.claude/hooks/pretool-agm-mode-tracker",
			Timeout: 5,
		},
		{
			Event:   "PreToolUse",
			Command: "~/.claude/hooks/agm-pretool-test-session-guard",
			Timeout: 5,
		},
		{
			Event:   "SessionStart",
			Command: "~/.claude/hooks/session-start/agm-state-ready",
			Timeout: 5,
		},
		{
			Event:   "SessionStart",
			Command: "~/.claude/hooks/session-start/agm-plan-continuity",
			Timeout: 10,
		},
		{
			Event:   "Stop",
			Command: "~/.claude/hooks/stop-agm-resource-cleanup",
			Timeout: 30,
		},
	}

	registered, err := registerHooksInSettings(homeDir, registrations)
	if err != nil {
		return fmt.Errorf("hook files installed but failed to register in settings.json: %w", err)
	}

	if registered > 0 {
		fmt.Printf("\n✓ Registered %d new hooks in ~/.claude/settings.json\n", registered)
	} else {
		fmt.Println("\nAll hooks already registered in ~/.claude/settings.json")
	}

	fmt.Println("\nHooks will be triggered automatically by Claude Code.")
	fmt.Println("State updates will be sent to AGM via 'agm session state set' command.")

	return nil
}

// registerHooksInSettings adds hook entries to ~/.claude/settings.json if not already present.
// Returns the number of newly registered hooks.
func registerHooksInSettings(homeDir string, registrations []hookRegistration) (int, error) {
	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")

	// Read existing settings
	var settings map[string]interface{}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			settings = make(map[string]interface{})
		} else {
			return 0, fmt.Errorf("failed to read settings.json: %w", err)
		}
	} else {
		if err := json.Unmarshal(data, &settings); err != nil {
			return 0, fmt.Errorf("failed to parse settings.json: %w", err)
		}
	}

	// Get or create hooks map
	hooksMap, _ := settings["hooks"].(map[string]interface{})
	if hooksMap == nil {
		hooksMap = make(map[string]interface{})
	}

	registered := 0
	for _, reg := range registrations {
		if addHookRegistration(hooksMap, reg) {
			registered++
		}
	}

	if registered > 0 {
		settings["hooks"] = hooksMap

		output, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return 0, fmt.Errorf("failed to marshal settings: %w", err)
		}
		// Append newline for POSIX compliance
		output = append(output, '\n')

		if err := os.WriteFile(settingsPath, output, 0600); err != nil {
			return 0, fmt.Errorf("failed to write settings.json: %w", err)
		}
	}

	return registered, nil
}

// addHookRegistration adds a single hook to the appropriate event array in the hooks map.
// Returns true if the hook was added (not already present).
func addHookRegistration(hooksMap map[string]interface{}, reg hookRegistration) bool {
	// Get or create the event array
	var eventGroups []interface{}
	if existing, ok := hooksMap[reg.Event]; ok {
		if arr, ok := existing.([]interface{}); ok {
			eventGroups = arr
		}
	}

	// Check if this hook command is already registered in any group
	for _, group := range eventGroups {
		groupMap, ok := group.(map[string]interface{})
		if !ok {
			continue
		}
		hooksList, ok := groupMap["hooks"].([]interface{})
		if !ok {
			continue
		}
		for _, h := range hooksList {
			hookMap, ok := h.(map[string]interface{})
			if !ok {
				continue
			}
			if cmd, ok := hookMap["command"].(string); ok && cmd == reg.Command {
				return false // Already registered
			}
		}
	}

	// Build the hook entry
	hookEntry := map[string]interface{}{
		"command": reg.Command,
		"type":    "command",
	}
	if reg.Timeout > 0 {
		hookEntry["timeout"] = reg.Timeout
	}

	// Build the group entry
	groupEntry := map[string]interface{}{
		"hooks": []interface{}{hookEntry},
	}
	if reg.Matcher != "" {
		groupEntry["matcher"] = reg.Matcher
	}

	eventGroups = append(eventGroups, groupEntry)
	hooksMap[reg.Event] = eventGroups

	return true
}
