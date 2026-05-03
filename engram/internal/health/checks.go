package health

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RunAllChecks executes all health checks and returns results
func (hc *HealthChecker) RunAllChecks() ([]CheckResult, error) {
	checks := []CheckResult{}

	// Core infrastructure checks (1-6)
	checks = append(checks, hc.checkWorkspaceExists())
	checks = append(checks, hc.checkConfigValid())
	checks = append(checks, hc.checkLogsDirectoryWritable())
	checks = append(checks, hc.checkLogSize())
	checks = append(checks, hc.checkCoreEngramsAccessible())
	checks = append(checks, hc.checkCacheDirectoryWritable())

	// Dependency checks (7-9)
	checks = append(checks, hc.checkYQAvailable())
	checks = append(checks, hc.checkJQAvailable())
	checks = append(checks, hc.checkPythonAvailable())

	// Hook checks (10-15)
	checks = append(checks, hc.checkHooksConfigured())
	checks = append(checks, hc.checkHookScriptsExecutable())
	checks = append(checks, hc.checkHookExtensionMatch())
	checks = append(checks, hc.checkHookPathsValid())
	checks = append(checks, hc.checkHookTimeouts())
	checks = append(checks, hc.checkHookConfigSyntax())

	// Marketplace checks (15-16)
	checks = append(checks, hc.checkMarketplaceConfigValid())
	checks = append(checks, hc.checkEnabledPluginMarketplaces())

	// Permission check (17)
	checks = append(checks, hc.checkFilePermissions())

	// Plugin health checks (13)
	pluginResults := hc.checkPluginHealth()
	checks = append(checks, pluginResults...)

	return checks, nil
}

// Check 1: Workspace exists
func (hc *HealthChecker) checkWorkspaceExists() CheckResult {
	info, err := os.Stat(hc.workspace)
	if err != nil {
		if os.IsNotExist(err) {
			return CheckResult{
				Name:     "workspace_exists",
				Category: "core",
				Status:   "error",
				Message:  "Workspace not initialized: ~/.engram/ missing",
				Fix:      "engram init",
			}
		}
		return CheckResult{
			Name:     "workspace_exists",
			Category: "core",
			Status:   "error",
			Message:  fmt.Sprintf("Cannot access workspace: %v", err),
		}
	}

	if !info.IsDir() {
		return CheckResult{
			Name:     "workspace_exists",
			Category: "core",
			Status:   "error",
			Message:  "~/.engram/ exists but is not a directory",
		}
	}

	return CheckResult{
		Name:     "workspace_exists",
		Category: "core",
		Status:   "ok",
	}
}

// Check 2: Config file valid
func (hc *HealthChecker) checkConfigValid() CheckResult {
	configPath := filepath.Join(hc.workspace, "user", "config.yaml")

	// Check if file exists
	info, err := os.Stat(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return CheckResult{
				Name:     "config_valid",
				Category: "core",
				Status:   "warning",
				Message:  "Config file missing: ~/.engram/user/config.yaml (using defaults)",
			}
		}
		return CheckResult{
			Name:     "config_valid",
			Category: "core",
			Status:   "error",
			Message:  fmt.Sprintf("Cannot access config: %v", err),
		}
	}

	if info.IsDir() {
		return CheckResult{
			Name:     "config_valid",
			Category: "core",
			Status:   "error",
			Message:  "Config path is a directory, not a file",
		}
	}

	// Read file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return CheckResult{
			Name:     "config_valid",
			Category: "core",
			Status:   "error",
			Message:  fmt.Sprintf("Cannot read config: %v", err),
		}
	}

	// Basic YAML validation (check for common syntax errors)
	content := string(data)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		// Check for unmatched brackets
		if strings.Count(line, "[") != strings.Count(line, "]") {
			return CheckResult{
				Name:     "config_valid",
				Category: "core",
				Status:   "error",
				Message:  fmt.Sprintf("YAML syntax error at line %d", i+1),
				Fix:      "Edit ~/.engram/user/config.yaml manually",
			}
		}
	}

	return CheckResult{
		Name:     "config_valid",
		Category: "core",
		Status:   "ok",
	}
}

// Check 3: Logs directory writable
func (hc *HealthChecker) checkLogsDirectoryWritable() CheckResult {
	logsDir := filepath.Join(hc.workspace, "logs")

	// Check if directory exists
	info, err := os.Stat(logsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return CheckResult{
				Name:     "logs_directory_writable",
				Category: "core",
				Status:   "warning",
				Message:  "Logs directory missing: ~/.engram/logs/",
				Fix:      "engram doctor --fix",
			}
		}
		return CheckResult{
			Name:     "logs_directory_writable",
			Category: "core",
			Status:   "error",
			Message:  fmt.Sprintf("Cannot access logs directory: %v", err),
		}
	}

	if !info.IsDir() {
		return CheckResult{
			Name:     "logs_directory_writable",
			Category: "core",
			Status:   "error",
			Message:  "Logs path exists but is not a directory",
		}
	}

	// Test write permission
	testFile := filepath.Join(logsDir, ".healthcheck")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return CheckResult{
			Name:     "logs_directory_writable",
			Category: "core",
			Status:   "error",
			Message:  "Logs directory not writable",
		}
	}
	os.Remove(testFile) // Clean up

	return CheckResult{
		Name:     "logs_directory_writable",
		Category: "core",
		Status:   "ok",
	}
}

