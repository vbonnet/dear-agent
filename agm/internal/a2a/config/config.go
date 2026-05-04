// Package config provides config-related functionality.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Config represents the engram and A2A configuration
type Config struct {
	Version     string            `yaml:"version"`
	Workspace   WorkspaceConfig   `yaml:"workspace"`
	A2A         A2AConfig         `yaml:"a2a"`
	Engram      EngramConfig      `yaml:"engram"`
	AGM         AGMConfig         `yaml:"agm"`
	Preferences PreferencesConfig `yaml:"preferences"`
}

// WorkspaceConfig contains workspace directory settings
type WorkspaceConfig struct {
	Root         string `yaml:"root"`
	SwarmDir     string `yaml:"swarm_dir"`
	WayfinderDir string `yaml:"wayfinder_dir"`
	ReposDir     string `yaml:"repos_dir"`
}

// A2AConfig contains A2A protocol settings
type A2AConfig struct {
	ChannelsDir   string `yaml:"channels_dir"`
	RetentionDays int    `yaml:"retention_days"`
	PollInterval  int    `yaml:"poll_interval"`
}

// EngramConfig contains engram-specific settings
type EngramConfig struct {
	PluginsDir string `yaml:"plugins_dir"`
	EngramsDir string `yaml:"engrams_dir"`
	LogsDir    string `yaml:"logs_dir"`
}

// AGMConfig contains AGM session management settings
type AGMConfig struct {
	ManifestsDir string `yaml:"manifests_dir"`
	DiagnosesDir string `yaml:"diagnoses_dir"`
}

// PreferencesConfig contains user preferences
type PreferencesConfig struct {
	DefaultModel string `yaml:"default_model"`
	AutoCommit   bool   `yaml:"auto_commit"`
	GitRemote    string `yaml:"git_remote"`
}

var (
	defaultConfig = Config{
		Version: "1.0",
		Workspace: WorkspaceConfig{
			Root:         "~/src",
			SwarmDir:     "~/src/swarm",
			WayfinderDir: "~/src/wf",
			ReposDir:     "~/src",
		},
		A2A: A2AConfig{
			ChannelsDir:   "~/src/a2a-channels/active",
			RetentionDays: 30,
			PollInterval:  60,
		},
		Engram: EngramConfig{
			PluginsDir: "~/.engram/plugins",
			EngramsDir: "~/.claude/context-aware-engrams",
			LogsDir:    "~/.engram/logs",
		},
		AGM: AGMConfig{
			ManifestsDir: "~/src/sessions",
			DiagnosesDir: "~/.agm/astrocyte/diagnoses",
		},
		Preferences: PreferencesConfig{
			DefaultModel: "sonnet",
			AutoCommit:   true,
			GitRemote:    "github",
		},
	}

	configCache     *Config
	configCacheLock sync.RWMutex
)

func getConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("~", ".engram", "config.yaml")
	}
	return filepath.Join(home, ".engram", "config.yaml")
}

// GetConfigPath is a variable to allow test overrides
var GetConfigPath = getConfigPath

// Load loads configuration from file or returns defaults
func Load(forceReload bool) (*Config, error) {
	configCacheLock.RLock()
	if configCache != nil && !forceReload {
		defer configCacheLock.RUnlock()
		return configCache, nil
	}
	configCacheLock.RUnlock()

	configCacheLock.Lock()
	defer configCacheLock.Unlock()

	if configCache != nil && !forceReload {
		return configCache, nil
	}

	configPath := GetConfigPath()

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		cfg := defaultConfig
		configCache = &cfg
		return configCache, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		cfg := defaultConfig
		configCache = &cfg
		return configCache, fmt.Errorf("failed to read config file (using defaults): %w", err)
	}

	var userConfig Config
	if err := yaml.Unmarshal(data, &userConfig); err != nil {
		cfg := defaultConfig
		configCache = &cfg
		return configCache, fmt.Errorf("failed to parse config file (using defaults): %w", err)
	}

	cfg := defaultConfig
	mergeConfig(&cfg, &userConfig)

	configCache = &cfg
	return configCache, nil
}

func mergeConfig(base, override *Config) {
	if override.Version != "" {
		base.Version = override.Version
	}
	mergeWorkspace(&base.Workspace, &override.Workspace)
	mergeA2A(&base.A2A, &override.A2A)
	mergeEngram(&base.Engram, &override.Engram)
	mergeAGM(&base.AGM, &override.AGM)
	mergePreferences(&base.Preferences, &override.Preferences)
}

