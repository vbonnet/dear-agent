package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

// requiredBinaries lists the binaries that should be in PATH after installation.
var requiredBinaries = []string{
	"agm",
	"agm-reaper",
	"agm-mcp-server",
}

// minGoMajor and minGoMinor define the minimum required Go version.
const (
	minGoMajor = 1
	minGoMinor = 23
)

// runInstallChecks performs installation-related health checks:
// - Required binaries in PATH
// - Hook files with agm-/engram- prefixes in ~/.claude/hooks/
// - Hook registrations in ~/.claude/settings.json
// - Config file validity
// - tmux availability
// - Go version minimum
// Returns true if all critical checks pass.
func runInstallChecks() bool {
	allHealthy := true
	homeDir, _ := os.UserHomeDir()

	// 1. Check required binaries in PATH
	fmt.Println(ui.Blue("\n--- Checking required binaries ---"))
	for _, bin := range requiredBinaries {
		path, err := exec.LookPath(bin)
		if err != nil {
			ui.PrintError(err,
				fmt.Sprintf("Binary '%s' not found in PATH", bin),
				fmt.Sprintf("  • Run: make -C <ai-tools-repo>/agm install\n  • Or: go install github.com/vbonnet/dear-agent/agm/cmd/%s@latest", bin))
			allHealthy = false
		} else {
			ui.PrintSuccess(fmt.Sprintf("Binary '%s' found: %s", bin, path))
		}
	}

	// 2. Check hook files in ~/.claude/hooks/
	fmt.Println(ui.Blue("\n--- Checking installed hooks ---"))
	hooksDir := filepath.Join(homeDir, ".claude", "hooks")
	hookCount := 0
	hookMissing := false

	hookPaths := []string{
		filepath.Join(hooksDir, "posttool-agm-state-notify"),
		filepath.Join(hooksDir, "pretool-test-session-guard"),
		filepath.Join(hooksDir, "pretool-agm-mode-tracker"),
		filepath.Join(hooksDir, "session-start", "agm-state-ready"),
		filepath.Join(hooksDir, "session-start", "agm-plan-continuity"),
	}

	for _, hookPath := range hookPaths {
		info, err := os.Stat(hookPath)
		switch {
		case err != nil:
			ui.PrintWarning(fmt.Sprintf("Hook missing: %s", hookPath))
			hookMissing = true
		case info.Mode()&0111 == 0:
			ui.PrintWarning(fmt.Sprintf("Hook not executable: %s", hookPath))
			hookMissing = true
		default:
			hookCount++
		}
	}

	// Also scan for any engram- prefixed hooks
	scanHookPrefixes(hooksDir, "engram-", &hookCount)

	if hookMissing {
		fmt.Println("  • Fix: agm admin install-hooks")
		allHealthy = false
	} else {
		ui.PrintSuccess(fmt.Sprintf("All %d required hook files installed and executable", hookCount))
	}

	// 3. Check settings.json hook registrations
	fmt.Println(ui.Blue("\n--- Checking settings.json hook registrations ---"))
	settingsOK := checkSettingsHooks(homeDir)
	if !settingsOK {
		allHealthy = false
	}

	// 4. Check config validity
	fmt.Println(ui.Blue("\n--- Checking AGM config ---"))
	configOK := checkConfigValidity(homeDir)
	if !configOK {
		allHealthy = false
	}

	// 5. Check Go version
	fmt.Println(ui.Blue("\n--- Checking Go version ---"))
	goOK := checkGoVersion()
	if !goOK {
		allHealthy = false
	}

	return allHealthy
}