// Check 4: Log size reasonable
func (hc *HealthChecker) checkLogSize() CheckResult {
	logsDir := filepath.Join(hc.workspace, "logs")
	const maxSize = 100 * 1024 * 1024 // 100MB

	entries, err := os.ReadDir(logsDir)
	if err != nil {
		// If logs directory doesn't exist, it's handled by check 3
		return CheckResult{
			Name:     "logs_size",
			Category: "core",
			Status:   "ok",
		}
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.Size() > maxSize {
			sizeMB := info.Size() / (1024 * 1024)
			return CheckResult{
				Name:     "logs_size",
				Category: "core",
				Status:   "warning",
				Message:  fmt.Sprintf("%s is %dMB (threshold: 100MB)", entry.Name(), sizeMB),
				Fix:      "engram doctor --fix",
			}
		}
	}

	return CheckResult{
		Name:     "logs_size",
		Category: "core",
		Status:   "ok",
	}
}

// Check 5: Core engrams accessible
func (hc *HealthChecker) checkCoreEngramsAccessible() CheckResult {
	coreSymlink := filepath.Join(hc.workspace, "core")
	engramsDir := filepath.Join(coreSymlink, "engrams")

	// First, check if ~/.engram/core symlink exists
	symlinkInfo, symlinkErr := os.Lstat(coreSymlink)
	if symlinkErr != nil {
		if os.IsNotExist(symlinkErr) {
			return CheckResult{
				Name:     "core_engrams_accessible",
				Category: "core",
				Status:   "error",
				Message:  "Core symlink missing: ~/.engram/core does not exist",
				Fix:      "ln -s /path/to/engram/repo ~/.engram/core",
			}
		}
		return CheckResult{
			Name:     "core_engrams_accessible",
			Category: "core",
			Status:   "error",
			Message:  fmt.Sprintf("Cannot access core symlink: %v", symlinkErr),
		}
	}

	// Verify it's actually a symlink
	if symlinkInfo.Mode()&os.ModeSymlink == 0 {
		return CheckResult{
			Name:     "core_engrams_accessible",
			Category: "core",
			Status:   "error",
			Message:  "~/.engram/core exists but is not a symlink",
			Fix:      "Remove ~/.engram/core and create symlink: ln -s /path/to/engram/repo ~/.engram/core",
		}
	}

	// Check if symlink target exists (detect broken symlink)
	targetPath, readErr := os.Readlink(coreSymlink)
	if readErr != nil {
		return CheckResult{
			Name:     "core_engrams_accessible",
			Category: "core",
			Status:   "error",
			Message:  fmt.Sprintf("Cannot read core symlink: %v", readErr),
		}
	}

	// Expand relative symlink paths
	if !filepath.IsAbs(targetPath) {
		targetPath = filepath.Join(hc.workspace, targetPath)
	}

	// Check if target exists
	if _, targetErr := os.Stat(targetPath); targetErr != nil {
		if os.IsNotExist(targetErr) {
			return CheckResult{
				Name:     "core_engrams_accessible",
				Category: "core",
				Status:   "error",
				Message:  fmt.Sprintf("Broken symlink: ~/.engram/core -> %s (target does not exist)", targetPath),
				Fix:      "Update symlink to point to correct engram repository: ln -sf /path/to/engram/repo ~/.engram/core",
			}
		}
		return CheckResult{
			Name:     "core_engrams_accessible",
			Category: "core",
			Status:   "error",
			Message:  fmt.Sprintf("Cannot access symlink target: %v", targetErr),
		}
	}

	// Now check engrams directory
	info, err := os.Stat(engramsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return CheckResult{
				Name:     "core_engrams_accessible",
				Category: "core",
				Status:   "warning",
				Message:  fmt.Sprintf("Core engrams directory missing: %s/engrams/ (symlink target may be incorrect)", targetPath),
				Fix:      "Verify symlink points to engram repository root containing core/engrams/",
			}
		}
		return CheckResult{
			Name:     "core_engrams_accessible",
			Category: "core",
			Status:   "warning",
			Message:  fmt.Sprintf("Cannot access core engrams: %v", err),
		}
	}

	if !info.IsDir() {
		return CheckResult{
			Name:     "core_engrams_accessible",
			Category: "core",
			Status:   "warning",
			Message:  "Core engrams path is not a directory",
		}
	}

	// Count .ai.md files
	count := 0
	filepath.Walk(engramsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".ai.md") {
			count++
		}
		return nil
	})

	if count == 0 {
		return CheckResult{
			Name:     "core_engrams_accessible",
			Category: "core",
			Status:   "warning",
			Message:  "No .ai.md files found in core engrams directory",
		}
	}

	return CheckResult{
		Name:     "core_engrams_accessible",
		Category: "core",
		Status:   "ok",
		Message:  fmt.Sprintf("%d engrams found (symlink: %s)", count, targetPath),
	}
}