func mergeWorkspace(base, override *WorkspaceConfig) {
	if override.Root != "" {
		base.Root = override.Root
	}
	if override.SwarmDir != "" {
		base.SwarmDir = override.SwarmDir
	}
	if override.WayfinderDir != "" {
		base.WayfinderDir = override.WayfinderDir
	}
	if override.ReposDir != "" {
		base.ReposDir = override.ReposDir
	}
}

func mergeA2A(base, override *A2AConfig) {
	if override.ChannelsDir != "" {
		base.ChannelsDir = override.ChannelsDir
	}
	if override.RetentionDays != 0 {
		base.RetentionDays = override.RetentionDays
	}
	if override.PollInterval != 0 {
		base.PollInterval = override.PollInterval
	}
}

func mergeEngram(base, override *EngramConfig) {
	if override.PluginsDir != "" {
		base.PluginsDir = override.PluginsDir
	}
	if override.EngramsDir != "" {
		base.EngramsDir = override.EngramsDir
	}
	if override.LogsDir != "" {
		base.LogsDir = override.LogsDir
	}
}

func mergeAGM(base, override *AGMConfig) {
	if override.ManifestsDir != "" {
		base.ManifestsDir = override.ManifestsDir
	}
	if override.DiagnosesDir != "" {
		base.DiagnosesDir = override.DiagnosesDir
	}
}

func mergePreferences(base, override *PreferencesConfig) {
	if override.DefaultModel != "" {
		base.DefaultModel = override.DefaultModel
	}
	base.AutoCommit = override.AutoCommit
	if override.GitRemote != "" {
		base.GitRemote = override.GitRemote
	}
}

// ExpandPath expands ~ and environment variables in path
func ExpandPath(path string) string {
	expanded := os.ExpandEnv(path)
	if len(expanded) > 0 && expanded[0] == '~' {
		home, err := os.UserHomeDir()
		if err == nil {
			if len(expanded) == 1 {
				return home
			}
			return filepath.Join(home, expanded[1:])
		}
	}
	return expanded
}

// GetChannelsDir returns the expanded A2A channels directory path
func GetChannelsDir() string {
	cfg, _ := Load(false)
	return ExpandPath(cfg.A2A.ChannelsDir)
}

// GetPollInterval returns the A2A auto-discovery poll interval (seconds)
func GetPollInterval() int {
	cfg, _ := Load(false)
	return cfg.A2A.PollInterval
}

// GetRetentionDays returns the A2A channel retention period (days)
func GetRetentionDays() int {
	cfg, _ := Load(false)
	return cfg.A2A.RetentionDays
}

// GetWorkspaceRoot returns the expanded workspace root directory
func GetWorkspaceRoot() string {
	cfg, _ := Load(false)
	return ExpandPath(cfg.Workspace.Root)
}

// GetSwarmDir returns the expanded swarm execution directory
func GetSwarmDir() string {
	cfg, _ := Load(false)
	return ExpandPath(cfg.Workspace.SwarmDir)
}

// GetWayfinderDir returns the expanded Wayfinder projects directory
func GetWayfinderDir() string {
	cfg, _ := Load(false)
	return ExpandPath(cfg.Workspace.WayfinderDir)
}

// GetPluginsDir returns the expanded engram plugins directory
func GetPluginsDir() string {
	cfg, _ := Load(false)
	return ExpandPath(cfg.Engram.PluginsDir)
}

// GetEngramsDir returns the expanded context-aware engrams directory
func GetEngramsDir() string {
	cfg, _ := Load(false)
	return ExpandPath(cfg.Engram.EngramsDir)
}

// GetManifestsDir returns the expanded AGM session manifests directory
func GetManifestsDir() string {
	cfg, _ := Load(false)
	return ExpandPath(cfg.AGM.ManifestsDir)
}

// GetDiagnosesDir returns the expanded Astrocyte diagnoses directory
func GetDiagnosesDir() string {
	cfg, _ := Load(false)
	return ExpandPath(cfg.AGM.DiagnosesDir)
}

// Validate validates the configuration
func Validate() error {
	cfg, err := Load(false)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.Version != "1.0" {
		return fmt.Errorf("unsupported config version: %s (expected 1.0)", cfg.Version)
	}

	criticalPaths := map[string]string{
		"a2a.channels_dir":    GetChannelsDir(),
		"workspace.swarm_dir": GetSwarmDir(),
	}

	for key, path := range criticalPaths {
		if path == "" {
			return fmt.Errorf("empty path for %s", key)
		}
	}

	return nil
}
