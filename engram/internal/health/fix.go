package health

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FixOperation represents a safe auto-fix operation
type FixOperation struct {
	Name        string   // Human-readable name
	Description string   // What it does
	Files       []string // Files affected
	Reversible  bool     // Can it be undone?
}

// Tier1Fixer applies safe auto-fix operations (Tier 1 only)
type Tier1Fixer struct {
	workspace string
}

// NewTier1Fixer creates a new fixer instance
func NewTier1Fixer(workspace string) *Tier1Fixer {
	return &Tier1Fixer{workspace: workspace}
}

// CreateMissingDirectory creates a directory if missing (Tier 1 Operation 1)
func (f *Tier1Fixer) CreateMissingDirectory(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	return nil
}

// RotateLog rotates an oversized log file (Tier 1 Operation 2)
func (f *Tier1Fixer) RotateLog(logPath string) error {
	// Check if log exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return nil // Nothing to rotate
	}

	backupPath := logPath + ".1"

	// Move current log to .1 (overwrites existing .1)
	if err := os.Rename(logPath, backupPath); err != nil {
		return fmt.Errorf("rotate log: %w", err)
	}

	return nil
}

// FixPermissions sets correct file permissions (Tier 1 Operation 3)
func (f *Tier1Fixer) FixPermissions(path string, mode os.FileMode) error {
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}
	return nil
}

// PreviewFixes shows what will be fixed (before applying)
func (f *Tier1Fixer) PreviewFixes(checks []CheckResult) []FixOperation {
	ops := []FixOperation{}

	for _, check := range checks {
		if check.Status != "warning" && check.Status != "error" {
			continue
		}

		// Determine if this is Tier 1 fixable
		switch check.Name {
		case "logs_size":
			ops = append(ops, FixOperation{
				Name:        "Rotate large log",
				Description: fmt.Sprintf("Rotate %s → %s.1 (keep backup)", extractFilenameFromMessage(check.Message), extractFilenameFromMessage(check.Message)),
				Files:       []string{extractFilenameFromMessage(check.Message)},
				Reversible:  true,
			})

		case "cache_directory_missing", "cache_directory_writable":
			cacheDir := filepath.Join(f.workspace, "cache")
			ops = append(ops, FixOperation{
				Name:        "Create cache directory",
				Description: "Create ~/.engram/cache/ (permissions: 755)",
				Files:       []string{cacheDir},
				Reversible:  true,
			})

		case "logs_directory_writable":
			logsDir := filepath.Join(f.workspace, "logs")
			ops = append(ops, FixOperation{
				Name:        "Create logs directory",
				Description: "Create ~/.engram/logs/ (permissions: 755)",
				Files:       []string{logsDir},
				Reversible:  true,
			})

		case "file_permissions":
			ops = append(ops, FixOperation{
				Name:        "Fix file permissions",
				Description: "Set cache files to 600 (user-only)",
				Files:       []string{"~/.engram/cache/*"},
				Reversible:  true,
			})

		case "hook_scripts_executable":
			if strings.Contains(check.Message, "Hook commands missing") {
				ops = append(ops, FixOperation{
					Name:        "Remove non-existent hooks",
					Description: "Remove references to missing/disabled hook files",
					Files:       []string{"~/.claude/settings.json"},
					Reversible:  true,
				})
			} else {
				ops = append(ops, FixOperation{
					Name:        "Fix hook script permissions",
					Description: "Set hook binaries to 755 (executable)",
					Files:       []string{"~/.claude/hooks/*"},
					Reversible:  true,
				})
			}

		case "hook_extension_match":
			ops = append(ops, FixOperation{
				Name:        "Fix hook extension mismatches",
				Description: "Update hooks.json to match actual binary names",
				Files:       []string{"engram-plugin/hooks/hooks.json"},
				Reversible:  true,
			})

		case "hook_paths_valid":
			if strings.Contains(check.Message, "Hook paths need correction") {
				ops = append(ops, FixOperation{
					Name:        "Fix hook paths",
					Description: "Correct known wrong paths in hook configurations",
					Files:       []string{"~/.claude/settings.json", "plugin hooks.json files"},
					Reversible:  true,
				})
			}

		case "marketplace_config_valid":
			ops = append(ops, FixOperation{
				Name:        "Fix corrupted marketplace config",
				Description: "Remove invalid entries, back up original",
				Files:       []string{"~/.claude/plugins/known_marketplaces.json"},
				Reversible:  true,
			})
		}
	}

	return ops
}