// Check 6: Cache directory writable
func (hc *HealthChecker) checkCacheDirectoryWritable() CheckResult {
	cacheDir := filepath.Join(hc.workspace, "cache")

	// Check if directory exists
	info, err := os.Stat(cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return CheckResult{
				Name:     "cache_directory_writable",
				Category: "core",
				Status:   "warning",
				Message:  "Cache directory missing: ~/.engram/cache/",
				Fix:      "engram doctor --fix",
			}
		}
		return CheckResult{
			Name:     "cache_directory_writable",
			Category: "core",
			Status:   "error",
			Message:  fmt.Sprintf("Cannot access cache directory: %v", err),
		}
	}

	if !info.IsDir() {
		return CheckResult{
			Name:     "cache_directory_writable",
			Category: "core",
			Status:   "error",
			Message:  "Cache path exists but is not a directory",
		}
	}

	// Test write permission
	testFile := filepath.Join(cacheDir, ".healthcheck")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return CheckResult{
			Name:     "cache_directory_writable",
			Category: "core",
			Status:   "error",
			Message:  "Cache directory not writable",
		}
	}
	os.Remove(testFile) // Clean up

	return CheckResult{
		Name:     "cache_directory_writable",
		Category: "core",
		Status:   "ok",
	}
}

// Check 7: yq available
func (hc *HealthChecker) checkYQAvailable() CheckResult {
	path, err := exec.LookPath("yq")
	if err != nil {
		return CheckResult{
			Name:     "yq_available",
			Category: "dependency",
			Status:   "info",
			Message:  "yq not available (config parsing uses fallback)",
			Fix:      getInstallCommand("yq"),
		}
	}

	// Get version
	cmd := exec.Command("yq", "--version")
	output, err := cmd.Output()
	if err != nil {
		return CheckResult{
			Name:     "yq_available",
			Category: "dependency",
			Status:   "info",
			Message:  fmt.Sprintf("yq available at %s", path),
		}
	}

	version := strings.TrimSpace(string(output))
	return CheckResult{
		Name:     "yq_available",
		Category: "dependency",
		Status:   "info",
		Message:  fmt.Sprintf("yq available (%s)", version),
	}
}

// Check 8: jq available
func (hc *HealthChecker) checkJQAvailable() CheckResult {
	path, err := exec.LookPath("jq")
	if err != nil {
		return CheckResult{
			Name:     "jq_available",
			Category: "dependency",
			Status:   "info",
			Message:  "jq not available (JSONL parsing unavailable)",
			Fix:      getInstallCommand("jq"),
		}
	}

	// Get version
	cmd := exec.Command("jq", "--version")
	output, err := cmd.Output()
	if err != nil {
		return CheckResult{
			Name:     "jq_available",
			Category: "dependency",
			Status:   "info",
			Message:  fmt.Sprintf("jq available at %s", path),
		}
	}

	version := strings.TrimSpace(string(output))
	return CheckResult{
		Name:     "jq_available",
		Category: "dependency",
		Status:   "info",
		Message:  fmt.Sprintf("jq available (%s)", version),
	}
}

// Check 9: Python available
func (hc *HealthChecker) checkPythonAvailable() CheckResult {
	path, err := exec.LookPath("python3")
	if err != nil {
		return CheckResult{
			Name:     "python_available",
			Category: "dependency",
			Status:   "warning",
			Message:  "python3 not available (ecphory ranking unavailable)",
			Fix:      getInstallCommand("python3"),
		}
	}

	// Get version
	cmd := exec.Command("python3", "--version")
	output, err := cmd.Output()
	if err != nil {
		return CheckResult{
			Name:     "python_available",
			Category: "dependency",
			Status:   "warning",
			Message:  fmt.Sprintf("python3 available at %s", path),
		}
	}

	version := strings.TrimSpace(string(output))
	return CheckResult{
		Name:     "python_available",
		Category: "dependency",
		Status:   "ok",
		Message:  version,
	}
}

// Check 10: Hooks configured
func (hc *HealthChecker) checkHooksConfigured() CheckResult {
	settingsPath := filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")

	// Check if settings file exists
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return CheckResult{
			Name:     "hooks_configured",
			Category: "hooks",
			Status:   "warning",
			Message:  "Claude settings.json not found",
		}
	}

	// Parse JSON
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return CheckResult{
			Name:     "hooks_configured",
			Category: "hooks",
			Status:   "warning",
			Message:  "Cannot parse settings.json",
		}
	}

	// Check for hooks
	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		return CheckResult{
			Name:     "hooks_configured",
			Category: "hooks",
			Status:   "warning",
			Message:  "No hooks configured in settings.json",
		}
	}

	// Check for SessionStart hooks
	sessionStart, ok := hooks["SessionStart"]
	if !ok || sessionStart == nil {
		return CheckResult{
			Name:     "hooks_configured",
			Category: "hooks",
			Status:   "warning",
			Message:  "No SessionStart hooks configured",
		}
	}

	return CheckResult{
		Name:     "hooks_configured",
		Category: "hooks",
		Status:   "ok",
	}
}

