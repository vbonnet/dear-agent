package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestDefaultPaths verifies that default configuration paths are constructed correctly
func TestDefaultPaths(t *testing.T) {
	paths := DefaultPaths()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}

	engramRoot := filepath.Join(homeDir, ".engram")

	tests := []struct {
		tier     ConfigTier
		expected string
	}{
		{TierCore, filepath.Join(engramRoot, "core", "config.yaml")},
		{TierCompany, filepath.Join(engramRoot, "company", "config.yaml")},
		{TierTeam, filepath.Join(engramRoot, "team", "config.yaml")},
		{TierUser, filepath.Join(engramRoot, "user", "config.yaml")},
	}

	for _, tt := range tests {
		t.Run(string(tt.tier), func(t *testing.T) {
			if got := paths[tt.tier]; got != tt.expected {
				t.Errorf("DefaultPaths()[%s] = %q, want %q", tt.tier, got, tt.expected)
			}
		})
	}
}

// TestNewLoader verifies that NewLoader creates a loader with default paths
func TestNewLoader(t *testing.T) {
	loader := NewLoader()

	if loader == nil {
		t.Fatal("NewLoader() returned nil")
	}

	if loader.paths == nil {
		t.Fatal("NewLoader() created loader with nil paths")
	}

	// Verify all tiers have paths
	tiers := []ConfigTier{TierCore, TierCompany, TierTeam, TierUser}
	for _, tier := range tiers {
		if _, ok := loader.paths[tier]; !ok {
			t.Errorf("NewLoader() missing path for tier %s", tier)
		}
	}
}

// TestLoad_CoreOnly verifies loading when only core config exists
func TestLoad_CoreOnly(t *testing.T) {
	// Create temporary directory for test configs
	tmpDir := t.TempDir()

	// Create core config
	coreDir := filepath.Join(tmpDir, "core")
	if err := os.MkdirAll(coreDir, 0755); err != nil {
		t.Fatalf("failed to create core dir: %v", err)
	}

	coreConfig := `platform:
  agent: "claude-code"
  engram_path: "engrams"
  token_budget: 50000
plugins:
  paths:
    - "./plugins"
    - "~/.engram/core/plugins"
telemetry:
  enabled: true
  path: "~/.engram/telemetry/events.jsonl"
`
	coreConfigPath := filepath.Join(coreDir, "config.yaml")
	if err := os.WriteFile(coreConfigPath, []byte(coreConfig), 0644); err != nil {
		t.Fatalf("failed to write core config: %v", err)
	}

	// Create loader with custom paths
	loader := &Loader{
		paths: map[ConfigTier]string{
			TierCore:    coreConfigPath,
			TierCompany: filepath.Join(tmpDir, "company", "config.yaml"),
			TierTeam:    filepath.Join(tmpDir, "team", "config.yaml"),
			TierUser:    filepath.Join(tmpDir, "user", "config.yaml"),
		},
		cachedMtimes: make(map[ConfigTier]time.Time),
	}

	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify loaded values
	if cfg.Platform.Agent != "claude-code" {
		t.Errorf("Platform.Agent = %q, want %q", cfg.Platform.Agent, "claude-code")
	}
	if cfg.Platform.EngramPath != "engrams" {
		t.Errorf("Platform.EngramPath = %q, want %q", cfg.Platform.EngramPath, "engrams")
	}
	if cfg.Platform.TokenBudget != 50000 {
		t.Errorf("Platform.TokenBudget = %d, want %d", cfg.Platform.TokenBudget, 50000)
	}
	if !cfg.Telemetry.Enabled {
		t.Error("Telemetry.Enabled = false, want true")
	}
	if len(cfg.Plugins.Paths) != 2 {
		t.Errorf("len(Plugins.Paths) = %d, want 2", len(cfg.Plugins.Paths))
	}
}

