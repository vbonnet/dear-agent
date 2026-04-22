// Package config provides configuration for the astrocyte daemon.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
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

	// Escalation configures auto-escalation behavior
	Escalation EscalationConfig `yaml:"escalation,omitempty"`

	// LoopMonitoring configures loop heartbeat monitoring
	LoopMonitoring LoopMonitoringConfig `yaml:"loop_monitoring,omitempty"`

	// EventBus configures event bus integration (optional)
	EventBus EventBusConfig `yaml:"eventbus,omitempty"`

	// Temporal configures Temporal workflow integration (optional)
	Temporal TemporalConfig `yaml:"temporal,omitempty"`
}

// PatternConfig specifies paths to pattern database YAML files.
type PatternConfig struct {
	Bash  string `yaml:"bash" validate:"required"`
	Beads string `yaml:"beads" validate:"required"`
	Git   string `yaml:"git" validate:"required"`
}

// ViolationsConfig configures violation logging.
type ViolationsConfig struct {
	Directory string `yaml:"directory" validate:"required"`
}

// MonitoringConfig configures session monitoring behavior.
type MonitoringConfig struct {
	// Interval between session checks (e.g., "60s", "1m")
	Interval string `yaml:"interval" validate:"required"`
	// StuckThreshold is minimum duration before considering session stuck (e.g., "10m", "15m")
	StuckThreshold string `yaml:"stuck_threshold" validate:"required"`

	// Parsed durations (populated from strings)
	IntervalDuration       time.Duration `yaml:"-" validate:"gt=0"`
	StuckThresholdDuration time.Duration `yaml:"-" validate:"gt=0"`
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
	Strategy string `yaml:"strategy" validate:"required,oneof=escape ctrl_c restart manual"`
	// MaxAttempts limits number of recovery attempts per session
	MaxAttempts int `yaml:"max_attempts"`
	// DedupCooldown is the cooldown period for incident deduplication (e.g., "15m")
	DedupCooldown string `yaml:"dedup_cooldown"`
	// Parsed duration (populated from string)
	DedupCooldownDuration time.Duration `yaml:"-"`
	// ExemptSessions is a list of session name prefixes that should never be
	// targeted for stuck detection, recovery, or escalation. Sessions whose
	// name starts with any prefix (case-sensitive) are skipped entirely.
	// Default: ["human", "orchestrator", "orchestrator-v2", "meta-orchestrator"]
	ExemptSessions []string `yaml:"exempt_sessions"`
}

// DefaultExemptSessions returns the default list of exempt session prefixes.
// These sessions have long inference periods and should never be interrupted.
var DefaultExemptSessions = []string{"human", "orchestrator", "orchestrator-v2", "meta-orchestrator"}

// IsSessionExempt checks if a session name matches any exempt prefix.
func (c *RecoveryConfig) IsSessionExempt(sessionName string) bool {
	for _, prefix := range c.ExemptSessions {
		if strings.HasPrefix(sessionName, prefix) {
			return true
		}
	}
	return false
}