// Check 11: Hook scripts exist and executable
// Reads actually-configured hooks from settings.json and plugin hooks.json
func (hc *HealthChecker) checkHookScriptsExecutable() CheckResult {
	scripts := hc.discoverConfiguredHookCommands()
	if len(scripts) == 0 {
		return CheckResult{
			Name:     "hook_scripts_executable",
			Category: "hooks",
			Status:   "ok",
			Message:  "No hook commands configured",
		}
	}

	missing := []string{}
	notExecutable := []string{}

	for _, script := range scripts {
		expanded := expandHome(script)
		info, err := os.Stat(expanded)
		if err != nil {
			if os.IsNotExist(err) {
				missing = append(missing, script)
			}
			continue
		}
		if info.Mode()&0111 == 0 {
			notExecutable = append(notExecutable, script)
		}
	}

	if len(missing) > 0 {
		return CheckResult{
			Name:     "hook_scripts_executable",
			Category: "hooks",
			Status:   "warning",
			Message:  fmt.Sprintf("Hook commands missing: %s", strings.Join(missing, ", ")),
		}
	}

	if len(notExecutable) > 0 {
		return CheckResult{
			Name:     "hook_scripts_executable",
			Category: "hooks",
			Status:   "error",
			Message:  fmt.Sprintf("Hook commands not executable: %s", strings.Join(notExecutable, ", ")),
			Fix:      "engram doctor --auto-fix",
		}
	}

	return CheckResult{
		Name:     "hook_scripts_executable",
		Category: "hooks",
		Status:   "ok",
	}
}

// discoverConfiguredHookCommands reads settings.json and plugin hooks.json
// to find all configured hook command paths.
func (hc *HealthChecker) discoverConfiguredHookCommands() []string {
	commands := []string{}
	seen := map[string]bool{}

	addCommand := func(cmd string) {
		if cmd != "" && !seen[cmd] {
			seen[cmd] = true
			commands = append(commands, cmd)
		}
	}

	// Read settings.json hooks
	settingsPath := filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")
	if data, err := os.ReadFile(settingsPath); err == nil {
		for _, cmd := range extractHookCommands(data) {
			addCommand(cmd)
		}
	}

	// Read enabled plugin hooks.json files
	if data, err := os.ReadFile(settingsPath); err == nil {
		for _, hooksFile := range discoverPluginHooksFiles(data) {
			expanded := expandHome(hooksFile)
			if hookData, err := os.ReadFile(expanded); err == nil {
				for _, cmd := range extractHookCommands(hookData) {
					addCommand(cmd)
				}
			}
		}
	}

	return commands
}

// extractHookCommands extracts all "command" values from a hooks JSON structure.
func extractHookCommands(data []byte) []string {
	var commands []string
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return commands
	}

	// Look for "hooks" key (plugin format) or hook categories directly
	hooksMap, ok := parsed["hooks"].(map[string]interface{})
	if !ok {
		hooksMap = parsed
	}

	for _, category := range hooksMap {
		entries, ok := category.([]interface{})
		if !ok {
			continue
		}
		for _, entry := range entries {
			entryMap, ok := entry.(map[string]interface{})
			if !ok {
				continue
			}
			if cmd, ok := entryMap["command"].(string); ok {
				commands = append(commands, cmd)
			}
			if hooks, ok := entryMap["hooks"].([]interface{}); ok {
				for _, hook := range hooks {
					hookMap, ok := hook.(map[string]interface{})
					if !ok {
						continue
					}
					if cmd, ok := hookMap["command"].(string); ok {
						commands = append(commands, cmd)
					}
				}
			}
		}
	}

	return commands
}

// discoverPluginHooksFiles finds hooks.json paths for enabled plugins.
func discoverPluginHooksFiles(settingsData []byte) []string {
	var paths []string
	var settings map[string]interface{}
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		return paths
	}

	enabled, ok := settings["enabledPlugins"].(map[string]interface{})
	if !ok {
		return paths
	}

	homeDir := os.Getenv("HOME")
	pluginSearchDirs := []string{
		filepath.Join(homeDir, ".claude", "plugins"),
		filepath.Join(homeDir, "src", "engram"),
	}

	for pluginKey, val := range enabled {
		isEnabled, ok := val.(bool)
		if !ok || !isEnabled {
			continue
		}

		pluginName := pluginKey
		if idx := strings.Index(pluginKey, "@"); idx >= 0 {
			pluginName = pluginKey[:idx]
		}

		for _, searchDir := range pluginSearchDirs {
			candidate := filepath.Join(searchDir, pluginName+"-plugin", "hooks", "hooks.json")
			if _, err := os.Stat(candidate); err == nil {
				paths = append(paths, candidate)
				break
			}
		}
	}

	return paths
}

