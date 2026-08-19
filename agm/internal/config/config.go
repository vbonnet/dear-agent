package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v3"
)

const maxConfigBytes = 1 << 20

// Config represents agm configuration
type Config struct {
	SessionsDir string `yaml:"sessions_dir"`
	LogLevel    string `yaml:"log_level"`
	LogFile     string `yaml:"log_file"`

	// UISettings is named in Go but inline in the shared YAML document, keeping
	// the existing defaults/ui root namespaces in the strict schema.
	UISettings UISettings `yaml:",inline" json:"-"`

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

	// Budget enforcement configuration
	Budget BudgetConfig `yaml:"budget"`

	// runtimeAuthority is captured only by Load after validation succeeds. It is
	// intentionally excluded from YAML and cannot be manufactured by callers.
	runtimeAuthority RuntimeAuthority
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
	Enabled   bool         `yaml:"enabled"`
	ServerURL string       `yaml:"server_url"`
	Reconnect ReconnectCfg `yaml:"reconnect"`
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
	Provider   string            `yaml:"provider"`             // Provider type: "auto", "bubblewrap", "overlayfs", "gvisor", "apfs", "mock"
	Repos      []string          `yaml:"repos"`                // Repositories to include as lower dirs
	Secrets    map[string]string `yaml:"secrets,omitempty"`    // Secrets to inject into sandbox
	Onboarding OnboardingConfig  `yaml:"onboarding,omitempty"` // Onboarding CLAUDE.md injection

	// WritableDirs are host paths a sandboxed session may write in addition to
	// its workspace, surfaced to the harness as --add-dir entries. Unlike Repos
	// these are NOT reflinked into the sandbox as lower dirs: they stay the real
	// host paths, which is what lets a worker commit to a real worktree or close
	// a bead in the real Beads DB. Empty by default.
	WritableDirs []string `yaml:"writable_dirs,omitempty"`

	// BypassCodexHookTrustReason requests the audited Codex hook-trust override
	// and states why. It is a reason rather than a bool on purpose: a bool is
	// exactly the switch an unattended agent flips and nobody reviews.
	//
	// Codex persists hook trust keyed by the ABSOLUTE path of hooks.json. A
	// sandboxed session runs from a fresh per-session workspace, so the hooks
	// reflinked into it always present at a never-before-seen path and Codex
	// blocks startup on "Hooks need review" — every time, unrecoverably, because
	// the path is different on the next spawn too.
	//
	// Two independent controls both have to pass, because they answer different
	// questions. Attestation asks whether the hooks are the reviewed ones:
	// enabling this requires an explicit Repos entry for the reviewed golden
	// checkout, and AGM pins the source commit, reads hooks.json and every
	// project-referenced hook from immutable Git objects, verifies their
	// SHA-256 digest against the sandbox copy, and materializes those exact
	// objects in a content-addressed, read-only host directory outside every
	// agent-writable root. Bypassed sessions execute project hooks only from
	// that immutable root for their full lifetime. AGM repeats verification
	// immediately before each launch and cold resume. The reviewed checkout is
	// never forwarded as a writable Codex add-dir. Any missing, uncommitted,
	// symlinked, changed, writable, or overlapping asset fails closed.
	//
	// Governance asks whether anyone agreed to run them unreviewed. Setting this
	// is a request, not a grant: the launch still refuses unless a human has
	// approved override kind "codex-hook-trust" through an interactive terminal
	// into root-owned storage (`agm override approve`), and every authorized launch is recorded to the
	// override ledger. See pkg/override.
	//
	// It is a reason rather than a bool on purpose: a bool is exactly the switch
	// an unattended agent flips and nobody reviews. Attested hooks are still
	// hooks running without per-path review, so this does not honour per-hook
	// "enabled = false" decisions recorded against the golden path. Empty by
	// default.
	BypassCodexHookTrustReason string `yaml:"bypass_codex_hook_trust_reason,omitempty"`
}

// OnboardingConfig controls CLAUDE.md injection into sandboxed sessions
type OnboardingConfig struct {
	Enabled      bool   `yaml:"enabled"`                 // Default: true - inject CLAUDE.md with worktree instructions
	TemplatePath string `yaml:"template_path,omitempty"` // Optional path to custom template file
}