// TestLoad_MultiTier verifies that higher tiers override lower tiers
func TestLoad_MultiTier(t *testing.T) {
	tmpDir := t.TempDir()

	// Core config (lowest priority)
	coreDir := filepath.Join(tmpDir, "core")
	if err := os.MkdirAll(coreDir, 0755); err != nil {
		t.Fatalf("failed to create core dir: %v", err)
	}
	coreConfig := `platform:
  agent: "unknown"
  token_budget: 50000
telemetry:
  enabled: true
`
	if err := os.WriteFile(filepath.Join(coreDir, "config.yaml"), []byte(coreConfig), 0644); err != nil {
		t.Fatalf("failed to write core config: %v", err)
	}

	// Company config (overrides agent)
	companyDir := filepath.Join(tmpDir, "company")
	if err := os.MkdirAll(companyDir, 0755); err != nil {
		t.Fatalf("failed to create company dir: %v", err)
	}
	companyConfig := `platform:
  agent: "cursor"
  token_budget: 75000
`
	if err := os.WriteFile(filepath.Join(companyDir, "config.yaml"), []byte(companyConfig), 0644); err != nil {
		t.Fatalf("failed to write company config: %v", err)
	}

	// User config (highest priority - overrides token budget)
	userDir := filepath.Join(tmpDir, "user")
	if err := os.MkdirAll(userDir, 0755); err != nil {
		t.Fatalf("failed to create user dir: %v", err)
	}
	userConfig := `platform:
  token_budget: 100000
telemetry:
  enabled: false
`
	if err := os.WriteFile(filepath.Join(userDir, "config.yaml"), []byte(userConfig), 0644); err != nil {
		t.Fatalf("failed to write user config: %v", err)
	}

	loader := &Loader{
		paths: map[ConfigTier]string{
			TierCore:    filepath.Join(coreDir, "config.yaml"),
			TierCompany: filepath.Join(companyDir, "config.yaml"),
			TierTeam:    filepath.Join(tmpDir, "team", "config.yaml"), // doesn't exist
			TierUser:    filepath.Join(userDir, "config.yaml"),
		},
		cachedMtimes: make(map[ConfigTier]time.Time),
	}

	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// User tier should win for token budget and telemetry
	if cfg.Platform.TokenBudget != 100000 {
		t.Errorf("Platform.TokenBudget = %d, want %d (user tier should override)", cfg.Platform.TokenBudget, 100000)
	}
	if cfg.Telemetry.Enabled {
		t.Error("Telemetry.Enabled = true, want false (user tier should override)")
	}

	// Company tier should win for agent
	if cfg.Platform.Agent != "cursor" {
		t.Errorf("Platform.Agent = %q, want %q (company tier should override core)", cfg.Platform.Agent, "cursor")
	}
}

// TestLoad_MissingCore verifies that missing core config returns error
func TestLoad_MissingCore(t *testing.T) {
	tmpDir := t.TempDir()

	loader := &Loader{
		paths: map[ConfigTier]string{
			TierCore:    filepath.Join(tmpDir, "core", "config.yaml"), // doesn't exist
			TierCompany: filepath.Join(tmpDir, "company", "config.yaml"),
			TierTeam:    filepath.Join(tmpDir, "team", "config.yaml"),
			TierUser:    filepath.Join(tmpDir, "user", "config.yaml"),
		},
		cachedMtimes: make(map[ConfigTier]time.Time),
	}

	_, err := loader.Load()
	if err == nil {
		t.Fatal("Load() succeeded with missing core config, want error")
	}
	// Error should be wrapped, but underlying cause should be NotExist
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Load() error should wrap os.ErrNotExist, got: %v", err)
	}
}

// TestLoad_InvalidYAML verifies that invalid YAML returns error
func TestLoad_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()

	coreDir := filepath.Join(tmpDir, "core")
	if err := os.MkdirAll(coreDir, 0755); err != nil {
		t.Fatalf("failed to create core dir: %v", err)
	}

	// Invalid YAML (unclosed quote)
	invalidYAML := `platform:
  agent: "unclosed
`
	coreConfigPath := filepath.Join(coreDir, "config.yaml")
	if err := os.WriteFile(coreConfigPath, []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("failed to write core config: %v", err)
	}

	loader := &Loader{
		paths: map[ConfigTier]string{
			TierCore:    coreConfigPath,
			TierCompany: filepath.Join(tmpDir, "company", "config.yaml"),
			TierTeam:    filepath.Join(tmpDir, "team", "config.yaml"),
			TierUser:    filepath.Join(tmpDir, "user", "config.yaml"),
		},
		cachedMtimes: make(map[ConfigTier]time.Time),
	}

	_, err := loader.Load()
	if err == nil {
		t.Fatal("Load() succeeded with invalid YAML, want error")
	}
}

// TestMerge_PluginPaths verifies that plugin paths are replaced, not appended
func TestMerge_PluginPaths(t *testing.T) {
	loader := &Loader{}

	dst := &Config{
		Plugins: PluginConfig{
			Paths: []string{"path1", "path2"},
		},
	}

	src := &Config{
		Plugins: PluginConfig{
			Paths: []string{"path3", "path4"},
		},
	}

	loader.merge(dst, src, TierUser)

	// Paths should be replaced, not appended
	if len(dst.Plugins.Paths) != 2 {
		t.Errorf("len(Plugins.Paths) = %d, want 2", len(dst.Plugins.Paths))
	}
	if dst.Plugins.Paths[0] != "path3" || dst.Plugins.Paths[1] != "path4" {
		t.Errorf("Plugins.Paths = %v, want [path3 path4]", dst.Plugins.Paths)
	}
}