// expandHome replaces ~ with the home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(os.Getenv("HOME"), path[2:])
	}
	return path
}

// RunHookChecks runs only hook-related health checks.
func (hc *HealthChecker) RunHookChecks() ([]CheckResult, error) {
	checks := []CheckResult{}
	checks = append(checks, hc.checkHooksConfigured())
	checks = append(checks, hc.checkHookScriptsExecutable())
	checks = append(checks, hc.checkHookExtensionMatch())
	checks = append(checks, hc.checkHookTimeouts())
	checks = append(checks, hc.checkHookConfigSyntax())
	return checks, nil
}

// Check 12: Hook extension mismatches
func (hc *HealthChecker) checkHookExtensionMatch() CheckResult {
	commands := hc.discoverConfiguredHookCommands()
	mismatches := []string{}

	for _, cmd := range commands {
		expanded := expandHome(cmd)
		if _, err := os.Stat(expanded); err == nil {
			continue // Exists as-is
		}

		// Check if removing .py/.sh finds the binary
		ext := filepath.Ext(expanded)
		if ext == ".py" || ext == ".sh" {
			withoutExt := strings.TrimSuffix(expanded, ext)
			if _, err := os.Stat(withoutExt); err == nil {
				mismatches = append(mismatches,
					fmt.Sprintf("%s (binary exists without %s)", cmd, ext))
			}
		}

		// Check if adding .py/.sh finds a script
		if ext == "" {
			for _, tryExt := range []string{".py", ".sh"} {
				if _, err := os.Stat(expanded + tryExt); err == nil {
					mismatches = append(mismatches,
						fmt.Sprintf("%s (exists as %s%s)", cmd, filepath.Base(expanded), tryExt))
				}
			}
		}
	}

	if len(mismatches) > 0 {
		return CheckResult{
			Name:     "hook_extension_match",
			Category: "hooks",
			Status:   "warning",
			Message:  fmt.Sprintf("Extension mismatches: %s", strings.Join(mismatches, "; ")),
			Fix:      "engram doctor --auto-fix",
		}
	}

	return CheckResult{
		Name:     "hook_extension_match",
		Category: "hooks",
		Status:   "ok",
	}
}

// Check 13: Hook paths valid (files exist at configured paths)
func (hc *HealthChecker) checkHookPathsValid() CheckResult {
	commands := hc.discoverConfiguredHookCommands()
	invalidPaths := []string{}
	wrongPaths := map[string]string{} // cmd -> suggested fix

	for _, cmd := range commands {
		expanded := expandHome(cmd)
		if _, err := os.Stat(expanded); err != nil {
			if os.IsNotExist(err) {
				// Check for known path corrections
				corrected := hc.suggestPathCorrection(cmd)
				if corrected != "" {
					wrongPaths[cmd] = corrected
				} else {
					invalidPaths = append(invalidPaths, cmd)
				}
			}
		}
	}

	if len(wrongPaths) > 0 {
		suggestions := []string{}
		for old, new := range wrongPaths {
			suggestions = append(suggestions, fmt.Sprintf("%s → %s", old, new))
		}
		return CheckResult{
			Name:     "hook_paths_valid",
			Category: "hooks",
			Status:   "warning",
			Message:  fmt.Sprintf("Hook paths need correction: %s", strings.Join(suggestions, "; ")),
			Fix:      "engram doctor --auto-fix",
		}
	}

	if len(invalidPaths) > 0 {
		return CheckResult{
			Name:     "hook_paths_valid",
			Category: "hooks",
			Status:   "warning",
			Message:  fmt.Sprintf("Hook paths missing: %s", strings.Join(invalidPaths, ", ")),
		}
	}

	return CheckResult{
		Name:     "hook_paths_valid",
		Category: "hooks",
		Status:   "ok",
	}
}

// suggestPathCorrection returns a corrected path if a known mapping exists.
func (hc *HealthChecker) suggestPathCorrection(path string) string {
	// Known bad → good path mappings
	corrections := map[string]string{
		"/main/hooks/":                 "/hooks/",
		"/.claude/hooks/sessionstart/": "/.claude/hooks/session-start/",
		"/src/ws/oss/.claude/hooks/":   "/src/ws/oss/repos/engram-research/.claude/hooks/",
	}

	for badPattern, goodPattern := range corrections {
		if strings.Contains(path, badPattern) {
			corrected := strings.Replace(path, badPattern, goodPattern, 1)
			// Verify the corrected path exists
			expanded := expandHome(corrected)
			if _, err := os.Stat(expanded); err == nil {
				return corrected
			}
		}
	}

	return ""
}