// BudgetConfig holds token and cost budget enforcement settings.
type BudgetConfig struct {
	// Enabled controls whether budget enforcement is active. Default: false.
	// Set to true after configuring WeeklyLimitUSD.
	Enabled bool `yaml:"enabled"`

	// WeeklyLimitUSD is the total USD spend allowed per 7-day week (starting Monday).
	// The daily budget is computed via smoothed carryover:
	//   day_budget = (weekly_limit / 7) + carryover_from_prior_days
	WeeklyLimitUSD float64 `yaml:"weekly_limit_usd"`

	// StateFile is where the budget ledger is persisted across restarts.
	// Default: ~/.agm/budget-state.json
	StateFile string `yaml:"state_file,omitempty"`

	// SessionTokenCaps maps harness name → max tokens per individual session.
	// Keys: "claude-code", "codex-cli", "agy", "opencode-cli", "pi-cli",
	// plus deprecated "gemini-cli" compatibility.
	// 0 means no per-session cap for that harness.
	SessionTokenCaps map[string]int64 `yaml:"session_token_caps,omitempty"`

	// FallbackChain is the ordered list of model identifiers to try when the
	// primary model's budget is exhausted: e.g. ["sonnet", "haiku"].
	// An empty list means no fallback — requests fail immediately on exhaustion.
	FallbackChain []string `yaml:"fallback_chain,omitempty"`
}

// Default returns default configuration
func Default() *Config {
	homeDir, _ := os.UserHomeDir()
	return defaultWithHome(homeDir)
}

func defaultWithHome(homeDir string) *Config {
	uid := os.Getuid()
	return &Config{
		SessionsDir: filepath.Join(homeDir, ".claude", "sessions"),
		LogLevel:    "info",
		LogFile:     "",
		UISettings:  DefaultUISettings(),
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
				"pi-cli":       "π",
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
		Budget: BudgetConfig{
			Enabled:        false, // Opt-in — must configure WeeklyLimitUSD first
			WeeklyLimitUSD: 0,
			StateFile:      filepath.Join(homeDir, ".agm", "budget-state.json"),
			SessionTokenCaps: map[string]int64{
				"claude-code":  0, // 0 = no cap
				"codex-cli":    0,
				"gemini-cli":   0,
				"opencode-cli": 0,
				"pi-cli":       0,
			},
			FallbackChain: []string{"sonnet", "haiku"},
		},
	}
}

// Load applies defaults < one selected file < declared environment overrides.
// Command flags are a separate caller-owned layer applied by loadConfigWithFlags.
func Load(cfgFile string) (*Config, error) {
	homeDir, err := resolveConfigHome()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve home directory for AGM configuration: %w", err)
	}
	cfg := defaultWithHome(homeDir)

	// Only the absent canonical default path selects defaults. A caller that
	// names a file selected that exact authority source, so its absence is an
	// error rather than permission to discard its restrictions.
	explicitPath := cfgFile != ""
	if !explicitPath {
		cfgFile = filepath.Join(homeDir, ".config", "agm", "config.yaml")
	}

	data, err := readConfigFile(cfgFile)
	if err != nil {
		if explicitPath || !errors.Is(err, os.ErrNotExist) || !configPathAbsent(cfgFile) {
			return nil, fmt.Errorf("failed to read config file %q: %w", cfgFile, err)
		}
	} else if err := decodeConfig(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %q: %w", cfgFile, err)
	} else {
		normalizeUISettings(&cfg.UISettings)
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

	if err := validateConfiguredPathSpelling(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	// Expand home directory in paths using the single physical HOME retained
	// for the selected snapshot.
	if cfg.SessionsDir != "" {
		cfg.SessionsDir = expandHomeAt(cfg.SessionsDir, homeDir)
	}
	if cfg.LogFile != "" {
		cfg.LogFile = expandHomeAt(cfg.LogFile, homeDir)
	}
	if cfg.Lock.Path != "" {
		cfg.Lock.Path = expandHomeAt(cfg.Lock.Path, homeDir)
	}
	for i, dir := range cfg.Sandbox.Repos {
		cfg.Sandbox.Repos[i] = expandHomeAt(dir, homeDir)
	}
	for i, dir := range cfg.Sandbox.WritableDirs {
		cfg.Sandbox.WritableDirs[i] = expandHomeAt(dir, homeDir)
	}

	// Refuse a sandbox root that is the home directory itself. Expansion turns
	// a bare "~" into an absolute path that passes every remaining check, and
	// resolveSandboxLowerDirs returns configured repos directly — it never
	// reaches the $HOME refusal that guards only the scan fallback. A provider
	// such as OverlayFS would then publish the entire home directory as a
	// readable lower layer, exposing credentials far outside the requested
	// repository.
	if err := rejectUnsafeSandboxRoots(cfg.Sandbox.Repos, homeDir, "sandbox.repos"); err != nil {
		return nil, err
	}
	if err := rejectUnsafeSandboxRoots(cfg.Sandbox.WritableDirs, homeDir, "sandbox.writable_dirs"); err != nil {
		return nil, err
	}

	// Validate configuration
	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	authority, err := captureRuntimeAuthority(cfg, homeDir)
	if err != nil {
		return nil, fmt.Errorf("config runtime authority failed: %w", err)
	}
	cfg.runtimeAuthority = authority

	return cfg, nil
}

func resolveConfigHome() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(homeDir) {
		return "", fmt.Errorf("path %q is not absolute", homeDir)
	}
	resolved, err := filepath.EvalSymlinks(homeDir)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", homeDir, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat %q: %w", resolved, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path %q is not a directory", resolved)
	}
	return resolved, nil
}