// TestMerge_DisabledPlugins verifies that disabled plugins are appended
func TestMerge_DisabledPlugins(t *testing.T) {
	loader := &Loader{}

	dst := &Config{
		Plugins: PluginConfig{
			Disabled: []string{"plugin1"},
		},
	}

	src := &Config{
		Plugins: PluginConfig{
			Disabled: []string{"plugin2", "plugin3"},
		},
	}

	loader.merge(dst, src, TierUser)

	// Disabled should be appended
	if len(dst.Plugins.Disabled) != 3 {
		t.Errorf("len(Plugins.Disabled) = %d, want 3", len(dst.Plugins.Disabled))
	}
	expected := []string{"plugin1", "plugin2", "plugin3"}
	for i, want := range expected {
		if i >= len(dst.Plugins.Disabled) || dst.Plugins.Disabled[i] != want {
			t.Errorf("Plugins.Disabled[%d] = %q, want %q", i, dst.Plugins.Disabled[i], want)
		}
	}
}

// TestMerge_EmptyValues verifies that empty values don't override
func TestMerge_EmptyValues(t *testing.T) {
	loader := &Loader{}

	dst := &Config{
		Platform: PlatformConfig{
			Agent:       "cursor",
			EngramPath:  "engrams",
			TokenBudget: 50000,
		},
	}

	src := &Config{
		Platform: PlatformConfig{
			// Only override token budget, leave others empty
			TokenBudget: 75000,
		},
	}

	loader.merge(dst, src, TierUser)

	// Empty values should not override
	if dst.Platform.Agent != "cursor" {
		t.Errorf("Platform.Agent = %q, want %q (empty should not override)", dst.Platform.Agent, "cursor")
	}
	if dst.Platform.EngramPath != "engrams" {
		t.Errorf("Platform.EngramPath = %q, want %q (empty should not override)", dst.Platform.EngramPath, "engrams")
	}

	// Non-empty value should override
	if dst.Platform.TokenBudget != 75000 {
		t.Errorf("Platform.TokenBudget = %d, want %d", dst.Platform.TokenBudget, 75000)
	}
}

// TestCache_PopulationAndHit verifies cache is populated and used on subsequent calls
func TestCache_PopulationAndHit(t *testing.T) {
	tmpDir := t.TempDir()

	// Create core config
	coreDir := filepath.Join(tmpDir, "core")
	if err := os.MkdirAll(coreDir, 0755); err != nil {
		t.Fatalf("failed to create core dir: %v", err)
	}

	coreConfig := `platform:
  agent: "claude-code"
  token_budget: 50000
telemetry:
  enabled: true
`
	coreConfigPath := filepath.Join(coreDir, "config.yaml")
	if err := os.WriteFile(coreConfigPath, []byte(coreConfig), 0644); err != nil {
		t.Fatalf("failed to write core config: %v", err)
	}

	loader := &Loader{
		paths: map[ConfigTier]string{
			TierCore:    coreConfigPath,
			TierCompany: filepath.Join(tmpDir, "company", "config.yaml"),
			TierTeam:    filepath.Join(tmpDir, "team", "config.yaml"),
			TierUser:    filepath.Join(tmpDir, "user", "config.yaml"),
		},
		cachedMtimes: make(map[ConfigTier]time.Time),
	}

	// First Load() - should populate cache
	cfg1, err := loader.Load()
	if err != nil {
		t.Fatalf("First Load() failed: %v", err)
	}

	// Verify cache was populated
	if loader.cachedConfig == nil {
		t.Fatal("Cache not populated after first Load()")
	}
	if len(loader.cachedMtimes) == 0 {
		t.Fatal("Cache mtimes not populated after first Load()")
	}

	// Second Load() - should return cached config
	cfg2, err := loader.Load()
	if err != nil {
		t.Fatalf("Second Load() failed: %v", err)
	}

	// Verify both loads return same config values
	if cfg1.Platform.Agent != cfg2.Platform.Agent {
		t.Errorf("Config changed between loads: agent %q vs %q", cfg1.Platform.Agent, cfg2.Platform.Agent)
	}
	if cfg1.Platform.TokenBudget != cfg2.Platform.TokenBudget {
		t.Errorf("Config changed between loads: token budget %d vs %d", cfg1.Platform.TokenBudget, cfg2.Platform.TokenBudget)
	}
}

