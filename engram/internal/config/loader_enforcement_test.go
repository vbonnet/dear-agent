package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/internal/testutil"
	"gopkg.in/yaml.v3"
)

// TestTelemetryEnforcement_CompanyEnforcesUserCannotDisable tests that
// when company config enforces telemetry, user config cannot disable it.
// This is critical for GDPR compliance - enforcement must work correctly.
func TestTelemetryEnforcement_CompanyEnforcesUserCannotDisable(t *testing.T) {
	testutil.SkipIfRoot(t) // Root bypasses filesystem permission checks
	tmpDir := t.TempDir()

	// Create core config (default: telemetry enabled)
	coreConfig := filepath.Join(tmpDir, "core.yaml")
	coreData := `
telemetry:
  enabled: true
`
	if err := os.WriteFile(coreConfig, []byte(coreData), 0644); err != nil {
		t.Fatalf("Failed to create core config: %v", err)
	}

	// Create company config with enforcement
	companyConfig := filepath.Join(tmpDir, "company.yaml")
	companyData := `
telemetry:
  enabled: true
  enforce: true
`
	if err := os.WriteFile(companyConfig, []byte(companyData), 0644); err != nil {
		t.Fatalf("Failed to create company config: %v", err)
	}

	// Create user config trying to disable telemetry (GDPR opt-out attempt)
	userConfig := filepath.Join(tmpDir, "user.yaml")
	userData := `
telemetry:
  enabled: false
`
	if err := os.WriteFile(userConfig, []byte(userData), 0644); err != nil {
		t.Fatalf("Failed to create user config: %v", err)
	}

	// Load configs in order: core -> company -> user
	loader := &Loader{
		paths: map[ConfigTier]string{
			TierCore:    coreConfig,
			TierCompany: companyConfig,
			TierTeam:    "/nonexistent", // Skip team
			TierUser:    userConfig,
		},
	}

	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify enforcement worked: telemetry must be enabled despite user config
	if !cfg.Telemetry.Enforce {
		t.Error("Expected telemetry.enforce=true from company config")
	}

	if !cfg.Telemetry.Enabled {
		t.Error("Expected telemetry.enabled=true (enforced), but user config disabled it")
		t.Error("GDPR VIOLATION: Enforcement logic failed - user can bypass company policy")
	}
}

// TestTelemetryEnforcement_NoEnforcementUserCanDisable tests that
// when company doesn't enforce, user can disable telemetry (GDPR opt-out).
func TestTelemetryEnforcement_NoEnforcementUserCanDisable(t *testing.T) {
	testutil.SkipIfRoot(t) // Root bypasses filesystem permission checks
	tmpDir := t.TempDir()

	// Create core config (default: telemetry enabled)
	coreConfig := filepath.Join(tmpDir, "core.yaml")
	coreData := `
telemetry:
  enabled: true
`
	if err := os.WriteFile(coreConfig, []byte(coreData), 0644); err != nil {
		t.Fatalf("Failed to create core config: %v", err)
	}

	// Create company config WITHOUT enforcement
	companyConfig := filepath.Join(tmpDir, "company.yaml")
	companyData := `
telemetry:
  enabled: true
  enforce: false
`
	if err := os.WriteFile(companyConfig, []byte(companyData), 0644); err != nil {
		t.Fatalf("Failed to create company config: %v", err)
	}

	// Create user config disabling telemetry (GDPR opt-out)
	userConfig := filepath.Join(tmpDir, "user.yaml")
	userData := `
telemetry:
  enabled: false
`
	if err := os.WriteFile(userConfig, []byte(userData), 0644); err != nil {
		t.Fatalf("Failed to create user config: %v", err)
	}

	// Load configs
	loader := &Loader{
		paths: map[ConfigTier]string{
			TierCore:    coreConfig,
			TierCompany: companyConfig,
			TierTeam:    "/nonexistent",
			TierUser:    userConfig,
		},
	}

	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify user opt-out worked
	if cfg.Telemetry.Enforce {
		t.Error("Expected telemetry.enforce=false (no enforcement)")
	}

	if cfg.Telemetry.Enabled {
		t.Error("Expected telemetry.enabled=false (user opted out)")
		t.Error("GDPR VIOLATION: User cannot opt-out of telemetry")
	}
}