// readConfigFile authenticates and snapshots one bounded regular-file source.
// Nonblocking open prevents FIFOs from hanging command initialization.
func readConfigFile(path string) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("open config source")
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("configuration source must be a regular file (mode %s)", info.Mode())
	}
	return readBoundedConfig(file, info.Size())
}

func readBoundedConfig(reader io.Reader, declaredSize int64) ([]byte, error) {
	if declaredSize < 0 {
		return nil, errors.New("configuration source reported a negative size")
	}
	if declaredSize > maxConfigBytes {
		return nil, fmt.Errorf("configuration source exceeds %d bytes", maxConfigBytes)
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxConfigBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxConfigBytes {
		return nil, fmt.Errorf("configuration source exceeds %d bytes", maxConfigBytes)
	}
	if int64(len(data)) != declaredSize {
		return nil, fmt.Errorf(
			"configuration source changed while reading: observed %d bytes, expected %d",
			len(data), declaredSize,
		)
	}
	return data, nil
}

// configPathAbsent distinguishes an ordinary missing path from ENOENT caused
// by a dangling symlink in any existing path component.
func configPathAbsent(path string) bool {
	volume := filepath.VolumeName(path)
	remainder := strings.TrimPrefix(path, volume)
	current := "."
	if filepath.IsAbs(path) {
		current = volume + string(os.PathSeparator)
		remainder = strings.TrimLeft(filepath.ToSlash(remainder), "/")
	} else {
		remainder = filepath.ToSlash(remainder)
	}

	for component := range strings.SplitSeq(remainder, "/") {
		if component == "" || component == "." {
			continue
		}
		if !strings.HasSuffix(current, string(os.PathSeparator)) {
			current += string(os.PathSeparator)
		}
		current += filepath.FromSlash(component)
		info, err := os.Lstat(current)
		if err != nil {
			return errors.Is(err, os.ErrNotExist)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if _, err := os.Stat(current); err != nil {
				return false
			}
		}
	}
	return false
}

// decodeConfig applies exactly one canonical mapping document to established
// defaults, authenticates sandbox authority representation, then performs one
// KnownFields decode across the complete runtime/UI union.
func decodeConfig(data []byte, cfg *Config) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("configuration file is empty")
		}
		return err
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 ||
		document.Content[0].Kind != yaml.MappingNode || document.Content[0].Tag != "!!map" {
		return errors.New("configuration document root must be a canonical YAML mapping")
	}
	if err := validateSandboxAuthorityShape(&document); err != nil {
		return err
	}

	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err == nil {
		return errors.New("multiple YAML documents are not allowed")
	} else if !errors.Is(err, io.EOF) {
		return err
	}

	strict := yaml.NewDecoder(bytes.NewReader(data))
	strict.KnownFields(true)
	return strict.Decode(cfg)
}

func validateSandboxAuthorityShape(document *yaml.Node) error {
	var root struct {
		Sandbox yaml.Node `yaml:"sandbox"`
	}
	if err := document.Decode(&root); err != nil {
		return err
	}
	sandbox := dereferenceAlias(&root.Sandbox)
	if sandbox.Kind == 0 {
		return nil
	}
	if sandbox.Kind != yaml.MappingNode || sandbox.Tag != "!!map" {
		return errors.New("sandbox must be a canonical mapping")
	}

	var authority struct {
		Enabled      yaml.Node `yaml:"enabled"`
		Provider     yaml.Node `yaml:"provider"`
		Repos        yaml.Node `yaml:"repos"`
		WritableDirs yaml.Node `yaml:"writable_dirs"`
	}
	if err := sandbox.Decode(&authority); err != nil {
		return err
	}
	if err := validateCanonicalAuthorityBool(&authority.Enabled, "sandbox.enabled"); err != nil {
		return err
	}
	if err := validateCanonicalAuthorityString(&authority.Provider, "sandbox.provider"); err != nil {
		return err
	}
	if err := validateCanonicalAuthoritySequence(&authority.Repos, "sandbox.repos"); err != nil {
		return err
	}
	return validateCanonicalAuthoritySequence(&authority.WritableDirs, "sandbox.writable_dirs")
}

