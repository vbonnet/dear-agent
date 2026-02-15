package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds Astrocyte daemon configuration.
// Configuration is loaded from YAML file (~/.config/astrocyte/config.yaml by default).
type Config struct {
	// Patterns specifies paths to pattern database YAML files
	Patterns PatternConfig `yaml:"patterns"`

	// Violations configures violation logging
	Violations ViolationsConfig `yaml:"violations"`

	// Monitoring configures session monitoring behavior
	Monitoring MonitoringConfig `yaml:"monitoring"`

	// Tmux configures tmux client behavior
	Tmux TmuxConfig `yaml:"tmux"`

	// Recovery configures recovery strategies
	Recovery RecoveryConfig `yaml:"recovery"`

	// Logging configures incident logging
	Logging LoggingConfig `yaml:"logging"`

	// EventBus configures event bus integration (optional)
	EventBus EventBusConfig `yaml:"eventbus,omitempty"`

	// Temporal configures Temporal workflow integration (optional)
	Temporal TemporalConfig `yaml:"temporal,omitempty"`
}

// PatternConfig specifies paths to pattern database YAML files.
type PatternConfig struct {
	Bash  string `yaml:"bash"`
	Beads string `yaml:"beads"`
	Git   string `yaml:"git"`
}

// ViolationsConfig configures violation logging.
type ViolationsConfig struct {
	Directory string `yaml:"directory"`
}

// MonitoringConfig configures session monitoring behavior.
type MonitoringConfig struct {
	// Interval between session checks (e.g., "60s", "1m")
	Interval string `yaml:"interval"`
	// StuckThreshold is minimum duration before considering session stuck (e.g., "10m", "15m")
	StuckThreshold string `yaml:"stuck_threshold"`

	// Parsed durations (populated from strings)
	IntervalDuration       time.Duration `yaml:"-"`
	StuckThresholdDuration time.Duration `yaml:"-"`
}

// TmuxConfig configures tmux client behavior.
type TmuxConfig struct {
	// Socket path to tmux socket (empty string = use default)
	Socket string `yaml:"socket"`
}

// RecoveryConfig configures recovery strategies.
type RecoveryConfig struct {
	// Enabled controls whether recovery is attempted
	Enabled bool `yaml:"enabled"`
	// Strategy specifies recovery approach: "escape", "ctrl_c", "restart", "manual"
	Strategy string `yaml:"strategy"`
	// MaxAttempts limits number of recovery attempts per session
	MaxAttempts int `yaml:"max_attempts"`
}

// LoggingConfig configures incident logging.
type LoggingConfig struct {
	// IncidentsFile is path to incidents.jsonl
	IncidentsFile string `yaml:"incidents_file"`
	// DiagnosesDir is path to diagnoses directory
	DiagnosesDir string `yaml:"diagnoses_dir"`
	// Verbose enables detailed logging
	Verbose bool `yaml:"verbose"`
}

// EventBusConfig configures event bus integration (optional).
type EventBusConfig struct {
	Enabled bool   `yaml:"enabled"`
	Broker  string `yaml:"broker"`
}

// TemporalConfig configures Temporal workflow integration (optional).
type TemporalConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Address   string `yaml:"address"`
	Namespace string `yaml:"namespace"`
}

// DefaultConfig returns configuration with sensible defaults.
// Conservative defaults prefer missing interruptions over false positives.
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()

	return &Config{
		Patterns: PatternConfig{
			Bash:  filepath.Join(homeDir, "src/ws/oss/repos/engram/patterns/bash-anti-patterns.yaml"),
			Beads: filepath.Join(homeDir, "src/ws/oss/repos/engram/patterns/beads-anti-patterns.yaml"),
			Git:   filepath.Join(homeDir, "src/ws/oss/repos/engram/patterns/git-anti-patterns.yaml"),
		},
		Violations: ViolationsConfig{
			Directory: filepath.Join(homeDir, "src/ws/oss/repos/engram/violations"),
		},
		Monitoring: MonitoringConfig{
			Interval:               "60s",  // Check every 60 seconds
			StuckThreshold:         "10m",  // Consider stuck after 10 minutes
			IntervalDuration:       60 * time.Second,
			StuckThresholdDuration: 10 * time.Minute,
		},
		Tmux: TmuxConfig{
			Socket: "", // Empty = use tmux default socket detection
		},
		Recovery: RecoveryConfig{
			Enabled:     true,
			Strategy:    "escape", // Send Escape key (safest default)
			MaxAttempts: 3,
		},
		Logging: LoggingConfig{
			IncidentsFile: filepath.Join(homeDir, ".agm/astrocyte/incidents.jsonl"),
			DiagnosesDir:  filepath.Join(homeDir, ".agm/astrocyte/diagnoses"),
			Verbose:       false,
		},
		EventBus: EventBusConfig{
			Enabled: false, // Disabled by default (optional integration)
		},
		Temporal: TemporalConfig{
			Enabled:   false, // Disabled by default (optional integration)
			Address:   "localhost:7233",
			Namespace: "default",
		},
	}
}