// TestCache_InvalidationOnFileChange verifies cache invalidates when file mtime changes
func TestCache_InvalidationOnFileChange(t *testing.T) {
	tmpDir := t.TempDir()

	// Create core config
	coreDir := filepath.Join(tmpDir, "core")
	if err := os.MkdirAll(coreDir, 0755); err != nil {
		t.Fatalf("failed to create core dir: %v", err)
	}

	coreConfig := `platform:
  token_budget: 50000
telemetry:
  enabled: true
`
	coreConfigPath := filepath.Join(coreDir, "config.yaml")
	if err := os.WriteFile(coreConfigPath, []byte(coreConfig), 0644); err != nil {
		t.Fatalf("failed to write core config: %v", err)
	}

	loader := &Loader{
		paths: map[ConfigTier]string{
			TierCore:    coreConfigPath,
			TierCompany: filepath.Join(tmpDir, "company", "config.yaml"),
			TierTeam:    filepath.Join(tmpDir, "team", "config.yaml"),
			TierUser:    filepath.Join(tmpDir, "user", "config.yaml"),
		},
		cachedMtimes: make(map[ConfigTier]time.Time),
	}

	// First Load()
	cfg1, err := loader.Load()
	if err != nil {
		t.Fatalf("First Load() failed: %v", err)
	}
	if cfg1.Platform.TokenBudget != 50000 {
		t.Errorf("First load: TokenBudget = %d, want 50000", cfg1.Platform.TokenBudget)
	}

	// Wait to ensure mtime changes
	time.Sleep(10 * time.Millisecond)

	// Modify config file
	modifiedConfig := `platform:
  token_budget: 75000
telemetry:
  enabled: true
`
	if err := os.WriteFile(coreConfigPath, []byte(modifiedConfig), 0644); err != nil {
		t.Fatalf("failed to modify core config: %v", err)
	}

	// Second Load() - should detect change and reload
	cfg2, err := loader.Load()
	if err != nil {
		t.Fatalf("Second Load() failed: %v", err)
	}

	// Verify new config was loaded
	if cfg2.Platform.TokenBudget != 75000 {
		t.Errorf("Second load: TokenBudget = %d, want 75000 (cache should have invalidated)", cfg2.Platform.TokenBudget)
	}
}

// TestCache_MissingOptionalFiles verifies cache handles missing optional files correctly
func TestCache_MissingOptionalFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create only core config (optional tiers missing)
	coreDir := filepath.Join(tmpDir, "core")
	if err := os.MkdirAll(coreDir, 0755); err != nil {
		t.Fatalf("failed to create core dir: %v", err)
	}

	coreConfig := `platform:
  token_budget: 50000
telemetry:
  enabled: true
`
	coreConfigPath := filepath.Join(coreDir, "config.yaml")
	if err := os.WriteFile(coreConfigPath, []byte(coreConfig), 0644); err != nil {
		t.Fatalf("failed to write core config: %v", err)
	}

	userConfigPath := filepath.Join(tmpDir, "user", "config.yaml")

	loader := &Loader{
		paths: map[ConfigTier]string{
			TierCore:    coreConfigPath,
			TierCompany: filepath.Join(tmpDir, "company", "config.yaml"),
			TierTeam:    filepath.Join(tmpDir, "team", "config.yaml"),
			TierUser:    userConfigPath,
		},
		cachedMtimes: make(map[ConfigTier]time.Time),
	}

	// First Load() with missing optional files
	cfg1, err := loader.Load()
	if err != nil {
		t.Fatalf("First Load() failed: %v", err)
	}
	if cfg1.Platform.TokenBudget != 50000 {
		t.Errorf("TokenBudget = %d, want 50000", cfg1.Platform.TokenBudget)
	}

	// Second Load() - should use cache
	cfg2, err := loader.Load()
	if err != nil {
		t.Fatalf("Second Load() failed: %v", err)
	}
	if cfg2.Platform.TokenBudget != 50000 {
		t.Errorf("TokenBudget = %d, want 50000", cfg2.Platform.TokenBudget)
	}

	// Now create user config
	userDir := filepath.Join(tmpDir, "user")
	if err := os.MkdirAll(userDir, 0755); err != nil {
		t.Fatalf("failed to create user dir: %v", err)
	}
	userConfig := `platform:
  token_budget: 75000
`
	if err := os.WriteFile(userConfigPath, []byte(userConfig), 0644); err != nil {
		t.Fatalf("failed to write user config: %v", err)
	}

	// Third Load() - should detect new file and reload
	cfg3, err := loader.Load()
	if err != nil {
		t.Fatalf("Third Load() failed: %v", err)
	}
	if cfg3.Platform.TokenBudget != 75000 {
		t.Errorf("TokenBudget = %d, want 75000 (cache should invalidate when optional file appears)", cfg3.Platform.TokenBudget)
	}
}

