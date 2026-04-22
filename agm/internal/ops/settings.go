package ops

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// removeFromAdditionalDirectories removes a directory from Claude's additionalDirectories in settings.json
// This is a best-effort operation that gracefully handles missing files or directories.
func removeFromAdditionalDirectories(dir string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")

	// Read existing settings
	var settings map[string]interface{}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Settings file doesn't exist, nothing to remove
			return nil
		}
		return fmt.Errorf("failed to read settings.json: %w", err)
	}

	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("failed to parse settings.json: %w", err)
	}

	// Get additionalDirectories array
	var additionalDirs []string
	if existing, ok := settings["additionalDirectories"]; ok {
		if dirs, ok := existing.([]interface{}); ok {
			for _, d := range dirs {
				if str, ok := d.(string); ok {
					additionalDirs = append(additionalDirs, str)
				}
			}
		}
	}

	// Filter out the directory to remove
	var filtered []string
	for _, d := range additionalDirs {
		if d != dir {
			filtered = append(filtered, d)
		}
	}

	// Update settings
	if len(filtered) == 0 {
		delete(settings, "additionalDirectories")
	} else {
		settings["additionalDirectories"] = filtered
	}

	// Write back to settings.json
	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, output, 0600); err != nil {
		return fmt.Errorf("failed to write settings.json: %w", err)
	}

	return nil
}