// LoggingConfig configures incident logging.
type LoggingConfig struct {
	// IncidentsFile is path to incidents.jsonl
	IncidentsFile string `yaml:"incidents_file" validate:"required"`
	// DiagnosesDir is path to diagnoses directory
	DiagnosesDir string `yaml:"diagnoses_dir" validate:"required"`
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

// LoopMonitoringConfig configures loop heartbeat monitoring.
type LoopMonitoringConfig struct {
	// EscalationCommand is the command to run after circuit breaker trips (3 failed wakes).
	// Receives JSON on stdin: {"session", "attempts", "last_heartbeat", "age"}
	EscalationCommand string `yaml:"escalation_command,omitempty"`
}

// EscalationConfig configures auto-escalation behavior.
type EscalationConfig struct {
	// Enabled controls whether escalation is active
	Enabled bool `yaml:"enabled"`
	// AutoApprove controls whether safe commands are auto-approved
	AutoApprove bool `yaml:"auto_approve"`
	// CooldownMinutes is the circuit breaker cooldown period
	CooldownMinutes int `yaml:"cooldown_minutes"`
	// MaxAutoApprovalsHour limits auto-approvals per session per hour
	MaxAutoApprovalsHour int `yaml:"max_auto_approvals_hour"`
	// AgmBinary is the path to the AGM binary
	AgmBinary string `yaml:"agm_binary"`
}

// DefaultConfig returns configuration with sensible defaults.
// Conservative defaults prefer missing interruptions over false positives.
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()

	return &Config{
		Patterns: PatternConfig{
			Bash:  filepath.Join(homeDir, "src/engram/patterns/bash-anti-patterns.yaml"),
			Beads: filepath.Join(homeDir, "src/engram/patterns/beads-anti-patterns.yaml"),
			Git:   filepath.Join(homeDir, "src/engram/patterns/git-anti-patterns.yaml"),
		},
		Violations: ViolationsConfig{
			Directory: filepath.Join(homeDir, "src/engram/violations"),
		},
		Monitoring: MonitoringConfig{
			Interval:               "60s", // Check every 60 seconds
			StuckThreshold:         "10m", // Consider stuck after 10 minutes
			IntervalDuration:       60 * time.Second,
			StuckThresholdDuration: 10 * time.Minute,
		},
		Tmux: TmuxConfig{
			Socket: "", // Empty = use tmux default socket detection
		},
		Recovery: RecoveryConfig{
			Enabled:               true,
			Strategy:              "escape", // Send Escape key (safest default)
			MaxAttempts:           3,
			DedupCooldown:         "15m",
			DedupCooldownDuration: 15 * time.Minute,
			ExemptSessions:        DefaultExemptSessions,
		},
		Logging: LoggingConfig{
			IncidentsFile: filepath.Join(homeDir, ".agm/logs/astrocyte/incidents.jsonl"),
			DiagnosesDir:  filepath.Join(homeDir, ".agm/logs/astrocyte/diagnoses"),
			Verbose:       false,
		},
		Escalation: EscalationConfig{
			Enabled:              true,
			AutoApprove:          true,
			CooldownMinutes:      30,
			MaxAutoApprovalsHour: 5,
			AgmBinary:            "agm",
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

	if c.Recovery.DedupCooldown != "" {
		c.Recovery.DedupCooldownDuration, err = time.ParseDuration(c.Recovery.DedupCooldown)
		if err != nil {
			return fmt.Errorf("invalid recovery.dedup_cooldown: %w", err)
		}
	}

	return nil
}

// Validate checks that configuration is valid using go-playground/validator.
// Returns error if any required fields are missing or invalid.
//
//nolint:gocyclo // validation functions are inherently branchy
func (c *Config) Validate() error {
	validate := validator.New()

	// Run struct validation
	if err := validate.Struct(c); err != nil {
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			for _, e := range validationErrors {
				field := e.StructField()
				tag := e.Tag()

				// Provide user-friendly error messages
				switch {
				case field == "Bash":
					return fmt.Errorf("patterns.bash path is required")
				case field == "Beads":
					return fmt.Errorf("patterns.beads path is required")
				case field == "Git":
					return fmt.Errorf("patterns.git path is required")
				case field == "Directory":
					return fmt.Errorf("violations.directory is required")
				case field == "Interval":
					return fmt.Errorf("monitoring.interval is required")
				case field == "StuckThreshold":
					return fmt.Errorf("monitoring.stuck_threshold is required")
				case field == "IntervalDuration" && tag == "gt":
					return fmt.Errorf("monitoring.interval must be positive")
				case field == "StuckThresholdDuration" && tag == "gt":
					return fmt.Errorf("monitoring.stuck_threshold must be positive")
				case field == "Strategy" && tag == "required":
					return fmt.Errorf("recovery.strategy is required")
				case field == "Strategy" && tag == "oneof":
					return fmt.Errorf("invalid recovery.strategy: %v (must be escape, ctrl_c, restart, or manual)", e.Value())
				case field == "IncidentsFile":
					return fmt.Errorf("logging.incidents_file is required")
				case field == "DiagnosesDir":
					return fmt.Errorf("logging.diagnoses_dir is required")
				default:
					return fmt.Errorf("validation failed for %s: %s", field, tag)
				}
			}
		}
		return err
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