// ApplyFixes executes Tier 1 operations
func (f *Tier1Fixer) ApplyFixes(ops []FixOperation, checks []CheckResult) (int, error) {
	applied := 0

	for _, op := range ops {
		check := f.findCheck(checks, op.Name)
		if check == nil {
			continue
		}

		if err := f.applyFix(check); err != nil {
			return applied, fmt.Errorf("apply fix '%s': %w", op.Name, err)
		}

		applied++
	}

	return applied, nil
}

// findCheck finds the check result matching an operation name.
func (f *Tier1Fixer) findCheck(checks []CheckResult, opName string) *CheckResult {
	for i := range checks {
		if matchesOperation(&checks[i], opName) {
			return &checks[i]
		}
	}
	return nil
}

// applyFix applies a fix based on check name.
func (f *Tier1Fixer) applyFix(check *CheckResult) error {
	switch check.Name {
	case "cache_directory_missing", "cache_directory_writable":
		return f.fixCacheDirectory()
	case "logs_directory_writable":
		return f.fixLogsDirectory()
	case "logs_size":
		return f.fixLogSize(check.Message)
	case "file_permissions":
		return f.fixFilePermissions()
	case "hook_scripts_executable":
		// Handle both missing hooks and permission issues
		if strings.Contains(check.Message, "missing") {
			return f.removeNonExistentHooks()
		}
		return f.fixHookScriptPermissions()
	case "hook_extension_match":
		return f.fixHookExtensionMismatches()
	case "hook_paths_valid":
		return f.fixHookPaths()
	case "marketplace_config_valid":
		return f.fixMarketplaceConfig()
	}
	return nil
}

// fixCacheDirectory creates the cache directory.
func (f *Tier1Fixer) fixCacheDirectory() error {
	cacheDir := filepath.Join(f.workspace, "cache")
	return f.CreateMissingDirectory(cacheDir)
}

// fixLogsDirectory creates the logs directory.
func (f *Tier1Fixer) fixLogsDirectory() error {
	logsDir := filepath.Join(f.workspace, "logs")
	return f.CreateMissingDirectory(logsDir)
}

// fixLogSize rotates a large log file.
func (f *Tier1Fixer) fixLogSize(message string) error {
	filename := extractFilenameFromMessage(message)
	logPath := filepath.Join(f.workspace, "logs", filename)
	return f.RotateLog(logPath)
}

// fixFilePermissions fixes permissions for all cache files.
func (f *Tier1Fixer) fixFilePermissions() error {
	cacheDir := filepath.Join(f.workspace, "cache")
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filePath := filepath.Join(cacheDir, entry.Name())
		if err := f.FixPermissions(filePath, 0600); err != nil {
			return err
		}
	}

	return nil
}

