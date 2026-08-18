package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/engram/internal/harnesseffort"
	"gopkg.in/yaml.v3"
)

// Loader handles loading and merging configuration files
type Loader struct {
	paths map[ConfigTier]string

	// Cache fields for performance optimization
	mu           sync.RWMutex
	cachedConfig *Config
	cachedMtimes map[ConfigTier]time.Time
}

// NewLoader creates a new configuration loader with default paths
func NewLoader() *Loader {
	return &Loader{
		paths:        DefaultPaths(),
		cachedMtimes: make(map[ConfigTier]time.Time),
	}
}

// Load reads and merges all configuration tiers into a single Config
// Uses caching with mtime-based invalidation for performance optimization
func (l *Loader) Load() (*Config, error) {
	// Check cache first (read lock)
	l.mu.RLock()
	if l.isCacheFresh() {
		cfg := l.cachedConfig
		l.mu.RUnlock()
		return cfg, nil
	}
	l.mu.RUnlock()

	// Cache miss - acquire write lock
	l.mu.Lock()
	defer l.mu.Unlock()

	// Double-check pattern: another goroutine may have populated cache
	if l.isCacheFresh() {
		return l.cachedConfig, nil
	}

	// Load config normally
	cfg := &Config{
		Platform: PlatformConfig{
			TokenBudget: 50000, // Default token budget
		},
		Plugins: PluginConfig{
			Security: SecurityConfig{
				ResourceLimits: ResourceLimitsConfig{
					MaxProcesses:       100,                    // Soft limit
					MaxProcessesHard:   200,                    // Hard limit
					MaxFileDescriptors: 8192,                   // Max open files
					MaxMemory:          4 * 1024 * 1024 * 1024, // 4GB
				},
			},
		},
		Telemetry: TelemetryConfig{
			Enabled: true,
		},
		VCS: VCSConfig{
			Enabled:       true,
			RepoPath:      "~/.engram/memories/",
			PushStrategy:  "async",
			BatchInterval: "5m",
			RemoteName:    "origin",
			Branch:        "main",
			Validation: VCSValidationConfig{
				RequireWhyFile: true,
				LintOnCommit:   true,
			},
		},
	}

	// Load tiers in order: core -> company -> team -> user
	// Later tiers override earlier ones
	tiers := []ConfigTier{TierCore, TierCompany, TierTeam, TierUser}
	newMtimes := make(map[ConfigTier]time.Time)

	for _, tier := range tiers {
		path := l.paths[tier]

		// Get mtime before loading
		info, statErr := os.Stat(path)

		if err := l.loadTier(path, cfg, tier); err != nil {
			// Skip missing files (only core is required)
			if tier != TierCore && os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("failed to load %s config from %s: %w\n\nTip: Run 'engram init' to create configuration files", tier, path, err)
		}

		// Store mtime if stat succeeded
		if statErr == nil {
			newMtimes[tier] = info.ModTime()
		}
	}

	// Update cache
	l.cachedConfig = cfg
	l.cachedMtimes = newMtimes

	return cfg, nil
}

// loadTier loads a single configuration file and merges it into cfg
func (l *Loader) loadTier(path string, cfg *Config, tier ConfigTier) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Unmarshal into temporary config
	var tierCfg Config
	if err := yaml.Unmarshal(data, &tierCfg); err != nil {
		return fmt.Errorf("invalid YAML syntax in %s: %w\n\nCheck for proper indentation and formatting", path, err)
	}

	// Merge into cfg (simple field-by-field merge)
	l.merge(cfg, &tierCfg, tier)

	return nil
}

// merge merges src into dst, with src taking precedence
func (l *Loader) merge(dst, src *Config, tier ConfigTier) {
	mergePlatform(&dst.Platform, &src.Platform)
	mergePlugins(&dst.Plugins, &src.Plugins)
	mergeTelemetry(&dst.Telemetry, &src.Telemetry, tier)
	mergeHarnessEffort(&dst.HarnessEffort, &src.HarnessEffort)
	mergeVCS(&dst.VCS, &src.VCS)
}

func mergePlatform(dst, src *PlatformConfig) {
	if src.Agent != "" {
		dst.Agent = src.Agent
	}
	if src.EngramPath != "" {
		dst.EngramPath = expandPathOrFallback(src.EngramPath)
	}
	if src.TokenBudget > 0 {
		dst.TokenBudget = src.TokenBudget
	}
}

func mergePlugins(dst, src *PluginConfig) {
	if len(src.Paths) > 0 {
		dst.Paths = expandPathsOrFallback(src.Paths)
	}
	if len(src.Disabled) > 0 {
		dst.Disabled = append(dst.Disabled, src.Disabled...)
	}
}