// Check 14: Hook timeout sanity
func (hc *HealthChecker) checkHookTimeouts() CheckResult {
	settingsPath := filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return CheckResult{
			Name:     "hook_timeouts",
			Category: "hooks",
			Status:   "ok",
		}
	}

	excessive := []string{}
	entries := extractHookEntriesWithPhase(data)
	for _, e := range entries {
		if e.Timeout > 60 {
			excessive = append(excessive,
				fmt.Sprintf("%s (%s, %ds)", e.Command, e.Phase, e.Timeout))
		}
	}

	// Also check plugin hooks
	for _, hf := range discoverPluginHooksFiles(data) {
		expanded := expandHome(hf)
		if pluginData, err := os.ReadFile(expanded); err == nil {
			for _, e := range extractHookEntriesWithPhase(pluginData) {
				if e.Timeout > 60 {
					excessive = append(excessive,
						fmt.Sprintf("%s (%s, %ds)", e.Command, e.Phase, e.Timeout))
				}
			}
		}
	}

	if len(excessive) > 0 {
		return CheckResult{
			Name:     "hook_timeouts",
			Category: "hooks",
			Status:   "warning",
			Message:  fmt.Sprintf("Excessive timeouts (>60s): %s", strings.Join(excessive, "; ")),
		}
	}

	return CheckResult{
		Name:     "hook_timeouts",
		Category: "hooks",
		Status:   "ok",
	}
}

// hookEntryWithPhase is used for timeout checking.
type hookEntryWithPhase struct {
	Command string
	Timeout int
	Phase   string
}

// extractHookEntriesWithPhase extracts hook entries with phase and timeout info.
func extractHookEntriesWithPhase(data []byte) []hookEntryWithPhase {
	var entries []hookEntryWithPhase
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return entries
	}

	hooksMap, ok := parsed["hooks"].(map[string]interface{})
	if !ok {
		hooksMap = parsed
	}

	for phase, category := range hooksMap {
		items, ok := category.([]interface{})
		if !ok {
			continue
		}
		for _, item := range items {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if cmd, ok := itemMap["command"].(string); ok {
				timeout := 0
				if t, ok := itemMap["timeout"].(float64); ok {
					timeout = int(t)
				}
				entries = append(entries, hookEntryWithPhase{
					Command: cmd, Timeout: timeout, Phase: phase,
				})
			}
			if hooks, ok := itemMap["hooks"].([]interface{}); ok {
				for _, hook := range hooks {
					hookMap, ok := hook.(map[string]interface{})
					if !ok {
						continue
					}
					if cmd, ok := hookMap["command"].(string); ok {
						timeout := 0
						if t, ok := hookMap["timeout"].(float64); ok {
							timeout = int(t)
						}
						entries = append(entries, hookEntryWithPhase{
							Command: cmd, Timeout: timeout, Phase: phase,
						})
					}
				}
			}
		}
	}

	return entries
}

// Check 14: Hook config syntax validation
func (hc *HealthChecker) checkHookConfigSyntax() CheckResult {
	settingsPath := filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")

	// Validate plugin hooks.json files
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return CheckResult{
			Name:     "hook_config_syntax",
			Category: "hooks",
			Status:   "ok",
		}
	}

	invalid := []string{}
	for _, hf := range discoverPluginHooksFiles(data) {
		expanded := expandHome(hf)
		pluginData, err := os.ReadFile(expanded)
		if err != nil {
			invalid = append(invalid, fmt.Sprintf("%s (unreadable)", hf))
			continue
		}
		var parsed interface{}
		if err := json.Unmarshal(pluginData, &parsed); err != nil {
			invalid = append(invalid, fmt.Sprintf("%s (invalid JSON: %v)", hf, err))
		}
	}

	if len(invalid) > 0 {
		return CheckResult{
			Name:     "hook_config_syntax",
			Category: "hooks",
			Status:   "error",
			Message:  fmt.Sprintf("Invalid hook configs: %s", strings.Join(invalid, "; ")),
		}
	}

	return CheckResult{
		Name:     "hook_config_syntax",
		Category: "hooks",
		Status:   "ok",
	}
}

// Check 12: File permissions correct
func (hc *HealthChecker) checkFilePermissions() CheckResult {
	// Check cache files (should be 600)
	cacheDir := filepath.Join(hc.workspace, "cache")
	if info, err := os.Stat(cacheDir); err == nil && info.IsDir() {
		entries, _ := os.ReadDir(cacheDir)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.Mode().Perm() != 0600 {
				return CheckResult{
					Name:     "file_permissions",
					Category: "security",
					Status:   "warning",
					Message:  fmt.Sprintf("Cache file %s has incorrect permissions", entry.Name()),
					Fix:      "engram doctor --fix",
				}
			}
		}
	}

	return CheckResult{
		Name:     "file_permissions",
		Category: "security",
		Status:   "ok",
	}
}

