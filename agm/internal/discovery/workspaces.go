package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"gopkg.in/yaml.v3"
)

// SessionLocation represents a session's current location in the filesystem
type SessionLocation struct {
	Workspace       string // e.g., "oss", "acme"
	SessionID       string // UUID from manifest
	Name            string // Session name (for unified storage path)
	ManifestPath    string // Current manifest.yaml path
	ConversationDir string // Current conversation directory
}

// DiscoveryResult holds the results of a cross-workspace session discovery,
// including diagnostic information about which directories were searched.
type DiscoveryResult struct {
	Locations    []SessionLocation
	DirsSearched []string // all directories that were scanned for manifests
	ConfigPath   string   // config file used ("" if legacy fallback)
	Method       string   // "config", "legacy", or "single-dir"
}

// agmConfig represents the AGM workspace configuration file (~/.agm/config.yaml)
type agmConfig struct {
	Version    int            `yaml:"version"`
	Workspaces []agmWorkspace `yaml:"workspaces"`
}

// agmWorkspace represents a workspace entry in the AGM config
type agmWorkspace struct {
	Name      string `yaml:"name"`
	Root      string `yaml:"root"`
	OutputDir string `yaml:"output_dir"`
	Enabled   bool   `yaml:"enabled"`
}

// FindSessionsAcrossWorkspaces discovers all sessions across all configured workspaces.
// Reads workspace config from ~/.agm/config.yaml to determine session locations.
// Falls back to legacy ~/src/ws/*/ scanning if config is not available.
func FindSessionsAcrossWorkspaces() (*DiscoveryResult, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Read workspace config
	configPath := filepath.Join(homeDir, ".agm", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		// Config doesn't exist - fall back to legacy scanning
		return findSessionsLegacy(homeDir)
	}

	var cfg agmConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		// Config is invalid - fall back to legacy scanning
		return findSessionsLegacy(homeDir)
	}

	if len(cfg.Workspaces) == 0 {
		return findSessionsLegacy(homeDir)
	}

	result := &DiscoveryResult{
		ConfigPath: configPath,
		Method:     "config",
	}

	// Scan workspace-specific directories
	for _, ws := range cfg.Workspaces {
		if !ws.Enabled {
			continue
		}

		root := expandConfigPath(ws.Root, homeDir)
		outputDir := expandConfigPath(ws.OutputDir, homeDir)
		if outputDir == "" {
			outputDir = root
		}

		// Compute sessions directories to scan (deduplicated)
		seen := make(map[string]bool)
		var sessionsDirs []string

		addDir := func(dir string) {
			if dir != "" && !seen[dir] {
				seen[dir] = true
				sessionsDirs = append(sessionsDirs, dir)
			}
		}

		// Primary: {output_dir}/sessions (when output_dir differs from root)
		if outputDir != root {
			addDir(filepath.Join(outputDir, "sessions"))
		}
		// Standard: {root}/.agm/sessions
		addDir(filepath.Join(root, ".agm", "sessions"))
		// Legacy: {root}/sessions
		addDir(filepath.Join(root, "sessions"))

		for _, sessionsDir := range sessionsDirs {
			result.DirsSearched = append(result.DirsSearched, sessionsDir)
			locs := scanSessionsDir(sessionsDir, ws.Name)
			result.Locations = append(result.Locations, locs...)
		}
	}

	// Also scan default fallback path ~/.claude/sessions (no workspace)
	claudeSessionsDir := filepath.Join(homeDir, ".claude", "sessions")
	result.DirsSearched = append(result.DirsSearched, claudeSessionsDir)
	locs := scanSessionsDir(claudeSessionsDir, "") // empty workspace = default
	result.Locations = append(result.Locations, locs...)

	return result, nil
}

// findSessionsLegacy uses the old hardcoded ~/src/ws/* pattern for backward compatibility
func findSessionsLegacy(homeDir string) (*DiscoveryResult, error) {
	workspacesPattern := filepath.Join(homeDir, "src", "ws", "*")
	workspaceDirs, err := filepath.Glob(workspacesPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to glob workspaces: %w", err)
	}

	result := &DiscoveryResult{
		Method: "legacy",
	}

	// Scan workspace-specific directories
	for _, wsDir := range workspaceDirs {
		wsName := filepath.Base(wsDir)
		sessionsDirs := []string{
			filepath.Join(wsDir, ".agm", "sessions"),
			filepath.Join(wsDir, "sessions"),
		}

		for _, sessionsDir := range sessionsDirs {
			result.DirsSearched = append(result.DirsSearched, sessionsDir)
			locs := scanSessionsDir(sessionsDir, wsName)
			result.Locations = append(result.Locations, locs...)
		}
	}

	// Also scan default fallback path ~/.claude/sessions (no workspace)
	claudeSessionsDir := filepath.Join(homeDir, ".claude", "sessions")
	result.DirsSearched = append(result.DirsSearched, claudeSessionsDir)
	locs := scanSessionsDir(claudeSessionsDir, "") // empty workspace = default
	result.Locations = append(result.Locations, locs...)

	return result, nil
}

// scanSessionsDir scans a directory for session manifests
func scanSessionsDir(sessionsDir, workspaceName string) []SessionLocation {
	if _, err := os.Stat(sessionsDir); os.IsNotExist(err) {
		return nil
	}

	sessionPattern := filepath.Join(sessionsDir, "*", "manifest.yaml")
	paths, err := filepath.Glob(sessionPattern)
	if err != nil {
		return nil
	}

	var locations []SessionLocation

	for _, manifestPath := range paths {
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue // Skip corrupted manifest
		}

		var m manifest.Manifest
		if err := yaml.Unmarshal(data, &m); err != nil {
			continue // Skip invalid YAML
		}

		sessionDir := filepath.Dir(manifestPath)
		locations = append(locations, SessionLocation{
			Workspace:       workspaceName,
			SessionID:       m.SessionID,
			Name:            m.Name,
			ManifestPath:    manifestPath,
			ConversationDir: sessionDir,
		})
	}

	return locations
}

// expandConfigPath expands ~ to home directory in config paths
func expandConfigPath(path, homeDir string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir, path[2:])
	}
	if path == "~" {
		return homeDir
	}
	return path
}