// TestCache_ThreadSafety verifies concurrent Load() calls are safe
func TestCache_ThreadSafety(t *testing.T) {
	tmpDir := t.TempDir()

	// Create core config
	coreDir := filepath.Join(tmpDir, "core")
	if err := os.MkdirAll(coreDir, 0755); err != nil {
		t.Fatalf("failed to create core dir: %v", err)
	}

	coreConfig := `platform:
  agent: "claude-code"
  token_budget: 50000
telemetry:
  enabled: true
`
	coreConfigPath := filepath.Join(coreDir, "config.yaml")
	if err := os.WriteFile(coreConfigPath, []byte(coreConfig), 0644); err != nil {
		t.Fatalf("failed to write core config: %v", err)
	}

	loader := &Loader{
		paths: map[ConfigTier]string{
			TierCore:    coreConfigPath,
			TierCompany: filepath.Join(tmpDir, "company", "config.yaml"),
			TierTeam:    filepath.Join(tmpDir, "team", "config.yaml"),
			TierUser:    filepath.Join(tmpDir, "user", "config.yaml"),
		},
		cachedMtimes: make(map[ConfigTier]time.Time),
	}

	// Launch multiple goroutines calling Load() concurrently
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg, err := loader.Load()
			if err != nil {
				errors <- err
				return
			}
			if cfg.Platform.Agent != "claude-code" {
				select {
				case errors <- fmt.Errorf("config value mismatch: got %q", cfg.Platform.Agent):
				default:
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Errorf("Concurrent Load() error: %v", err)
	}
}

// TestExpandPath verifies tilde expansion works correctly
func TestExpandPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "tilde only",
			input:    "~",
			expected: homeDir,
		},
		{
			name:     "tilde with path",
			input:    "~/.engram",
			expected: filepath.Join(homeDir, ".engram"),
		},
		{
			name:     "tilde with nested path",
			input:    "~/.engram/telemetry/events.jsonl",
			expected: filepath.Join(homeDir, ".engram/telemetry/events.jsonl"),
		},
		{
			name:     "no tilde - relative path",
			input:    "./config",
			expected: "./config",
		},
		{
			name:     "no tilde - absolute path",
			input:    "/etc/engram/config.yaml",
			expected: "/etc/engram/config.yaml",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := expandPath(tt.input)
			if err != nil {
				t.Fatalf("expandPath(%q) returned error: %v", tt.input, err)
			}
			if result != tt.expected {
				t.Errorf("expandPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestMerge_TildeExpansion_TelemetryPath verifies tilde expansion in telemetry path (P0 bug fix)
func TestMerge_TildeExpansion_TelemetryPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}

	loader := &Loader{}

	dst := &Config{}
	src := &Config{
		Telemetry: TelemetryConfig{
			Path: "~/.engram/telemetry/events.jsonl",
		},
	}

	loader.merge(dst, src, TierUser)

	expected := filepath.Join(homeDir, ".engram/telemetry/events.jsonl")
	if dst.Telemetry.Path != expected {
		t.Errorf("Telemetry.Path = %q, want %q (tilde should be expanded)", dst.Telemetry.Path, expected)
	}

	// Verify NO literal tilde directory
	if strings.HasPrefix(dst.Telemetry.Path, "~") {
		t.Errorf("Telemetry.Path still contains literal tilde: %q", dst.Telemetry.Path)
	}
}

// TestMerge_TildeExpansion_EngramPath verifies tilde expansion in platform engram path
func TestMerge_TildeExpansion_EngramPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}

	loader := &Loader{}

	dst := &Config{}
	src := &Config{
		Platform: PlatformConfig{
			EngramPath: "~/engrams",
		},
	}

	loader.merge(dst, src, TierUser)

	expected := filepath.Join(homeDir, "engrams")
	if dst.Platform.EngramPath != expected {
		t.Errorf("Platform.EngramPath = %q, want %q (tilde should be expanded)", dst.Platform.EngramPath, expected)
	}

	// Verify NO literal tilde directory
	if strings.HasPrefix(dst.Platform.EngramPath, "~") {
		t.Errorf("Platform.EngramPath still contains literal tilde: %q", dst.Platform.EngramPath)
	}
}