// fixHookScriptPermissions fixes permissions for configured hook scripts.
func (f *Tier1Fixer) fixHookScriptPermissions() error {
	// Use the same discovery as health checks
	hooksDir := filepath.Join(os.Getenv("HOME"), ".claude", "hooks")
	entries, err := os.ReadDir(hooksDir)
	if err != nil {
		return nil // No hooks dir, nothing to fix
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(hooksDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.Mode()&0111 == 0 {
			if err := f.FixPermissions(path, 0755); err != nil {
				return err
			}
		}
	}

	return nil
}

// fixHookExtensionMismatches fixes .py/.sh extension mismatches in settings.json and hooks.json.
// When config references foo.py but only foo exists, updates the reference.
func (f *Tier1Fixer) fixHookExtensionMismatches() error {
	settingsPath := filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")
	settingsData, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil
	}

	// Fix settings.json itself first
	if err := f.fixExtensionsInFile(settingsPath, settingsData); err != nil {
		return fmt.Errorf("fix settings.json extensions: %w", err)
	}

	// Re-read settings after potential update
	settingsData, _ = os.ReadFile(settingsPath)

	// Find and fix plugin hooks.json files
	for _, hf := range discoverPluginHooksFiles(settingsData) {
		expanded := expandHome(hf)
		data, err := os.ReadFile(expanded)
		if err != nil {
			continue
		}

		if err := f.fixExtensionsInFile(expanded, data); err != nil {
			// Log error but continue with other files
			continue
		}
	}

	return nil
}

// fixExtensionsInFile fixes extension mismatches in a single JSON file.
func (f *Tier1Fixer) fixExtensionsInFile(filePath string, data []byte) error {
	modified := false
	content := string(data)

	// Find commands with .py or .sh extensions where binary exists without extension
	for _, cmd := range extractHookCommands(data) {
		cmdExpanded := expandHome(cmd)
		if _, err := os.Stat(cmdExpanded); err == nil {
			continue // Exists as-is
		}

		ext := filepath.Ext(cmd)
		if ext == ".py" || ext == ".sh" {
			withoutExt := strings.TrimSuffix(cmdExpanded, ext)
			if _, err := os.Stat(withoutExt); err == nil {
				// Binary exists without extension — fix the reference
				fixedCmd := strings.TrimSuffix(cmd, ext)
				content = strings.ReplaceAll(content, `"`+cmd+`"`, `"`+fixedCmd+`"`)
				modified = true
			}
		}
	}

	if modified {
		// Back up original
		backupPath := filePath + ".bak"
		if err := os.WriteFile(backupPath, data, 0644); err != nil {
			return fmt.Errorf("backup file: %w", err)
		}

		// Write fixed content
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			return fmt.Errorf("write fixed file: %w", err)
		}
	}

	return nil
}

// fixHookPaths corrects known wrong hook paths in settings.json and plugin hooks.json.
func (f *Tier1Fixer) fixHookPaths() error {
	settingsPath := filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")
	settingsData, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil
	}

	// Fix settings.json itself first
	if err := f.fixPathsInFile(settingsPath, settingsData); err != nil {
		return fmt.Errorf("fix settings.json paths: %w", err)
	}

	// Re-read settings after potential update
	settingsData, _ = os.ReadFile(settingsPath)

	// Find and fix plugin hooks.json files
	for _, hf := range discoverPluginHooksFiles(settingsData) {
		expanded := expandHome(hf)
		data, err := os.ReadFile(expanded)
		if err != nil {
			continue
		}

		if err := f.fixPathsInFile(expanded, data); err != nil {
			// Log error but continue with other files
			continue
		}
	}

	return nil
}