func validateCanonicalAuthoritySequence(node *yaml.Node, field string) error {
	sequence := dereferenceAlias(node)
	if sequence.Kind != 0 && (sequence.Kind != yaml.SequenceNode || sequence.Tag != "!!seq") {
		return fmt.Errorf("%s must be a sequence", field)
	}
	for i, item := range sequence.Content {
		item = dereferenceAlias(item)
		if item.Kind != yaml.ScalarNode || item.Tag != "!!str" || strings.TrimSpace(item.Value) == "" {
			return fmt.Errorf("%s[%d] must be a non-empty canonical string", field, i)
		}
	}
	return nil
}

func validateCanonicalAuthorityString(node *yaml.Node, field string) error {
	value := dereferenceAlias(node)
	if value.Kind == 0 {
		return nil
	}
	if value.Kind != yaml.ScalarNode || value.Tag != "!!str" || strings.TrimSpace(value.Value) == "" {
		return fmt.Errorf("%s must be a non-empty canonical string", field)
	}
	return nil
}

func validateCanonicalAuthorityBool(node *yaml.Node, field string) error {
	value := dereferenceAlias(node)
	if value.Kind == 0 {
		return nil
	}
	if value.Kind != yaml.ScalarNode || value.Tag != "!!bool" ||
		(value.Value != "true" && value.Value != "false") {
		return fmt.Errorf("%s must be a canonical true or false boolean", field)
	}
	return nil
}

func dereferenceAlias(node *yaml.Node) *yaml.Node {
	seen := make(map[*yaml.Node]struct{})
	for node != nil && node.Kind == yaml.AliasNode {
		if _, ok := seen[node]; ok {
			return &yaml.Node{}
		}
		seen[node] = struct{}{}
		node = node.Alias
	}
	if node == nil {
		return &yaml.Node{}
	}
	return node
}

// validate performs configuration validation
func validate(cfg *Config) error {
	for i, dir := range cfg.Sandbox.Repos {
		if !filepath.IsAbs(dir) {
			return fmt.Errorf("sandbox.repos[%d] must be absolute after home expansion", i)
		}
	}
	for i, dir := range cfg.Sandbox.WritableDirs {
		if !filepath.IsAbs(dir) {
			return fmt.Errorf("sandbox.writable_dirs[%d] must be absolute after home expansion", i)
		}
	}

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
func validateConfiguredPathSpelling(cfg *Config) error {
	for _, authority := range []struct {
		field string
		paths []string
	}{
		{field: "sandbox.repos", paths: cfg.Sandbox.Repos},
		{field: "sandbox.writable_dirs", paths: cfg.Sandbox.WritableDirs},
	} {
		for i, path := range authority.paths {
			if containsDotPathComponent(path) {
				return fmt.Errorf("%s[%d] must not contain . or .. path components", authority.field, i)
			}
		}
	}
	return nil
}

func containsDotPathComponent(path string) bool {
	volume := filepath.VolumeName(path)
	remainder := strings.TrimPrefix(path, volume)
	for component := range strings.SplitSeq(filepath.ToSlash(remainder), "/") {
		if component == "." || component == ".." {
			return true
		}
	}
	return false
}

// expandHome expands ~ to home directory
func expandHome(path string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return expandHomeAt(path, homeDir)
}

func expandHomeAt(path, homeDir string) string {
	if len(path) == 0 || path[0] != '~' {
		return path
	}

	if len(path) == 1 {
		return homeDir
	}
	if !os.IsPathSeparator(path[1]) {
		return path
	}

	return filepath.Join(homeDir, path[2:])
}

// rejectUnsafeSandboxRoots refuses sandbox roots that are too broad to clone:
// the home directory itself, the filesystem root, or an empty entry. Each is
// checked both literally and through symlinks, so "~", "$HOME", and a symlink
// pointing at either are all refused.
func rejectUnsafeSandboxRoots(dirs []string, homeDir, field string) error {
	resolvedHome := homeDir
	if r, err := filepath.EvalSymlinks(homeDir); err == nil {
		resolvedHome = r
	}
	for _, dir := range dirs {
		if strings.TrimSpace(dir) == "" {
			return fmt.Errorf("%s contains an empty entry", field)
		}
		candidates := []string{filepath.Clean(dir)}
		if r, err := filepath.EvalSymlinks(dir); err == nil {
			candidates = append(candidates, filepath.Clean(r))
		}
		for _, candidate := range candidates {
			if candidate == "/" {
				return fmt.Errorf("%s entry %q is the filesystem root; name the repository directory instead", field, dir)
			}
			if homeDir != "" && (candidate == filepath.Clean(homeDir) || candidate == filepath.Clean(resolvedHome)) {
				return fmt.Errorf("%s entry %q resolves to the home directory; a sandbox lower layer must be a repository, "+
					"not $HOME — every file under it, including credentials, would be readable inside the sandbox", field, dir)
			}
		}
	}
	return nil
}