// TestMerge_TildeExpansion_PluginPaths verifies tilde expansion in plugin paths
func TestMerge_TildeExpansion_PluginPaths(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}

	loader := &Loader{}

	dst := &Config{}
	src := &Config{
		Plugins: PluginConfig{
			Paths: []string{
				"~/.engram/core/plugins",
				"~/.engram/user/plugins",
				"./local-plugins",
				"/etc/engram/plugins",
			},
		},
	}

	loader.merge(dst, src, TierUser)

	expectedPaths := []string{
		filepath.Join(homeDir, ".engram/core/plugins"),
		filepath.Join(homeDir, ".engram/user/plugins"),
		"./local-plugins",
		"/etc/engram/plugins",
	}

	if len(dst.Plugins.Paths) != len(expectedPaths) {
		t.Fatalf("len(Plugins.Paths) = %d, want %d", len(dst.Plugins.Paths), len(expectedPaths))
	}

	for i, expected := range expectedPaths {
		if dst.Plugins.Paths[i] != expected {
			t.Errorf("Plugins.Paths[%d] = %q, want %q (tilde should be expanded)", i, dst.Plugins.Paths[i], expected)
		}

		// Verify NO literal tilde directory
		if strings.HasPrefix(dst.Plugins.Paths[i], "~") {
			t.Errorf("Plugins.Paths[%d] still contains literal tilde: %q", i, dst.Plugins.Paths[i])
		}
	}
}

// TestLoad_TildeExpansion_Integration verifies tilde expansion works end-to-end
func TestLoad_TildeExpansion_Integration(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}

	tmpDir := t.TempDir()

	// Create core config with tilde paths
	coreDir := filepath.Join(tmpDir, "core")
	if err := os.MkdirAll(coreDir, 0755); err != nil {
		t.Fatalf("failed to create core dir: %v", err)
	}

	coreConfig := `platform:
  agent: "claude-code"
  engram_path: "~/my-engrams"
  token_budget: 50000
plugins:
  paths:
    - "~/.engram/core/plugins"
    - "./local-plugins"
telemetry:
  enabled: true
  path: "~/.engram/telemetry/events.jsonl"
`
	coreConfigPath := filepath.Join(coreDir, "config.yaml")
	if err := os.WriteFile(coreConfigPath, []byte(coreConfig), 0644); err != nil {
		t.Fatalf("failed to write core config: %v", err)
	}

	loader := &Loader{
		paths: map[ConfigTier]string{
			TierCore:    coreConfigPath,
			TierCompany: filepath.Join(tmpDir, "company", "config.yaml"),
			TierTeam:    filepath.Join(tmpDir, "team", "config.yaml"),
			TierUser:    filepath.Join(tmpDir, "user", "config.yaml"),
		},
		cachedMtimes: make(map[ConfigTier]time.Time),
	}

	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify telemetry path is expanded
	expectedTelemetryPath := filepath.Join(homeDir, ".engram/telemetry/events.jsonl")
	if cfg.Telemetry.Path != expectedTelemetryPath {
		t.Errorf("Telemetry.Path = %q, want %q", cfg.Telemetry.Path, expectedTelemetryPath)
	}
	if strings.HasPrefix(cfg.Telemetry.Path, "~") {
		t.Errorf("Telemetry.Path contains literal tilde: %q", cfg.Telemetry.Path)
	}

	// Verify engram path is expanded
	expectedEngramPath := filepath.Join(homeDir, "my-engrams")
	if cfg.Platform.EngramPath != expectedEngramPath {
		t.Errorf("Platform.EngramPath = %q, want %q", cfg.Platform.EngramPath, expectedEngramPath)
	}
	if strings.HasPrefix(cfg.Platform.EngramPath, "~") {
		t.Errorf("Platform.EngramPath contains literal tilde: %q", cfg.Platform.EngramPath)
	}

	// Verify plugin paths are expanded
	if len(cfg.Plugins.Paths) != 2 {
		t.Fatalf("len(Plugins.Paths) = %d, want 2", len(cfg.Plugins.Paths))
	}

	expectedPluginPath0 := filepath.Join(homeDir, ".engram/core/plugins")
	if cfg.Plugins.Paths[0] != expectedPluginPath0 {
		t.Errorf("Plugins.Paths[0] = %q, want %q", cfg.Plugins.Paths[0], expectedPluginPath0)
	}
	if strings.HasPrefix(cfg.Plugins.Paths[0], "~") {
		t.Errorf("Plugins.Paths[0] contains literal tilde: %q", cfg.Plugins.Paths[0])
	}

	// Second path should not be expanded (no tilde)
	if cfg.Plugins.Paths[1] != "./local-plugins" {
		t.Errorf("Plugins.Paths[1] = %q, want %q", cfg.Plugins.Paths[1], "./local-plugins")
	}
}