// TestTelemetryEnforcement_NoCompanyConfigUserCanDisable tests that
// without company config, user can freely disable telemetry.
func TestTelemetryEnforcement_NoCompanyConfigUserCanDisable(t *testing.T) {
	testutil.SkipIfRoot(t) // Root bypasses filesystem permission checks
	tmpDir := t.TempDir()

	// Create core config (default: telemetry enabled)
	coreConfig := filepath.Join(tmpDir, "core.yaml")
	coreData := `
telemetry:
  enabled: true
`
	if err := os.WriteFile(coreConfig, []byte(coreData), 0644); err != nil {
		t.Fatalf("Failed to create core config: %v", err)
	}

	// Create user config disabling telemetry (GDPR opt-out)
	userConfig := filepath.Join(tmpDir, "user.yaml")
	userData := `
telemetry:
  enabled: false
`
	if err := os.WriteFile(userConfig, []byte(userData), 0644); err != nil {
		t.Fatalf("Failed to create user config: %v", err)
	}

	// Load configs (no company config)
	loader := &Loader{
		paths: map[ConfigTier]string{
			TierCore:    coreConfig,
			TierCompany: "/nonexistent",
			TierTeam:    "/nonexistent",
			TierUser:    userConfig,
		},
	}

	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify user opt-out worked
	if cfg.Telemetry.Enabled {
		t.Error("Expected telemetry.enabled=false (user opted out)")
		t.Error("GDPR VIOLATION: User cannot opt-out without company config")
	}
}

// TestTelemetryEnforcement_DefaultEnabled tests that telemetry is enabled by default
func TestTelemetryEnforcement_DefaultEnabled(t *testing.T) {
	testutil.SkipIfRoot(t) // Root bypasses filesystem permission checks
	tmpDir := t.TempDir()

	// Create core config (default: telemetry enabled)
	coreConfig := filepath.Join(tmpDir, "core.yaml")
	coreData := `
telemetry:
  enabled: true
`
	if err := os.WriteFile(coreConfig, []byte(coreData), 0644); err != nil {
		t.Fatalf("Failed to create core config: %v", err)
	}

	// Load config (no other tiers)
	loader := &Loader{
		paths: map[ConfigTier]string{
			TierCore:    coreConfig,
			TierCompany: "/nonexistent",
			TierTeam:    "/nonexistent",
			TierUser:    "/nonexistent",
		},
	}

	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify default is enabled (as per ADR)
	if !cfg.Telemetry.Enabled {
		t.Error("Expected telemetry.enabled=true by default")
	}

	// Verify no enforcement by default
	if cfg.Telemetry.Enforce {
		t.Error("Expected telemetry.enforce=false by default")
	}
}

// TestTelemetryEnforcement_TeamCannotOverrideCompany tests that
// team config cannot override company enforcement.
func TestTelemetryEnforcement_TeamCannotOverrideCompany(t *testing.T) {
	testutil.SkipIfRoot(t) // Root bypasses filesystem permission checks
	tmpDir := t.TempDir()

	// Create core config
	coreConfig := filepath.Join(tmpDir, "core.yaml")
	coreData := `
telemetry:
  enabled: true
`
	if err := os.WriteFile(coreConfig, []byte(coreData), 0644); err != nil {
		t.Fatalf("Failed to create core config: %v", err)
	}

	// Create company config with enforcement
	companyConfig := filepath.Join(tmpDir, "company.yaml")
	companyData := `
telemetry:
  enabled: true
  enforce: true
`
	if err := os.WriteFile(companyConfig, []byte(companyData), 0644); err != nil {
		t.Fatalf("Failed to create company config: %v", err)
	}

	// Create team config trying to disable enforcement
	teamConfig := filepath.Join(tmpDir, "team.yaml")
	teamData := `
telemetry:
  enabled: false
  enforce: false
`
	if err := os.WriteFile(teamConfig, []byte(teamData), 0644); err != nil {
		t.Fatalf("Failed to create team config: %v", err)
	}

	// Load configs
	loader := &Loader{
		paths: map[ConfigTier]string{
			TierCore:    coreConfig,
			TierCompany: companyConfig,
			TierTeam:    teamConfig,
			TierUser:    "/nonexistent",
		},
	}

	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify enforcement persists (team cannot override)
	if !cfg.Telemetry.Enforce {
		t.Error("Expected telemetry.enforce=true (team cannot override company)")
	}

	if !cfg.Telemetry.Enabled {
		t.Error("Expected telemetry.enabled=true (enforced by company)")
		t.Error("SECURITY ISSUE: Team config can bypass company enforcement")
	}
}

