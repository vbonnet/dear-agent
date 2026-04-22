package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents agm configuration
type Config struct {
	SessionsDir string `yaml:"sessions_dir"`
	LogLevel    string `yaml:"log_level"`
	LogFile     string `yaml:"log_file"`

	// Workspace configuration
	Workspace           string `yaml:"workspace,omitempty"`        // Explicit workspace or auto-detect
	WorkspaceConfigPath string `yaml:"workspace_config,omitempty"` // Path to workspace config (default: ~/.agm/config.yaml)

	// Storage configuration (centralized component storage support)
	Storage StorageConfig `yaml:"storage"`

	// Resilience features
	Timeout     TimeoutConfig     `yaml:"timeout"`
	Lock        LockConfig        `yaml:"lock"`
	HealthCheck HealthCheckConfig `yaml:"health_check"`

	// Multi-agent adapters
	Adapters AdaptersConfig `yaml:"adapters"`

	// Auto-resume configuration
	AutoResume AutoResumeConfig `yaml:"auto_resume"`

	// Status line configuration
	StatusLine StatusLineConfig `yaml:"status_line"`

	// Sandbox configuration
	Sandbox SandboxConfig `yaml:"sandbox"`
}

// StorageConfig holds centralized storage configuration
type StorageConfig struct {
	Mode         string            `yaml:"mode"`          // "dotfile" (default) or "centralized"
	Workspace    string            `yaml:"workspace"`     // Workspace name or absolute path (for centralized mode)
	RelativePath string            `yaml:"relative_path"` // Path within workspace (default: ".agm")
	Dolt         DoltStorageConfig `yaml:"dolt"`          // Dolt-specific configuration
}

// DoltStorageConfig holds Dolt-specific storage configuration
type DoltStorageConfig struct {
	StartScript string `yaml:"start_script"` // Path to auto-start script for Dolt server
}

// TimeoutConfig holds timeout configuration
type TimeoutConfig struct {
	TmuxCommands time.Duration `yaml:"tmux_commands"` // Default: 5s
	Enabled      bool          `yaml:"enabled"`       // Default: true
}

// LockConfig holds lock configuration
type LockConfig struct {
	Enabled bool   `yaml:"enabled"` // Default: true
	Path    string `yaml:"path"`    // Default: /tmp/agm-{UID}/agm.lock
}

// HealthCheckConfig holds health check configuration
type HealthCheckConfig struct {
	Enabled       bool          `yaml:"enabled"`        // Default: true
	CacheDuration time.Duration `yaml:"cache_duration"` // Default: 5s
	ProbeTimeout  time.Duration `yaml:"probe_timeout"`  // Default: 2s
}

// AdaptersConfig holds configuration for multi-agent adapters
type AdaptersConfig struct {
	OpenCode    OpenCodeConfig    `yaml:"opencode"`
	ClaudeHooks ClaudeHooksConfig `yaml:"claude_hooks"`
	GeminiHooks GeminiHooksConfig `yaml:"gemini_hooks"`
}

// OpenCodeConfig holds configuration for OpenCode SSE adapter
type OpenCodeConfig struct {
	Enabled      bool         `yaml:"enabled"`
	ServerURL    string       `yaml:"server_url"`
	Reconnect    ReconnectCfg `yaml:"reconnect"`
	FallbackTmux bool         `yaml:"fallback_to_tmux"`
}

// ReconnectCfg holds reconnection configuration for SSE adapter
type ReconnectCfg struct {
	InitialDelay time.Duration `yaml:"initial_delay"`
	MaxDelay     time.Duration `yaml:"max_delay"`
	Multiplier   int           `yaml:"multiplier"`
}

// ClaudeHooksConfig holds configuration for Claude webhook adapter
type ClaudeHooksConfig struct {
	Enabled    bool   `yaml:"enabled"`
	ListenAddr string `yaml:"listen_addr"`
}

// GeminiHooksConfig holds configuration for Gemini hook adapter
type GeminiHooksConfig struct {
	Enabled    bool   `yaml:"enabled"`
	SocketPath string `yaml:"socket_path"`
}

// AutoResumeConfig holds configuration for automatic session resumption on boot
type AutoResumeConfig struct {
	Enabled         bool   `yaml:"enabled"`          // Default: false (opt-in)
	IncludeArchived bool   `yaml:"include_archived"` // Default: false
	WorkspaceFilter string `yaml:"workspace_filter"` // Default: "" (all workspaces)
	DelaySeconds    int    `yaml:"delay_seconds"`    // Default: 5 (wait after boot)
}