// TestMerge_TildeExpansion_NoTilde verifies paths without tilde are unchanged
func TestMerge_TildeExpansion_NoTilde(t *testing.T) {
	loader := &Loader{}

	dst := &Config{}
	src := &Config{
		Platform: PlatformConfig{
			EngramPath: "/var/lib/engrams",
		},
		Plugins: PluginConfig{
			Paths: []string{"./plugins", "/etc/engram/plugins"},
		},
		Telemetry: TelemetryConfig{
			Path: "/var/log/engram/telemetry.jsonl",
		},
	}

	loader.merge(dst, src, TierUser)

	// Verify paths are unchanged (no tilde to expand)
	if dst.Platform.EngramPath != "/var/lib/engrams" {
		t.Errorf("Platform.EngramPath = %q, want /var/lib/engrams", dst.Platform.EngramPath)
	}
	if dst.Telemetry.Path != "/var/log/engram/telemetry.jsonl" {
		t.Errorf("Telemetry.Path = %q, want /var/log/engram/telemetry.jsonl", dst.Telemetry.Path)
	}
	if len(dst.Plugins.Paths) != 2 {
		t.Fatalf("len(Plugins.Paths) = %d, want 2", len(dst.Plugins.Paths))
	}
	if dst.Plugins.Paths[0] != "./plugins" {
		t.Errorf("Plugins.Paths[0] = %q, want ./plugins", dst.Plugins.Paths[0])
	}
	if dst.Plugins.Paths[1] != "/etc/engram/plugins" {
		t.Errorf("Plugins.Paths[1] = %q, want /etc/engram/plugins", dst.Plugins.Paths[1])
	}
}

// TestLoad_W0Defaults verifies that W0 config defaults are loaded correctly
func TestLoad_W0Defaults(t *testing.T) {
	// Create temporary directory for test configs
	tmpDir := t.TempDir()

	// Create loader with test paths using helper
	loader := createTestLoader(t, tmpDir)

	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify W0 top-level defaults
	if !cfg.Wayfinder.W0.Enabled {
		t.Error("W0.Enabled should be true by default")
	}

	if cfg.Wayfinder.W0.Enforce {
		t.Error("W0.Enforce should be false by default")
	}

	// Verify subsection defaults using helpers
	assertW0DetectionDefaults(t, cfg)
	assertW0QuestionsDefaults(t, cfg)
	assertW0SynthesisDefaults(t, cfg)
	assertW0TelemetryDefaults(t, cfg)
}

// TestLoad_W0UserOverride verifies that user can override W0 settings
func TestLoad_W0UserOverride(t *testing.T) {
	// Create temporary directory for test configs
	tmpDir := t.TempDir()

	// Create core config
	coreDir := filepath.Join(tmpDir, "core")
	if err := os.MkdirAll(coreDir, 0755); err != nil {
		t.Fatalf("failed to create core dir: %v", err)
	}

	coreConfig := `platform:
  agent: "claude-code"
`
	coreConfigPath := filepath.Join(coreDir, "config.yaml")
	if err := os.WriteFile(coreConfigPath, []byte(coreConfig), 0644); err != nil {
		t.Fatalf("failed to write core config: %v", err)
	}

	// Create user config with W0 disabled
	userDir := filepath.Join(tmpDir, "user")
	if err := os.MkdirAll(userDir, 0755); err != nil {
		t.Fatalf("failed to create user dir: %v", err)
	}

	userConfig := `wayfinder:
  w0:
    enabled: false
    detection:
      min_word_count: 20
`
	userConfigPath := filepath.Join(userDir, "config.yaml")
	if err := os.WriteFile(userConfigPath, []byte(userConfig), 0644); err != nil {
		t.Fatalf("failed to write user config: %v", err)
	}

	// Create loader with test paths
	loader := &Loader{
		paths: map[ConfigTier]string{
			TierCore:    coreConfigPath,
			TierCompany: filepath.Join(tmpDir, "company", "config.yaml"),
			TierTeam:    filepath.Join(tmpDir, "team", "config.yaml"),
			TierUser:    userConfigPath,
		},
		cachedMtimes: make(map[ConfigTier]time.Time),
	}

	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify user overrides were applied
	if cfg.Wayfinder.W0.Enabled {
		t.Error("W0.Enabled should be false (user override)")
	}

	if cfg.Wayfinder.W0.Detection.MinWordCount != 20 {
		t.Errorf("W0.Detection.MinWordCount = %d, want 20 (user override)", cfg.Wayfinder.W0.Detection.MinWordCount)
	}

	// Verify non-overridden defaults still apply
	if cfg.Wayfinder.W0.Detection.MaxSkipWordCount != 50 {
		t.Errorf("W0.Detection.MaxSkipWordCount = %d, want 50 (default)", cfg.Wayfinder.W0.Detection.MaxSkipWordCount)
	}
}