// TestTelemetryEnforcement_UserCannotSetEnforce tests that user tier cannot set enforce=true
// P0-2: Only Core/Company tiers can set enforcement flag
func TestTelemetryEnforcement_UserCannotSetEnforce(t *testing.T) {
	testutil.SkipIfRoot(t) // Root bypasses filesystem permission checks
	tmpDir := t.TempDir()

	// Create core config (no enforcement)
	coreConfig := filepath.Join(tmpDir, "core.yaml")
	coreData := `
telemetry:
  enabled: true
  enforce: false
`
	if err := os.WriteFile(coreConfig, []byte(coreData), 0644); err != nil {
		t.Fatalf("Failed to create core config: %v", err)
	}

	// Create user config trying to set enforce=true (security violation attempt)
	userConfig := filepath.Join(tmpDir, "user.yaml")
	userData := `
telemetry:
  enabled: true
  enforce: true
`
	if err := os.WriteFile(userConfig, []byte(userData), 0644); err != nil {
		t.Fatalf("Failed to create user config: %v", err)
	}

	// Load configs
	loader := &Loader{
		paths: map[ConfigTier]string{
			TierCore:    coreConfig,
			TierCompany: "/nonexistent",
			TierTeam:    "/nonexistent",
			TierUser:    userConfig,
		},
	}

	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify user cannot set enforcement
	if cfg.Telemetry.Enforce {
		t.Error("Expected telemetry.enforce=false (user tier cannot set enforcement)")
		t.Error("P0-2 SECURITY ISSUE: User can bypass company enforcement by setting enforce=true")
	}
}

// TestTelemetryEnforcement_TeamCannotSetEnforce tests that team tier cannot set enforce=true
// P0-2: Only Core/Company tiers can set enforcement flag
func TestTelemetryEnforcement_TeamCannotSetEnforce(t *testing.T) {
	testutil.SkipIfRoot(t) // Root bypasses filesystem permission checks
	tmpDir := t.TempDir()

	// Create core config (no enforcement)
	coreConfig := filepath.Join(tmpDir, "core.yaml")
	coreData := `
telemetry:
  enabled: true
  enforce: false
`
	if err := os.WriteFile(coreConfig, []byte(coreData), 0644); err != nil {
		t.Fatalf("Failed to create core config: %v", err)
	}

	// Create team config trying to set enforce=true (security violation attempt)
	teamConfig := filepath.Join(tmpDir, "team.yaml")
	teamData := `
telemetry:
  enabled: true
  enforce: true
`
	if err := os.WriteFile(teamConfig, []byte(teamData), 0644); err != nil {
		t.Fatalf("Failed to create team config: %v", err)
	}

	// Load configs
	loader := &Loader{
		paths: map[ConfigTier]string{
			TierCore:    coreConfig,
			TierCompany: "/nonexistent",
			TierTeam:    teamConfig,
			TierUser:    "/nonexistent",
		},
	}

	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify team cannot set enforcement
	if cfg.Telemetry.Enforce {
		t.Error("Expected telemetry.enforce=false (team tier cannot set enforcement)")
		t.Error("P0-2 SECURITY ISSUE: Team can set enforcement without company authorization")
	}
}

// TestTelemetryEnforcement_CompanyCanSetEnforce tests that company tier CAN set enforce=true
// P0-2: Company tier is authorized to set enforcement
func TestTelemetryEnforcement_CompanyCanSetEnforce(t *testing.T) {
	testutil.SkipIfRoot(t) // Root bypasses filesystem permission checks
	tmpDir := t.TempDir()

	// Create core config (no enforcement)
	coreConfig := filepath.Join(tmpDir, "core.yaml")
	coreData := `
telemetry:
  enabled: true
  enforce: false
`
	if err := os.WriteFile(coreConfig, []byte(coreData), 0644); err != nil {
		t.Fatalf("Failed to create core config: %v", err)
	}

	// Create company config setting enforce=true (valid)
	companyConfig := filepath.Join(tmpDir, "company.yaml")
	companyData := `
telemetry:
  enabled: true
  enforce: true
`
	if err := os.WriteFile(companyConfig, []byte(companyData), 0644); err != nil {
		t.Fatalf("Failed to create company config: %v", err)
	}

	// Load configs
	loader := &Loader{
		paths: map[ConfigTier]string{
			TierCore:    coreConfig,
			TierCompany: companyConfig,
			TierTeam:    "/nonexistent",
			TierUser:    "/nonexistent",
		},
	}

	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify company CAN set enforcement
	if !cfg.Telemetry.Enforce {
		t.Error("Expected telemetry.enforce=true (company tier should be able to set enforcement)")
	}
}

