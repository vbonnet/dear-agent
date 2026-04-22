package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Install and verify AGM hooks in Claude Code settings",
	Long: `Set up AGM integration with Claude Code by installing required hooks.

This command is idempotent: it can be run multiple times safely.
It will:
  1. Install hook binaries to ~/.claude/hooks/
  2. Register hooks in ~/.claude/settings.json
  3. Detect and warn about stale hook registrations

This is the recommended way to set up AGM after installation.

Examples:
  agm setup                # Install/update all hooks
  agm setup --check        # Check hook health without installing`,
	RunE: runSetup,
}

var setupCheckOnly bool

func init() {
	setupCmd.Flags().BoolVar(&setupCheckOnly, "check", false, "Check hook health without installing")
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	if setupCheckOnly {
		return checkHookHealth(homeDir)
	}

	// Delegate to install-hooks for actual installation
	fmt.Println("Setting up AGM hooks...")
	fmt.Println()

	// Run the install-hooks logic directly
	if err := runInstallHooks(cmd, args); err != nil {
		return err
	}

	// Additional: check for stale hook registrations
	fmt.Println()
	stale := findStaleHooks(homeDir)
	if len(stale) > 0 {
		fmt.Printf("\n⚠ Found %d stale hook registration(s):\n", len(stale))
		for _, s := range stale {
			fmt.Printf("  • %s (binary missing: %s)\n", s.event, s.command)
		}
		fmt.Println("\nThese hooks are registered but their binaries don't exist.")
		fmt.Println("Remove them from ~/.claude/settings.json or reinstall the binaries.")
	}

	return nil
}

type staleHook struct {
	event   string
	command string
}

// findStaleHooks checks settings.json for hook commands whose binaries don't exist.
func findStaleHooks(homeDir string) []staleHook {
	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil
	}

	hooksMap, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		return nil
	}

	var stale []staleHook
	for event, groups := range hooksMap {
		arr, ok := groups.([]interface{})
		if !ok {
			continue
		}
		for _, group := range arr {
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
				command, ok := hookMap["command"].(string)
				if !ok {
					continue
				}
				// Expand ~ to home dir
				expanded := command
				if len(expanded) > 1 && expanded[0] == '~' {
					expanded = filepath.Join(homeDir, expanded[1:])
				}
				if _, err := os.Stat(expanded); os.IsNotExist(err) {
					stale = append(stale, staleHook{event: event, command: command})
				}
			}
		}
	}

	return stale
}

// checkHookHealth reports on the state of installed hooks without modifying anything.
func checkHookHealth(homeDir string) error {
	fmt.Println("AGM Hook Health Check")
	fmt.Println("=====================")
	fmt.Println()

	// Check hooks directory
	hooksDir := filepath.Join(homeDir, ".claude", "hooks")
	if _, err := os.Stat(hooksDir); os.IsNotExist(err) {
		fmt.Println("❌ Hooks directory does not exist: ~/.claude/hooks/")
		fmt.Println("   Run 'agm setup' to install hooks.")
		return nil
	}
	fmt.Println("✓ Hooks directory exists")

	// Check settings.json
	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		fmt.Println("❌ settings.json does not exist")
		fmt.Println("   Run 'agm setup' to create it.")
		return nil
	}
	fmt.Println("✓ settings.json exists")

	// Check for stale hooks
	stale := findStaleHooks(homeDir)
	if len(stale) > 0 {
		fmt.Printf("⚠ %d stale hook(s) found:\n", len(stale))
		for _, s := range stale {
			fmt.Printf("  • %s: %s\n", s.event, s.command)
		}
	} else {
		fmt.Println("✓ No stale hooks detected")
	}

	fmt.Println()
	fmt.Println("Health check complete.")
	return nil
}