// Check 15: Marketplace config valid
func (hc *HealthChecker) checkMarketplaceConfigValid() CheckResult {
	mktPath := filepath.Join(os.Getenv("HOME"), ".claude", "plugins", "known_marketplaces.json")

	data, err := os.ReadFile(mktPath)
	if err != nil {
		if os.IsNotExist(err) {
			return CheckResult{
				Name:     "marketplace_config_valid",
				Category: "marketplace",
				Status:   "ok",
				Message:  "No marketplace config (none installed)",
			}
		}
		return CheckResult{
			Name:     "marketplace_config_valid",
			Category: "marketplace",
			Status:   "warning",
			Message:  fmt.Sprintf("Cannot read marketplace config: %v", err),
		}
	}

	// Parse JSON
	var marketplaces map[string]json.RawMessage
	if err := json.Unmarshal(data, &marketplaces); err != nil {
		return CheckResult{
			Name:     "marketplace_config_valid",
			Category: "marketplace",
			Status:   "error",
			Message:  "Marketplace config is malformed JSON — WILL CRASH CLAUDE CODE!",
			Fix:      "engram doctor --auto-fix",
		}
	}

	// Validate each entry
	var invalid []string
	for name, raw := range marketplaces {
		var entry map[string]interface{}
		if err := json.Unmarshal(raw, &entry); err != nil {
			invalid = append(invalid, fmt.Sprintf("%s (unparseable)", name))
			continue
		}

		source, ok := entry["source"]
		if !ok {
			invalid = append(invalid, fmt.Sprintf("%s (no source field)", name))
			continue
		}

		// Source can be either a string (direct path) or an object (github/url/directory)
		switch sourceVal := source.(type) {
		case string:
			// Direct path string is valid (this is our fixed format)
			if sourceVal == "" {
				invalid = append(invalid, fmt.Sprintf("%s (empty source)", name))
			}
		case map[string]interface{}:
			// Source is an object - check nested source field
			if nestedSource, ok := sourceVal["source"].(string); ok {
				switch nestedSource {
				case "github", "url":
					// Valid
				case "directory":
					// Invalid - this will crash Claude Code
					invalid = append(invalid, fmt.Sprintf("%s (source=%q)", name, nestedSource))
				default:
					invalid = append(invalid, fmt.Sprintf("%s (unknown source=%q)", name, nestedSource))
				}
			} else {
				invalid = append(invalid, fmt.Sprintf("%s (malformed source object)", name))
			}
		default:
			invalid = append(invalid, fmt.Sprintf("%s (invalid source type: %T)", name, source))
		}
	}

	if len(invalid) > 0 {
		return CheckResult{
			Name:     "marketplace_config_valid",
			Category: "marketplace",
			Status:   "error",
			Message:  fmt.Sprintf("Invalid marketplace entries — WILL CRASH CLAUDE CODE! %s", strings.Join(invalid, ", ")),
			Fix:      "engram doctor --auto-fix",
		}
	}

	return CheckResult{
		Name:     "marketplace_config_valid",
		Category: "marketplace",
		Status:   "ok",
		Message:  fmt.Sprintf("%d marketplace(s) configured", len(marketplaces)),
	}
}

// Check 16: Enabled plugins have registered marketplaces
func (hc *HealthChecker) checkEnabledPluginMarketplaces() CheckResult {
	settingsPath := filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")
	settingsData, err := os.ReadFile(settingsPath)
	if err != nil {
		return CheckResult{
			Name:     "marketplace_plugins_available",
			Category: "marketplace",
			Status:   "ok",
		}
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		return CheckResult{
			Name:     "marketplace_plugins_available",
			Category: "marketplace",
			Status:   "ok",
		}
	}

	enabled, ok := settings["enabledPlugins"].(map[string]interface{})
	if !ok {
		return CheckResult{
			Name:     "marketplace_plugins_available",
			Category: "marketplace",
			Status:   "ok",
		}
	}

	// Load marketplace config
	mktPath := filepath.Join(os.Getenv("HOME"), ".claude", "plugins", "known_marketplaces.json")
	mktData, err := os.ReadFile(mktPath)
	var marketplaces map[string]json.RawMessage
	if err == nil {
		json.Unmarshal(mktData, &marketplaces)
	}

	// Cross-reference: check if any enabled plugin's marketplace source is missing
	var orphaned []string
	for pluginKey, val := range enabled {
		isEnabled, ok := val.(bool)
		if !ok || !isEnabled {
			continue
		}

		// Extract marketplace name from plugin key (part after @)
		atIdx := strings.Index(pluginKey, "@")
		if atIdx < 0 {
			continue // No marketplace reference
		}
		mktName := pluginKey[atIdx+1:]

		// Check if this marketplace is registered
		if marketplaces != nil {
			found := false
			for name := range marketplaces {
				if name == mktName || strings.Contains(name, mktName) {
					found = true
					break
				}
			}
			if !found {
				// Only warn for marketplace-sourced plugins, not local ones
				// Local plugins (from ~/src/engram) won't have marketplace entries
				pluginName := pluginKey[:atIdx]
				localPlugin := false
				homeDir := os.Getenv("HOME")
				localPaths := []string{
					filepath.Join(homeDir, "src", "engram", pluginName+"-plugin"),
					filepath.Join(homeDir, ".claude", "plugins", pluginName+"-plugin"),
				}
				for _, lp := range localPaths {
					if _, err := os.Stat(lp); err == nil {
						localPlugin = true
						break
					}
				}
				if !localPlugin {
					orphaned = append(orphaned, pluginKey)
				}
			}
		}
	}

	if len(orphaned) > 0 {
		return CheckResult{
			Name:     "marketplace_plugins_available",
			Category: "marketplace",
			Status:   "warning",
			Message:  fmt.Sprintf("Plugins without registered marketplace: %s", strings.Join(orphaned, ", ")),
		}
	}

	return CheckResult{
		Name:     "marketplace_plugins_available",
		Category: "marketplace",
		Status:   "ok",
	}
}