// fixPathsInFile corrects known wrong paths in a single JSON file.
func (f *Tier1Fixer) fixPathsInFile(filePath string, data []byte) error {
	modified := false
	content := string(data)

	// Known bad → good path mappings
	corrections := map[string]string{
		"/main/hooks/":                                     "/hooks/",
		"/.claude/hooks/sessionstart/":                     "/.claude/hooks/session-start/",
		"/src/ws/oss/.claude/hooks/":                       "/src/ws/oss/repos/engram-research/.claude/hooks/",
		"~/src/ws/oss/repos/engram/main/hooks/":            "~/src/ws/oss/repos/engram/hooks/",
		"~/.claude/hooks/posttool-context-monitor.py":      "~/.claude/hooks/posttool-context-monitor",
		"~/.claude/hooks/pretool-beads-protection.py":      "~/.claude/hooks/pretool-beads-protection",
		"~/.claude/hooks/pretool-validate-paired-files.py": "~/.claude/hooks/pretool-validate-paired-files",
		"~/.claude/hooks/pretool-path-validator.py":        "~/.claude/hooks/pretool-path-validator",
		"~/.claude/hooks/posttool-auto-commit-beads.py":    "~/.claude/hooks/posttool-auto-commit-beads",
	}

	// Extract all hook commands and check each one
	commands := extractHookCommands(data)
	for _, cmd := range commands {
		cmdExpanded := expandHome(cmd)

		// Check if command exists as-is
		if _, err := os.Stat(cmdExpanded); err == nil {
			continue // Path is valid, no correction needed
		}

		// Try each correction pattern
		for badPattern, goodPattern := range corrections {
			if strings.Contains(cmd, badPattern) {
				// Apply correction to get the corrected command
				correctedCmd := strings.ReplaceAll(cmd, badPattern, goodPattern)
				correctedExpanded := expandHome(correctedCmd)

				// Check if the corrected path exists
				if _, err := os.Stat(correctedExpanded); err == nil {
					// Corrected path exists - apply the fix
					content = strings.ReplaceAll(content, `"`+cmd+`"`, `"`+correctedCmd+`"`)
					modified = true
					break // Found a working correction, move to next command
				}
			}
		}
	}

	if modified {
		// Back up original
		backupPath := filePath + ".bak"
		if err := os.WriteFile(backupPath, data, 0644); err != nil {
			return fmt.Errorf("backup file: %w", err)
		}

		// Write fixed content
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			return fmt.Errorf("write fixed file: %w", err)
		}
	}

	return nil
}

// removeNonExistentHooks removes references to missing/disabled hook files from settings.json.
func (f *Tier1Fixer) removeNonExistentHooks() error {
	settingsPath := filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil
	}

	// Parse settings
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("parse settings.json: %w", err)
	}

	// Back up original
	backupPath := settingsPath + ".bak"
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return fmt.Errorf("backup settings.json: %w", err)
	}

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return nil // No hooks section
	}

	modified := false

	// Process each hook category (SessionStart, PreToolUse, PostToolUse, etc.)
	for category, value := range hooks {
		entries, ok := value.([]any)
		if !ok {
			continue
		}

		filteredEntries := []any{}

		for _, entry := range entries {
			entryMap, ok := entry.(map[string]any)
			if !ok {
				filteredEntries = append(filteredEntries, entry)
				continue
			}

			// Check if this entry has a command
			if cmd, ok := entryMap["command"].(string); ok {
				cmdExpanded := expandHome(cmd)
				if _, err := os.Stat(cmdExpanded); err == nil {
					// Command exists, keep it
					filteredEntries = append(filteredEntries, entry)
				} else {
					// Command missing, skip it (remove from config)
					modified = true
				}
				continue
			}

			// Check if this entry has nested hooks array
			if nestedHooks, ok := entryMap["hooks"].([]any); ok {
				filteredNested := []any{}
				for _, nestedHook := range nestedHooks {
					nestedMap, ok := nestedHook.(map[string]any)
					if !ok {
						filteredNested = append(filteredNested, nestedHook)
						continue
					}

					if cmd, ok := nestedMap["command"].(string); ok {
						cmdExpanded := expandHome(cmd)
						if _, err := os.Stat(cmdExpanded); err == nil {
							// Command exists, keep it
							filteredNested = append(filteredNested, nestedHook)
						} else {
							// Command missing, skip it
							modified = true
						}
					}
				}

				// Update the hooks array
				if len(filteredNested) > 0 {
					entryMap["hooks"] = filteredNested
					filteredEntries = append(filteredEntries, entryMap)
				} else if len(filteredNested) != len(nestedHooks) {
					// All nested hooks were removed, skip the entire entry
					modified = true
				} else {
					// Keep entry with all nested hooks
					filteredEntries = append(filteredEntries, entry)
				}
				continue
			}

			// No command field, keep as-is
			filteredEntries = append(filteredEntries, entry)
		}

		// Update the category with filtered entries
		hooks[category] = filteredEntries
	}

	if modified {
		settings["hooks"] = hooks

		// Write updated settings
		out, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal settings.json: %w", err)
		}
		out = append(out, '\n')

		if err := os.WriteFile(settingsPath, out, 0644); err != nil {
			return fmt.Errorf("write settings.json: %w", err)
		}
	}

	return nil
}