func mergeTelemetry(dst, src *TelemetryConfig, tier ConfigTier) {
	// Handle enforcement (P0-2 security fix)
	// Only Core and Company tiers can set enforcement flag
	if src.Enforce && (tier == TierCore || tier == TierCompany) {
		dst.Enforce = true
	}

	// If enforced, user cannot disable telemetry
	if dst.Enforce {
		dst.Enabled = true
	} else {
		dst.Enabled = src.Enabled
	}

	// Merge other telemetry settings
	if src.Storage != "" {
		dst.Storage = src.Storage
	}
	if src.Path != "" {
		dst.Path = expandPathOrFallback(src.Path)
	}
	if src.MaxSizeMB > 0 {
		dst.MaxSizeMB = src.MaxSizeMB
	}
	if src.RetentionDays > 0 {
		dst.RetentionDays = src.RetentionDays
	}
	if src.Sink != nil {
		dst.Sink = src.Sink
	}
	if src.LocalBackup {
		dst.LocalBackup = src.LocalBackup
	}
}

func mergeHarnessEffort(dst, src *harnesseffort.HarnessEffortConfig) {
	if src.SubagentPreference != "" {
		dst.SubagentPreference = src.SubagentPreference
	}
	if len(src.ModelAliases) > 0 {
		if dst.ModelAliases == nil {
			dst.ModelAliases = make(map[string]string)
		}
		for k, v := range src.ModelAliases {
			dst.ModelAliases[k] = v
		}
	}
	if len(src.TaskTypes) > 0 {
		if dst.TaskTypes == nil {
			dst.TaskTypes = make(map[string]harnesseffort.TaskTypeConfig)
		}
		for k, v := range src.TaskTypes {
			dst.TaskTypes[k] = v
		}
	}
	if len(src.Tiers) > 0 {
		if dst.Tiers == nil {
			dst.Tiers = make(map[string]harnesseffort.TierConfig)
		}
		for k, v := range src.Tiers {
			dst.Tiers[k] = v
		}
	}
}

//nolint:gocyclo // reason: linear field-by-field merger; helpers per group would just shuffle complexity
func mergeVCS(dst, src *VCSConfig) {
	// Bool fields: only override if VCS section was explicitly provided
	// We detect this by checking if any field has a non-zero value
	hasVCSConfig := src.RepoPath != "" || src.PushStrategy != "" ||
		src.RemoteURL != "" || src.RemoteName != "" || src.Branch != ""

	if hasVCSConfig {
		dst.Enabled = src.Enabled
	}
	if src.RepoPath != "" {
		dst.RepoPath = expandPathOrFallback(src.RepoPath)
	}
	if src.PushStrategy != "" {
		dst.PushStrategy = src.PushStrategy
	}
	if src.BatchInterval != "" {
		dst.BatchInterval = src.BatchInterval
	}
	if src.RemoteURL != "" {
		dst.RemoteURL = src.RemoteURL
	}
	if src.RemoteName != "" {
		dst.RemoteName = src.RemoteName
	}
	if src.Branch != "" {
		dst.Branch = src.Branch
	}
	// Validation: override if any validation field is set
	if hasVCSConfig {
		dst.Validation.RequireWhyFile = src.Validation.RequireWhyFile
		dst.Validation.LintOnCommit = src.Validation.LintOnCommit
	}
	// OptIn: only set true values (opt-in semantics)
	if src.OptIn.ErrorMemory {
		dst.OptIn.ErrorMemory = true
	}
	if src.OptIn.EcphoryMetadata {
		dst.OptIn.EcphoryMetadata = true
	}
	if src.OptIn.Logs {
		dst.OptIn.Logs = true
	}
}

func expandPathOrFallback(path string) string {
	if expanded, err := expandPath(path); err == nil {
		return expanded
	}
	return path
}

func expandPathsOrFallback(paths []string) []string {
	expanded := make([]string, 0, len(paths))
	for _, path := range paths {
		expanded = append(expanded, expandPathOrFallback(path))
	}
	return expanded
}

// isCacheFresh checks if cached config is still valid by comparing mtimes
// Must be called with at least read lock held (mu.RLock or mu.Lock)
func (l *Loader) isCacheFresh() bool {
	if l.cachedConfig == nil {
		return false
	}

	tiers := []ConfigTier{TierCore, TierCompany, TierTeam, TierUser}
	for _, tier := range tiers {
		path := l.paths[tier]

		info, err := os.Stat(path)
		if err != nil {
			// File error/missing
			if os.IsNotExist(err) {
				// Check if this is an optional tier
				if tier != TierCore {
					// Optional file - check if it was cached
					if _, wasCached := l.cachedMtimes[tier]; wasCached {
						// Was cached but now missing - invalidate
						return false
					}
					// Never existed, cache still valid
					continue
				}
			}
			// Core file missing or other error - invalidate
			return false
		}

		// Check mtime
		cachedMtime, ok := l.cachedMtimes[tier]
		if !ok || !info.ModTime().Equal(cachedMtime) {
			return false
		}
	}

	return true
}

// expandPath expands ~ to home directory
// Reused from plugin loader to ensure consistent path expansion
func expandPath(path string) (string, error) {
	if len(path) == 0 || path[0] != '~' {
		return path, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(homeDir, path[1:]), nil
}