// LoadConfig loads configuration from a YAML file.
// Falls back to default configuration if file doesn't exist.
// Returns error only if file exists but cannot be parsed.
func LoadConfig(path string) (*Config, error) {
	// If path doesn't exist, use defaults
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	// Read config file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	config := DefaultConfig() // Start with defaults
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	// Parse duration strings
	if err := config.parseDurations(); err != nil {
		return nil, fmt.Errorf("failed to parse durations: %w", err)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// parseDurations parses duration strings into time.Duration values.
func (c *Config) parseDurations() error {
	var err error

	c.Monitoring.IntervalDuration, err = time.ParseDuration(c.Monitoring.Interval)
	if err != nil {
		return fmt.Errorf("invalid monitoring.interval: %w", err)
	}

	c.Monitoring.StuckThresholdDuration, err = time.ParseDuration(c.Monitoring.StuckThreshold)
	if err != nil {
		return fmt.Errorf("invalid monitoring.stuck_threshold: %w", err)
	}

	return nil
}

// Validate checks that configuration is valid.
// Returns error if any required fields are missing or invalid.
func (c *Config) Validate() error {
	// Validate pattern paths exist
	if c.Patterns.Bash == "" {
		return fmt.Errorf("patterns.bash path is required")
	}
	if c.Patterns.Beads == "" {
		return fmt.Errorf("patterns.beads path is required")
	}
	if c.Patterns.Git == "" {
		return fmt.Errorf("patterns.git path is required")
	}

	// Validate violations directory
	if c.Violations.Directory == "" {
		return fmt.Errorf("violations.directory is required")
	}

	// Validate monitoring durations
	if c.Monitoring.IntervalDuration <= 0 {
		return fmt.Errorf("monitoring.interval must be positive")
	}
	if c.Monitoring.StuckThresholdDuration <= 0 {
		return fmt.Errorf("monitoring.stuck_threshold must be positive")
	}

	// Validate recovery strategy
	validStrategies := map[string]bool{
		"escape":  true,
		"ctrl_c":  true,
		"restart": true,
		"manual":  true,
	}
	if !validStrategies[c.Recovery.Strategy] {
		return fmt.Errorf("invalid recovery.strategy: %s (must be escape, ctrl_c, restart, or manual)", c.Recovery.Strategy)
	}

	// Validate logging paths
	if c.Logging.IncidentsFile == "" {
		return fmt.Errorf("logging.incidents_file is required")
	}
	if c.Logging.DiagnosesDir == "" {
		return fmt.Errorf("logging.diagnoses_dir is required")
	}

	return nil
}

// ExpandPaths expands ~ and environment variables in all path fields.
func (c *Config) ExpandPaths() {
	homeDir, _ := os.UserHomeDir()

	// Expand pattern paths
	c.Patterns.Bash = expandPath(c.Patterns.Bash, homeDir)
	c.Patterns.Beads = expandPath(c.Patterns.Beads, homeDir)
	c.Patterns.Git = expandPath(c.Patterns.Git, homeDir)

	// Expand violations directory
	c.Violations.Directory = expandPath(c.Violations.Directory, homeDir)

	// Expand logging paths
	c.Logging.IncidentsFile = expandPath(c.Logging.IncidentsFile, homeDir)
	c.Logging.DiagnosesDir = expandPath(c.Logging.DiagnosesDir, homeDir)
}

// expandPath expands ~ to home directory and evaluates environment variables.
func expandPath(path, homeDir string) string {
	// Expand ~ to home directory
	if path[:1] == "~" {
		path = filepath.Join(homeDir, path[1:])
	}

	// Expand environment variables
	path = os.ExpandEnv(path)

	return path
}

// DefaultConfigPath returns the default config file path.
func DefaultConfigPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".config/astrocyte/config.yaml")
}