// StatusLineConfig holds configuration for tmux status line integration
type StatusLineConfig struct {
	Enabled          bool              `yaml:"enabled"`            // Default: true
	DefaultFormat    string            `yaml:"default_format"`     // Template string for status line
	RefreshInterval  int               `yaml:"refresh_interval"`   // Refresh interval in seconds (default: 10)
	ShowContextUsage bool              `yaml:"show_context_usage"` // Show context usage percentage (default: true)
	ShowGitStatus    bool              `yaml:"show_git_status"`    // Show git branch and uncommitted count (default: true)
	HarnessIcons     map[string]string `yaml:"harness_icons"`      // Custom icons for harness types
	CustomFormats    map[string]string `yaml:"custom_formats"`     // Named template presets (minimal, compact, etc.)
}

// SandboxConfig holds configuration for sandbox isolation
type SandboxConfig struct {
	Enabled    bool              `yaml:"enabled"`              // Default: true (sandbox-by-default)
	Provider   string            `yaml:"provider"`             // Provider type: "auto", "overlayfs", "apfs", "claudecode-worktree", "mock"
	Repos      []string          `yaml:"repos"`                // Repositories to include as lower dirs
	Secrets    map[string]string `yaml:"secrets,omitempty"`    // Secrets to inject into sandbox
	Onboarding OnboardingConfig  `yaml:"onboarding,omitempty"` // Onboarding CLAUDE.md injection
}

// OnboardingConfig controls CLAUDE.md injection into sandboxed sessions
type OnboardingConfig struct {
	Enabled      bool   `yaml:"enabled"`                 // Default: true - inject CLAUDE.md with worktree instructions
	TemplatePath string `yaml:"template_path,omitempty"` // Optional path to custom template file
}

// Default returns default configuration
func Default() *Config {
	homeDir, _ := os.UserHomeDir()
	uid := os.Getuid()
	return &Config{
		SessionsDir: filepath.Join(homeDir, ".claude", "sessions"),
		LogLevel:    "info",
		LogFile:     "",
		Storage: StorageConfig{
			Mode:         "dotfile", // Default mode for backward compatibility
			Workspace:    "",        // Empty = use mode: dotfile
			RelativePath: ".agm",    // Default path within workspace
		},
		Timeout: TimeoutConfig{
			TmuxCommands: 5 * time.Second,
			Enabled:      true,
		},
		Lock: LockConfig{
			Enabled: true,
			Path:    fmt.Sprintf("/tmp/agm-%d/agm.lock", uid),
		},
		HealthCheck: HealthCheckConfig{
			Enabled:       true,
			CacheDuration: 5 * time.Second,
			ProbeTimeout:  2 * time.Second,
		},
		Adapters: AdaptersConfig{
			OpenCode: OpenCodeConfig{
				Enabled:   false, // Opt-in
				ServerURL: "http://localhost:4096",
				Reconnect: ReconnectCfg{
					InitialDelay: 1 * time.Second,
					MaxDelay:     30 * time.Second,
					Multiplier:   2,
				},
				FallbackTmux: true,
			},
			ClaudeHooks: ClaudeHooksConfig{
				Enabled:    false,
				ListenAddr: "127.0.0.1:14321",
			},
			GeminiHooks: GeminiHooksConfig{
				Enabled:    false,
				SocketPath: "/tmp/agm-gemini-hook.sock",
			},
		},
		AutoResume: AutoResumeConfig{
			Enabled:         false, // Opt-in (disabled by default)
			IncludeArchived: false,
			WorkspaceFilter: "",
			DelaySeconds:    5,
		},
		StatusLine: StatusLineConfig{
			Enabled:          true,
			DefaultFormat:    "", // Empty = use statusline.DefaultTemplate()
			RefreshInterval:  10,
			ShowContextUsage: true,
			ShowGitStatus:    true,
			HarnessIcons: map[string]string{
				"claude-code":  "🤖",
				"gemini-cli":   "✨",
				"codex-cli":    "🧠",
				"opencode-cli": "💻",
			},
			CustomFormats: map[string]string{
				"minimal":     "{{.AgentIcon}} {{.State}} | {{if ge .ContextPercent 0.0}}{{printf \"%.0f\" .ContextPercent}}%{{else}}--{{end}}",
				"compact":     "{{.AgentIcon}} #[fg={{.StateColor}}]●#[default] {{if ge .ContextPercent 0.0}}{{printf \"%.0f\" .ContextPercent}}%{{else}}--{{end}} | {{.Branch}}",
				"multi-agent": "{{.AgentIcon}}{{.AgentType}} | #[fg={{.StateColor}}]{{.State}}#[default] | {{if ge .ContextPercent 0.0}}{{printf \"%.0f\" .ContextPercent}}%{{else}}--{{end}}",
				"full":        "{{.AgentIcon}} #[fg={{.StateColor}}]{{.State}}#[default] | CTX:#[fg={{.ContextColor}}]{{if ge .ContextPercent 0.0}}{{printf \"%.0f\" .ContextPercent}}%{{else}}--{{end}}#[default] | {{.Branch}}{{if gt .Uncommitted 0}}(+{{.Uncommitted}}){{end}} | {{.SessionName}}",
			},
		},
		Sandbox: SandboxConfig{
			Enabled:  true,   // Sandbox-by-default (use --no-sandbox to disable)
			Provider: "auto", // Auto-detect provider
			Repos:    []string{},
			Secrets:  make(map[string]string),
			Onboarding: OnboardingConfig{
				Enabled: true, // Inject CLAUDE.md with worktree instructions by default
			},
		},
	}
}