// getInstallCommand returns platform-appropriate install command
func getInstallCommand(pkg string) string {
	// Detect platform
	if _, err := exec.LookPath("brew"); err == nil {
		return fmt.Sprintf("brew install %s", pkg)
	}
	if _, err := exec.LookPath("apt"); err == nil {
		return fmt.Sprintf("sudo apt install %s", pkg)
	}
	if _, err := exec.LookPath("yum"); err == nil {
		return fmt.Sprintf("sudo yum install %s", pkg)
	}
	if _, err := exec.LookPath("pacman"); err == nil {
		return fmt.Sprintf("sudo pacman -S %s", pkg)
	}
	return fmt.Sprintf("Install %s using your package manager", pkg)
}

// Check 13: Plugin health checks
func (hc *HealthChecker) checkPluginHealth() []CheckResult {
	results := []CheckResult{}

	// Discover plugins
	plugins := hc.discoverPlugins()
	if len(plugins) == 0 {
		return results // No plugins, no checks
	}

	// Run health checks for each plugin (parallel)
	resultsChan := make(chan CheckResult, len(plugins))

	for _, plugin := range plugins {
		go func(p PluginInfo) {
			resultsChan <- hc.executePluginHealthCheck(p)
		}(plugin)
	}

	// Collect results
	for i := 0; i < len(plugins); i++ {
		results = append(results, <-resultsChan)
	}

	return results
}

// PluginInfo represents a discovered plugin
type PluginInfo struct {
	Name       string
	Path       string
	HealthFile string
}

// discoverPlugins finds all plugins with health-check.sh scripts
func (hc *HealthChecker) discoverPlugins() []PluginInfo {
	plugins := []PluginInfo{}

	pluginsDir := filepath.Join(hc.workspace, "plugins")
	if _, err := os.Stat(pluginsDir); os.IsNotExist(err) {
		return plugins // No plugins directory
	}

	// Scan plugins directory
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		return plugins
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Look for health-check.sh script
		healthFile := filepath.Join(pluginsDir, entry.Name(), "health-check.sh")
		if _, err := os.Stat(healthFile); err == nil {
			plugins = append(plugins, PluginInfo{
				Name:       entry.Name(),
				Path:       filepath.Join(pluginsDir, entry.Name()),
				HealthFile: healthFile,
			})
		}
	}

	return plugins
}

// executePluginHealthCheck runs a plugin's health-check.sh script
func (hc *HealthChecker) executePluginHealthCheck(plugin PluginInfo) CheckResult {
	// Check if script is executable
	info, err := os.Stat(plugin.HealthFile)
	if err != nil {
		return CheckResult{
			Name:     fmt.Sprintf("plugin_%s", plugin.Name),
			Category: "plugin",
			Status:   "error",
			Message:  fmt.Sprintf("Plugin %s health script missing", plugin.Name),
		}
	}

	if info.Mode()&0111 == 0 {
		return CheckResult{
			Name:     fmt.Sprintf("plugin_%s", plugin.Name),
			Category: "plugin",
			Status:   "error",
			Message:  fmt.Sprintf("Plugin %s health script not executable", plugin.Name),
			Fix:      fmt.Sprintf("chmod +x %s", plugin.HealthFile),
		}
	}

	// Execute health check script (with timeout)
	cmd := exec.Command(plugin.HealthFile)
	cmd.Dir = plugin.Path
	output, err := cmd.CombinedOutput()

	// Script exit codes:
	// 0 = healthy
	// 1 = warning
	// 2 = error
	// Other = error
	exitCode := 0
	if err != nil {
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return CheckResult{
				Name:     fmt.Sprintf("plugin_%s", plugin.Name),
				Category: "plugin",
				Status:   "error",
				Message:  fmt.Sprintf("Plugin %s health check failed: %v", plugin.Name, err),
			}
		}
	}

	message := strings.TrimSpace(string(output))
	if message == "" {
		message = fmt.Sprintf("Plugin %s is healthy", plugin.Name)
	}

	status := "ok"
	switch exitCode {
	case 1:
		status = "warning"
	case 2:
		status = "error"
	default:
		if exitCode != 0 {
			status = "error"
		}
		message = fmt.Sprintf("Plugin %s health check returned unexpected exit code %d", plugin.Name, exitCode)
	}

	return CheckResult{
		Name:     fmt.Sprintf("plugin_%s", plugin.Name),
		Category: "plugin",
		Status:   status,
		Message:  message,
	}
}