// TestTelemetryConfig_AllFields tests that all telemetry config fields are loaded
func TestTelemetryConfig_AllFields(t *testing.T) {
	tmpDir := t.TempDir()

	// Create config with all fields
	configData := `
telemetry:
  enabled: true
  enforce: true
  storage: remote
  path: /var/log/engram
  max_size_mb: 500
  retention_days: 180
  local_backup: true
  sink:
    type: http
    url: https://telemetry.example.com/v1/events
    auth:
      type: bearer
      token_env: ENGRAM_TELEMETRY_TOKEN
`
	configPath := createTelemetryConfigFile(t, tmpDir, configData)

	// Load config
	cfg := loadConfigFile(t, configPath)

	// Verify all fields
	assertTelemetryTopLevel(t, cfg)
	assertTelemetrySink(t, cfg)
	assertTelemetryAuth(t, cfg)
}

// Test helper functions for TestTelemetryConfig_AllFields

// createTelemetryConfigFile creates a config file with telemetry settings
func createTelemetryConfigFile(t *testing.T, tmpDir string, configData string) string {
	t.Helper()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configData), 0644); err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}
	return configPath
}

// loadConfigFile reads and unmarshals a config file
func loadConfigFile(t *testing.T, configPath string) *Config {
	t.Helper()
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}
	return &cfg
}

// assertTelemetryTopLevel verifies top-level telemetry config fields
func assertTelemetryTopLevel(t *testing.T, cfg *Config) {
	t.Helper()
	if !cfg.Telemetry.Enabled {
		t.Error("Expected enabled=true")
	}
	if !cfg.Telemetry.Enforce {
		t.Error("Expected enforce=true")
	}
	if cfg.Telemetry.Storage != "remote" {
		t.Errorf("Expected storage='remote', got '%s'", cfg.Telemetry.Storage)
	}
	if cfg.Telemetry.Path != "/var/log/engram" {
		t.Errorf("Expected path='/var/log/engram', got '%s'", cfg.Telemetry.Path)
	}
	if cfg.Telemetry.MaxSizeMB != 500 {
		t.Errorf("Expected max_size_mb=500, got %d", cfg.Telemetry.MaxSizeMB)
	}
	if cfg.Telemetry.RetentionDays != 180 {
		t.Errorf("Expected retention_days=180, got %d", cfg.Telemetry.RetentionDays)
	}
	if !cfg.Telemetry.LocalBackup {
		t.Error("Expected local_backup=true")
	}
}

// assertTelemetrySink verifies telemetry sink config fields
func assertTelemetrySink(t *testing.T, cfg *Config) {
	t.Helper()
	if cfg.Telemetry.Sink == nil {
		t.Fatal("Expected sink config, got nil")
	}
	if cfg.Telemetry.Sink.Type != "http" {
		t.Errorf("Expected sink.type='http', got '%s'", cfg.Telemetry.Sink.Type)
	}
	if cfg.Telemetry.Sink.URL != "https://telemetry.example.com/v1/events" {
		t.Errorf("Expected sink.url, got '%s'", cfg.Telemetry.Sink.URL)
	}
}

// assertTelemetryAuth verifies telemetry auth config fields
func assertTelemetryAuth(t *testing.T, cfg *Config) {
	t.Helper()
	if cfg.Telemetry.Sink.Auth == nil {
		t.Fatal("Expected auth config, got nil")
	}
	if cfg.Telemetry.Sink.Auth.Type != "bearer" {
		t.Errorf("Expected auth.type='bearer', got '%s'", cfg.Telemetry.Sink.Auth.Type)
	}
	if cfg.Telemetry.Sink.Auth.TokenEnv != "ENGRAM_TELEMETRY_TOKEN" {
		t.Errorf("Expected token_env='ENGRAM_TELEMETRY_TOKEN', got '%s'", cfg.Telemetry.Sink.Auth.TokenEnv)
	}
}