// Load loads configuration with precedence: defaults < file < env < flags
func Load(cfgFile string) (*Config, error) {
	cfg := Default()

	// Load from config file if exists
	if cfgFile == "" {
		homeDir, _ := os.UserHomeDir()
		cfgFile = filepath.Join(homeDir, ".config", "agm", "config.yaml")
	}

	if data, err := os.ReadFile(cfgFile); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	// Override with environment variables
	if dir := os.Getenv("AGM_SESSIONS_DIR"); dir != "" {
		cfg.SessionsDir = dir
	}
	if level := os.Getenv("AGM_LOG_LEVEL"); level != "" {
		cfg.LogLevel = level
	}
	if file := os.Getenv("AGM_LOG_FILE"); file != "" {
		cfg.LogFile = file
	}

	// OpenCode adapter environment overrides
	if url := os.Getenv("OPENCODE_SERVER_URL"); url != "" {
		cfg.Adapters.OpenCode.ServerURL = url
	}
	if enabled := os.Getenv("OPENCODE_ADAPTER_ENABLED"); enabled != "" {
		cfg.Adapters.OpenCode.Enabled = enabled == "true" || enabled == "1"
	}

	// Expand home directory in paths
	if cfg.SessionsDir != "" {
		cfg.SessionsDir = expandHome(cfg.SessionsDir)
	}
	if cfg.LogFile != "" {
		cfg.LogFile = expandHome(cfg.LogFile)
	}
	if cfg.Lock.Path != "" {
		cfg.Lock.Path = expandHome(cfg.Lock.Path)
	}

	// Validate configuration
	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// validate performs configuration validation
func validate(cfg *Config) error {
	// Validate OpenCode adapter configuration
	if cfg.Adapters.OpenCode.Enabled {
		if cfg.Adapters.OpenCode.ServerURL == "" {
			return fmt.Errorf("adapters.opencode.server_url is required when enabled")
		}

		// Validate reconnect configuration
		if cfg.Adapters.OpenCode.Reconnect.InitialDelay <= 0 {
			return fmt.Errorf("adapters.opencode.reconnect.initial_delay must be > 0")
		}
		if cfg.Adapters.OpenCode.Reconnect.MaxDelay <= 0 {
			return fmt.Errorf("adapters.opencode.reconnect.max_delay must be > 0")
		}
		if cfg.Adapters.OpenCode.Reconnect.MaxDelay < cfg.Adapters.OpenCode.Reconnect.InitialDelay {
			return fmt.Errorf("adapters.opencode.reconnect.max_delay must be >= initial_delay")
		}
		if cfg.Adapters.OpenCode.Reconnect.Multiplier < 1 {
			return fmt.Errorf("adapters.opencode.reconnect.multiplier must be >= 1")
		}
	}

	return nil
}

// expandHome expands ~ to home directory
func expandHome(path string) string {
	if len(path) == 0 || path[0] != '~' {
		return path
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	if len(path) == 1 {
		return homeDir
	}

	return filepath.Join(homeDir, path[2:])
}