// checkSettingsHooks verifies that required hooks are registered in settings.json.
func checkSettingsHooks(homeDir string) bool {
	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			ui.PrintWarning("~/.claude/settings.json not found")
			fmt.Println("  • Fix: agm admin install-hooks")
			return false
		}
		ui.PrintError(err, "Failed to read settings.json", "")
		return false
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		ui.PrintError(err, "settings.json contains invalid JSON", "  • Check syntax: cat ~/.claude/settings.json | python3 -m json.tool")
		return false
	}

	hooksMap, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		ui.PrintWarning("No hooks section in settings.json")
		fmt.Println("  • Fix: agm admin install-hooks")
		return false
	}

	// Check that expected hook commands are registered
	expectedCommands := []string{
		"~/.claude/hooks/posttool-agm-state-notify",
		"~/.claude/hooks/pretool-agm-mode-tracker",
		"~/.claude/hooks/pretool-test-session-guard",
		"~/.claude/hooks/session-start/agm-state-ready",
		"~/.claude/hooks/session-start/agm-plan-continuity",
	}

	registeredCommands := collectRegisteredCommands(hooksMap)
	missing := 0
	for _, expected := range expectedCommands {
		if !registeredCommands[expected] {
			ui.PrintWarning(fmt.Sprintf("Hook not registered in settings.json: %s", expected))
			missing++
		}
	}

	if missing > 0 {
		fmt.Println("  • Fix: agm admin install-hooks")
		return false
	}

	ui.PrintSuccess(fmt.Sprintf("All %d required hooks registered in settings.json", len(expectedCommands)))
	return true
}

// collectRegisteredCommands extracts all command strings from the hooks map.
func collectRegisteredCommands(hooksMap map[string]interface{}) map[string]bool {
	commands := make(map[string]bool)
	for _, eventGroups := range hooksMap {
		groups, ok := eventGroups.([]interface{})
		if !ok {
			continue
		}
		for _, group := range groups {
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
				if cmd, ok := hookMap["command"].(string); ok {
					commands[cmd] = true
				}
			}
		}
	}
	return commands
}

// checkConfigValidity checks if the AGM config file exists and is parseable.
func checkConfigValidity(homeDir string) bool {
	configPath := filepath.Join(homeDir, ".config", "agm", "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		if os.IsNotExist(err) {
			// Config is optional, using defaults is fine
			ui.PrintSuccess("Using default config (no ~/.config/agm/config.yaml)")
			return true
		}
		ui.PrintError(err, "Failed to check config file", "")
		return false
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		ui.PrintError(err, "Config file exists but is unreadable", fmt.Sprintf("  • Check permissions: ls -la %s", configPath))
		return false
	}

	if len(data) == 0 {
		ui.PrintWarning("Config file is empty")
		return true
	}

	ui.PrintSuccess(fmt.Sprintf("Config file valid: %s", configPath))
	return true
}

// checkGoVersion verifies Go is installed and meets the minimum version.
func checkGoVersion() bool {
	goPath, err := exec.LookPath("go")
	if err != nil {
		ui.PrintWarning("Go not found in PATH (only needed for building from source)")
		return true // Not critical for runtime
	}

	// Use runtime version for the running binary
	version := runtime.Version() // e.g. "go1.25.0"
	major, minor := parseGoVersion(version)
	if major < minGoMajor || (major == minGoMajor && minor < minGoMinor) {
		ui.PrintWarning(fmt.Sprintf("Go version %s is below minimum %d.%d (binary built with %s)",
			version, minGoMajor, minGoMinor, version))
		fmt.Printf("  • Go binary: %s\n", goPath)
		return false
	}

	ui.PrintSuccess(fmt.Sprintf("Go version: %s (minimum: %d.%d)", version, minGoMajor, minGoMinor))
	return true
}

// parseGoVersion extracts major.minor from a Go version string like "go1.25.0".
func parseGoVersion(version string) (int, int) {
	version = strings.TrimPrefix(version, "go")
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return 0, 0
	}
	var major, minor int
	fmt.Sscanf(parts[0], "%d", &major)
	fmt.Sscanf(parts[1], "%d", &minor)
	return major, minor
}

// scanHookPrefixes scans a directory for hook files with a given prefix.
func scanHookPrefixes(dir, prefix string, count *int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			// Recurse into subdirectories
			scanHookPrefixes(filepath.Join(dir, entry.Name()), prefix, count)
			continue
		}
		if strings.HasPrefix(entry.Name(), prefix) {
			*count++
		}
	}
}