// Test helper functions for TestLoad_W0Defaults

// createTestLoader creates a loader with minimal core config for testing
func createTestLoader(t *testing.T, tmpDir string) *Loader {
	t.Helper()
	// Create core config with minimal content
	coreDir := filepath.Join(tmpDir, "core")
	if err := os.MkdirAll(coreDir, 0755); err != nil {
		t.Fatalf("failed to create core dir: %v", err)
	}

	coreConfig := `platform:
  agent: "claude-code"
`
	coreConfigPath := filepath.Join(coreDir, "config.yaml")
	if err := os.WriteFile(coreConfigPath, []byte(coreConfig), 0644); err != nil {
		t.Fatalf("failed to write core config: %v", err)
	}

	return &Loader{
		paths: map[ConfigTier]string{
			TierCore:    coreConfigPath,
			TierCompany: filepath.Join(tmpDir, "company", "config.yaml"),
			TierTeam:    filepath.Join(tmpDir, "team", "config.yaml"),
			TierUser:    filepath.Join(tmpDir, "user", "config.yaml"),
		},
		cachedMtimes: make(map[ConfigTier]time.Time),
	}
}

// assertW0DetectionDefaults verifies W0 detection configuration defaults
func assertW0DetectionDefaults(t *testing.T, cfg *Config) {
	t.Helper()
	if !cfg.Wayfinder.W0.Detection.Enabled {
		t.Error("W0.Detection.Enabled should be true by default")
	}
	if cfg.Wayfinder.W0.Detection.MinWordCount != 30 {
		t.Errorf("W0.Detection.MinWordCount = %d, want 30", cfg.Wayfinder.W0.Detection.MinWordCount)
	}
	if cfg.Wayfinder.W0.Detection.MaxSkipWordCount != 50 {
		t.Errorf("W0.Detection.MaxSkipWordCount = %d, want 50", cfg.Wayfinder.W0.Detection.MaxSkipWordCount)
	}
	if cfg.Wayfinder.W0.Detection.VaguenessThreshold != 0.6 {
		t.Errorf("W0.Detection.VaguenessThreshold = %f, want 0.6", cfg.Wayfinder.W0.Detection.VaguenessThreshold)
	}
}

// assertW0QuestionsDefaults verifies W0 questions configuration defaults
func assertW0QuestionsDefaults(t *testing.T, cfg *Config) {
	t.Helper()
	if cfg.Wayfinder.W0.Questions.MaxRounds != 3 {
		t.Errorf("W0.Questions.MaxRounds = %d, want 3", cfg.Wayfinder.W0.Questions.MaxRounds)
	}
	if !cfg.Wayfinder.W0.Questions.IncludeExamples {
		t.Error("W0.Questions.IncludeExamples should be true by default")
	}
	if !cfg.Wayfinder.W0.Questions.IncludeHelpText {
		t.Error("W0.Questions.IncludeHelpText should be true by default")
	}
}

// assertW0SynthesisDefaults verifies W0 synthesis configuration defaults
func assertW0SynthesisDefaults(t *testing.T, cfg *Config) {
	t.Helper()
	if cfg.Wayfinder.W0.Synthesis.Method != "few_shot_cot" {
		t.Errorf("W0.Synthesis.Method = %q, want few_shot_cot", cfg.Wayfinder.W0.Synthesis.Method)
	}
	if cfg.Wayfinder.W0.Synthesis.TimeoutSeconds != 15 {
		t.Errorf("W0.Synthesis.TimeoutSeconds = %d, want 15", cfg.Wayfinder.W0.Synthesis.TimeoutSeconds)
	}
	if !cfg.Wayfinder.W0.Synthesis.RetryOnFailure {
		t.Error("W0.Synthesis.RetryOnFailure should be true by default")
	}
}

// assertW0TelemetryDefaults verifies W0 telemetry configuration defaults
func assertW0TelemetryDefaults(t *testing.T, cfg *Config) {
	t.Helper()
	if !cfg.Wayfinder.W0.Telemetry.Enabled {
		t.Error("W0.Telemetry.Enabled should be true by default")
	}
	if !cfg.Wayfinder.W0.Telemetry.LogTechnicalTerms {
		t.Error("W0.Telemetry.LogTechnicalTerms should be true by default")
	}
	if !cfg.Wayfinder.W0.Telemetry.LogResponseMetadata {
		t.Error("W0.Telemetry.LogResponseMetadata should be true by default")
	}
}