// fixMarketplaceConfig removes invalid entries and fixes source="directory" in known_marketplaces.json.
func (f *Tier1Fixer) fixMarketplaceConfig() error {
	mktPath := filepath.Join(os.Getenv("HOME"), ".claude", "plugins", "known_marketplaces.json")

	data, err := os.ReadFile(mktPath)
	if err != nil {
		return nil // Nothing to fix
	}

	// Back up original
	backupPath := mktPath + ".bak"
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return fmt.Errorf("backup marketplace config: %w", err)
	}

	// Try to parse
	var marketplaces map[string]json.RawMessage
	if err := json.Unmarshal(data, &marketplaces); err != nil {
		// Completely malformed — write empty object
		if err := os.WriteFile(mktPath, []byte("{}"), 0644); err != nil {
			return fmt.Errorf("write empty marketplace config: %w", err)
		}
		return nil
	}

	// Filter and fix invalid entries
	cleaned := make(map[string]json.RawMessage)
	for name, raw := range marketplaces {
		// Try to parse the entry
		var entry map[string]interface{}
		if err := json.Unmarshal(raw, &entry); err != nil {
			continue // Skip unparseable entries
		}

		// Check if source field is an object (invalid) or string (valid)
		if sourceObj, ok := entry["source"].(map[string]interface{}); ok {
			// source is an object - need to convert it
			if sourcePath, ok := sourceObj["path"].(string); ok {
				// Convert source object to direct path string
				entry["source"] = sourcePath
				// Re-marshal the fixed entry
				if fixedRaw, err := json.Marshal(entry); err == nil {
					cleaned[name] = fixedRaw
				}
			}
			// If source object doesn't have a path or other valid structure, skip it
		} else if sourceStr, ok := entry["source"].(string); ok {
			// source is already a string, validate it's not "directory"
			if sourceStr != "directory" {
				cleaned[name] = raw // Keep valid string entries
			}
		}
	}

	// Write cleaned config
	out, err := json.MarshalIndent(cleaned, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cleaned marketplace config: %w", err)
	}
	out = append(out, '\n')

	if err := os.WriteFile(mktPath, out, 0644); err != nil {
		return fmt.Errorf("write cleaned marketplace config: %w", err)
	}

	return nil
}

// matchesOperation checks if a check result matches an operation
func matchesOperation(check *CheckResult, opName string) bool {
	switch opName {
	case "Rotate large log":
		return check.Name == "logs_size"
	case "Create cache directory":
		return check.Name == "cache_directory_missing" || check.Name == "cache_directory_writable"
	case "Create logs directory":
		return check.Name == "logs_directory_writable"
	case "Fix file permissions":
		return check.Name == "file_permissions"
	case "Fix hook script permissions":
		return check.Name == "hook_scripts_executable" && !strings.Contains(check.Message, "missing")
	case "Fix hook extension mismatches":
		return check.Name == "hook_extension_match"
	case "Fix hook paths":
		return check.Name == "hook_paths_valid"
	case "Remove non-existent hooks":
		return check.Name == "hook_scripts_executable" && strings.Contains(check.Message, "missing")
	case "Fix corrupted marketplace config":
		return check.Name == "marketplace_config_valid"
	}
	return false
}

// extractFilenameFromMessage extracts filename from error message
func extractFilenameFromMessage(message string) string {
	// Example message: "ecphory.log is 250MB (threshold: 100MB)"
	// Extract "ecphory.log"
	parts := splitMessage(message)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// splitMessage splits message by space and returns parts
func splitMessage(s string) []string {
	var parts []string
	var current string
	for _, r := range s {
		if r == ' ' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(r)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}
