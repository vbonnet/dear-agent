// Package goldenref provides workspace root detection for golden reference
// enforcement. It reads a YAML config to determine which directories are
// protected workspace roots (always on main branch, no agent writes).
package goldenref

import (
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// DefaultConfigPath is the default location for the golden-ref config.
const DefaultConfigPath = "~/.config/golden-ref/config.yaml"

// SessionIsolationConfig holds settings for session-based worktree isolation.
type SessionIsolationConfig struct {
	Enabled       bool   `yaml:"enabled"`        // Enable session-based worktree auto-provisioning
	AutoProvision bool   `yaml:"auto_provision"` // Automatically create worktrees on first write
	BranchPrefix  string `yaml:"branch_prefix"`  // Prefix for session branch names (default: "session-")
	CleanupOnEnd  bool   `yaml:"cleanup_on_end"` // Mark worktrees for cleanup on session end
	MaxAgeDays    int    `yaml:"max_age_days"`   // Maximum age in days before cleanup (default: 7)
}

// Config holds workspace root definitions for golden reference enforcement.
type Config struct {
	Version          int                     `yaml:"version"`
	WorkspaceRoots   []string                `yaml:"workspace_roots"`
	WorktreeBase     string                  `yaml:"worktree_base"`
	SessionIsolation *SessionIsolationConfig `yaml:"session_isolation,omitempty"`

	// expandedRoots caches roots with ~ expanded (populated on load)
	expandedRoots []string
}

// LoadConfig reads the golden-ref config from the default path.
// Returns an empty config (not an error) if the file is missing or invalid.
// This fail-safe behavior ensures the hook never blocks due to config issues.
func LoadConfig() (*Config, error) {
	return LoadConfigFrom(expandHome(DefaultConfigPath))
}

// LoadConfigFrom reads the golden-ref config from the given path.
// Returns an empty config if the file is missing or invalid (fail-safe).
func LoadConfigFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return &Config{}, nil //nolint:nilerr // fail-safe: missing file = empty config
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return &Config{}, nil //nolint:nilerr // fail-safe: invalid YAML = empty config
	}

	// Pre-expand all roots for fast matching
	cfg.expandedRoots = make([]string, 0, len(cfg.WorkspaceRoots))
	for _, root := range cfg.WorkspaceRoots {
		expanded := expandHome(root)
		// Ensure trailing separator for prefix matching
		if !strings.HasSuffix(expanded, string(filepath.Separator)) {
			expanded += string(filepath.Separator)
		}
		cfg.expandedRoots = append(cfg.expandedRoots, expanded)
	}

	cfg.WorktreeBase = expandHome(cfg.WorktreeBase)

	return &cfg, nil
}

// IsWorkspaceRoot returns true if the given path is under any configured
// workspace root. Paths are cleaned and ~ is expanded before comparison.
// Uses path-boundary matching to avoid false positives (e.g. src vs src-backup).
func (c *Config) IsWorkspaceRoot(path string) bool {
	if len(c.expandedRoots) == 0 {
		return false
	}

	cleaned := filepath.Clean(expandHome(path))

	for _, root := range c.expandedRoots {
		rootClean := filepath.Clean(root)
		// Must match at a path boundary: cleaned starts with root + separator
		if strings.HasPrefix(cleaned, rootClean+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// ExpandedRoots returns workspace roots with ~ expanded to the home directory.
func (c *Config) ExpandedRoots() []string {
	return c.expandedRoots
}

// MatchedRoot returns the workspace root that contains the given path,
// or empty string if no match.
func (c *Config) MatchedRoot(path string) string {
	cleaned := filepath.Clean(expandHome(path))

	for i, root := range c.expandedRoots {
		rootClean := filepath.Clean(root)
		if strings.HasPrefix(cleaned, rootClean+string(filepath.Separator)) {
			return c.WorkspaceRoots[i]
		}
	}
	return ""
}

// NewConfigForTest creates a Config with pre-expanded absolute roots.
// Intended for use in tests that need to inject specific workspace roots.
func NewConfigForTest(roots []string, worktreeBase string) *Config {
	cfg := &Config{
		Version:        1,
		WorkspaceRoots: roots,
		WorktreeBase:   worktreeBase,
		expandedRoots:  make([]string, len(roots)),
	}
	for i, root := range roots {
		expanded := expandHome(root)
		if !strings.HasSuffix(expanded, string(filepath.Separator)) {
			expanded += string(filepath.Separator)
		}
		cfg.expandedRoots[i] = expanded
	}
	return cfg
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}
