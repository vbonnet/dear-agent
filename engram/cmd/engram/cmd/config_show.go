package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/internal/config"
	"gopkg.in/yaml.v3"
)

var (
	showJSON   bool
	showSource bool
)

var configShowCmd = &cobra.Command{
	Use:   "show [section]",
	Short: "Display effective configuration",
	Long: `Display the effective configuration with source annotations.

Shows which configuration file or environment variable provides each setting.
This helps understand configuration precedence and troubleshoot conflicts.

PRECEDENCE ORDER (highest to lowest):
  1. CLI flags (--flag=value)
  2. Environment variables (ENGRAM_SETTING=value)
  3. User config (~/.engram/user/config.yaml)
  4. Team config (~/.engram/team/config.yaml)
  5. Company config (~/.engram/company/config.yaml)
  6. Core config (~/.engram/core/config.yaml)
  7. Built-in defaults (hardcoded)

ARGUMENTS
  section   Optional section to display (platform, plugins, telemetry, wayfinder)

FLAGS
  --json    Output as JSON
  --source  Show source annotations (default: true)

EXAMPLES
  # Show all configuration
  $ engram config show

  # Show only telemetry section
  $ engram config show telemetry

  # Show as JSON for scripting
  $ engram config show --json

  # Show without source annotations
  $ engram config show --source=false

OUTPUT FORMAT
  Each setting shows the effective value and its source:

    Telemetry:
      enabled: true                   (core config)
      path: ~/.engram/telemetry.jsonl (user config)
      enforce: false                  (company config)

  Environment variable overrides are shown:
      enabled: true                   (env: ENGRAM_TELEMETRY_ENABLED)

TROUBLESHOOTING
  Use this command to diagnose configuration issues:
  - "Why is this setting not working?" → Check which tier provides it
  - "Which config file is being used?" → See source annotations
  - "Is my user config overriding team?" → Compare sources`,
	RunE: runConfigShow,
}

func init() {
	configCmd.AddCommand(configShowCmd)

	configShowCmd.Flags().BoolVar(&showJSON, "json", false, "Output as JSON")
	configShowCmd.Flags().BoolVar(&showSource, "source", true, "Show source annotations")
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	// Create loader
	loader := config.NewLoader()

	// Load configuration
	cfg, err := loader.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Load configuration with precedence tracking
	effective, err := loadEffectiveConfig(loader)
	if err != nil {
		return fmt.Errorf("failed to load effective configuration: %w", err)
	}

	// Filter by section if specified
	var section string
	if len(args) > 0 {
		section = strings.ToLower(args[0])
		if !isValidSection(section) {
			return fmt.Errorf("invalid section: %s (valid: platform, plugins, telemetry, wayfinder)", section)
		}
	}

	// Output format
	if showJSON {
		return outputConfigJSON(effective, section)
	}

	return outputConfigMarkdown(cfg, effective, section)
}

// EffectiveConfig represents configuration with source tracking
type EffectiveConfig struct {
	Platform  map[string]SourcedValue `json:"platform"`
	Plugins   map[string]SourcedValue `json:"plugins"`
	Telemetry map[string]SourcedValue `json:"telemetry"`
	Wayfinder map[string]SourcedValue `json:"wayfinder"`
}

// SourcedValue represents a config value with its source
type SourcedValue struct {
	Value  interface{} `json:"value"`
	Source string      `json:"source"`
}

// loadEffectiveConfig loads configuration and tracks which tier provides each setting
func loadEffectiveConfig(loader *config.Loader) (*EffectiveConfig, error) {
	effective := &EffectiveConfig{
		Platform:  make(map[string]SourcedValue),
		Plugins:   make(map[string]SourcedValue),
		Telemetry: make(map[string]SourcedValue),
		Wayfinder: make(map[string]SourcedValue),
	}

	// Load each tier and track sources
	tiers := []config.ConfigTier{config.TierCore, config.TierCompany, config.TierTeam, config.TierUser}
	paths := config.DefaultPaths()

	for _, tier := range tiers {
		path := paths[tier]

		// Load tier config
		tierCfg, err := loadTierConfig(path)
		if err != nil {
			// Skip missing optional tiers
			if tier != config.TierCore && os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("failed to load %s config: %w", tier, err)
		}

		// Track sources from this tier
		trackPlatformSources(effective, tierCfg, tier)
		trackPluginSources(effective, tierCfg, tier)
		trackTelemetrySources(effective, tierCfg, tier)
		trackWayfinderSources(effective, tierCfg, tier)
	}

	// Check for environment variable overrides
	checkEnvOverrides(effective)

	return effective, nil
}

// loadTierConfig loads a single tier configuration file
func loadTierConfig(path string) (*config.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// trackPlatformSources records platform setting sources
func trackPlatformSources(eff *EffectiveConfig, cfg *config.Config, tier config.ConfigTier) {
	if cfg.Platform.Agent != "" {
		eff.Platform["agent"] = SourcedValue{
			Value:  cfg.Platform.Agent,
			Source: formatSource(tier),
		}
	}
	if cfg.Platform.EngramPath != "" {
		eff.Platform["engram_path"] = SourcedValue{
			Value:  cfg.Platform.EngramPath,
			Source: formatSource(tier),
		}
	}
	if cfg.Platform.TokenBudget > 0 {
		eff.Platform["token_budget"] = SourcedValue{
			Value:  cfg.Platform.TokenBudget,
			Source: formatSource(tier),
		}
	}
}

// trackPluginSources records plugin setting sources
func trackPluginSources(eff *EffectiveConfig, cfg *config.Config, tier config.ConfigTier) {
	if len(cfg.Plugins.Paths) > 0 {
		eff.Plugins["paths"] = SourcedValue{
			Value:  cfg.Plugins.Paths,
			Source: formatSource(tier),
		}
	}
	if len(cfg.Plugins.Disabled) > 0 {
		eff.Plugins["disabled"] = SourcedValue{
			Value:  cfg.Plugins.Disabled,
			Source: formatSource(tier),
		}
	}
}

// trackTelemetrySources records telemetry setting sources
func trackTelemetrySources(eff *EffectiveConfig, cfg *config.Config, tier config.ConfigTier) {
	// Special handling for enforce flag (only Core/Company can set)
	if cfg.Telemetry.Enforce && (tier == config.TierCore || tier == config.TierCompany) {
		eff.Telemetry["enforce"] = SourcedValue{
			Value:  true,
			Source: formatSource(tier),
		}
	}

	// Enabled flag (respects enforcement)
	_, hasEnforce := eff.Telemetry["enforce"]
	if hasEnforce {
		// Enforcement is active - telemetry always enabled
		eff.Telemetry["enabled"] = SourcedValue{
			Value:  true,
			Source: "enforced by " + eff.Telemetry["enforce"].Source,
		}
	} else {
		// Normal merge
		eff.Telemetry["enabled"] = SourcedValue{
			Value:  cfg.Telemetry.Enabled,
			Source: formatSource(tier),
		}
	}

	if cfg.Telemetry.Storage != "" {
		eff.Telemetry["storage"] = SourcedValue{
			Value:  cfg.Telemetry.Storage,
			Source: formatSource(tier),
		}
	}
	if cfg.Telemetry.Path != "" {
		eff.Telemetry["path"] = SourcedValue{
			Value:  cfg.Telemetry.Path,
			Source: formatSource(tier),
		}
	}
	if cfg.Telemetry.MaxSizeMB > 0 {
		eff.Telemetry["max_size_mb"] = SourcedValue{
			Value:  cfg.Telemetry.MaxSizeMB,
			Source: formatSource(tier),
		}
	}
	if cfg.Telemetry.RetentionDays > 0 {
		eff.Telemetry["retention_days"] = SourcedValue{
			Value:  cfg.Telemetry.RetentionDays,
			Source: formatSource(tier),
		}
	}
}

// trackWayfinderSources records wayfinder setting sources
func trackWayfinderSources(eff *EffectiveConfig, cfg *config.Config, tier config.ConfigTier) {
	// W0 enforcement (only Core/Company can set)
	if cfg.Wayfinder.W0.Enforce && (tier == config.TierCore || tier == config.TierCompany) {
		eff.Wayfinder["w0.enforce"] = SourcedValue{
			Value:  true,
			Source: formatSource(tier),
		}
	}

	// W0 enabled (respects enforcement)
	_, hasEnforce := eff.Wayfinder["w0.enforce"]
	if hasEnforce {
		eff.Wayfinder["w0.enabled"] = SourcedValue{
			Value:  true,
			Source: "enforced by " + eff.Wayfinder["w0.enforce"].Source,
		}
	} else {
		eff.Wayfinder["w0.enabled"] = SourcedValue{
			Value:  cfg.Wayfinder.W0.Enabled,
			Source: formatSource(tier),
		}
	}

	// W0 detection settings
	if cfg.Wayfinder.W0.Detection.MinWordCount > 0 {
		eff.Wayfinder["w0.detection.min_word_count"] = SourcedValue{
			Value:  cfg.Wayfinder.W0.Detection.MinWordCount,
			Source: formatSource(tier),
		}
	}
	if cfg.Wayfinder.W0.Detection.MaxSkipWordCount > 0 {
		eff.Wayfinder["w0.detection.max_skip_word_count"] = SourcedValue{
			Value:  cfg.Wayfinder.W0.Detection.MaxSkipWordCount,
			Source: formatSource(tier),
		}
	}
	if cfg.Wayfinder.W0.Detection.VaguenessThreshold > 0 {
		eff.Wayfinder["w0.detection.vagueness_threshold"] = SourcedValue{
			Value:  cfg.Wayfinder.W0.Detection.VaguenessThreshold,
			Source: formatSource(tier),
		}
	}
}

// checkEnvOverrides checks for environment variable overrides
func checkEnvOverrides(eff *EffectiveConfig) {
	// Platform overrides
	if agent := os.Getenv("ENGRAM_PLATFORM_AGENT"); agent != "" {
		eff.Platform["agent"] = SourcedValue{
			Value:  agent,
			Source: "env: ENGRAM_PLATFORM_AGENT",
		}
	}
	if path := os.Getenv("ENGRAM_PLATFORM_ENGRAM_PATH"); path != "" {
		eff.Platform["engram_path"] = SourcedValue{
			Value:  path,
			Source: "env: ENGRAM_PLATFORM_ENGRAM_PATH",
		}
	}

	// Telemetry overrides
	if enabled := os.Getenv("ENGRAM_TELEMETRY_ENABLED"); enabled != "" {
		eff.Telemetry["enabled"] = SourcedValue{
			Value:  enabled == "1" || enabled == "true",
			Source: "env: ENGRAM_TELEMETRY_ENABLED",
		}
	}
	if path := os.Getenv("ENGRAM_TELEMETRY_PATH"); path != "" {
		eff.Telemetry["path"] = SourcedValue{
			Value:  path,
			Source: "env: ENGRAM_TELEMETRY_PATH",
		}
	}

	// Memory provider overrides
	if provider := os.Getenv("ENGRAM_MEMORY_PROVIDER"); provider != "" {
		eff.Platform["memory_provider"] = SourcedValue{
			Value:  provider,
			Source: "env: ENGRAM_MEMORY_PROVIDER",
		}
	}
	if cfg := os.Getenv("ENGRAM_MEMORY_CONFIG"); cfg != "" {
		eff.Platform["memory_config"] = SourcedValue{
			Value:  cfg,
			Source: "env: ENGRAM_MEMORY_CONFIG",
		}
	}
}

// formatSource formats tier name for display
func formatSource(tier config.ConfigTier) string {
	switch tier {
	case config.TierCore:
		return "core config"
	case config.TierCompany:
		return "company config"
	case config.TierTeam:
		return "team config"
	case config.TierUser:
		return "user config"
	default:
		return "default"
	}
}

// isValidSection checks if section name is valid
func isValidSection(section string) bool {
	validSections := map[string]bool{
		"platform":  true,
		"plugins":   true,
		"telemetry": true,
		"wayfinder": true,
	}
	return validSections[section]
}

// outputConfigJSON outputs configuration as JSON
func outputConfigJSON(eff *EffectiveConfig, section string) error {
	var output interface{}

	if section == "" {
		output = eff
	} else {
		switch section {
		case "platform":
			output = eff.Platform
		case "plugins":
			output = eff.Plugins
		case "telemetry":
			output = eff.Telemetry
		case "wayfinder":
			output = eff.Wayfinder
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

// outputConfigMarkdown outputs configuration in markdown format
func outputConfigMarkdown(cfg *config.Config, eff *EffectiveConfig, section string) error {
	fmt.Println("Effective Configuration")
	fmt.Println()

	if section == "" || section == "platform" {
		printSection("Platform", eff.Platform)
	}

	if section == "" || section == "plugins" {
		printSection("Plugins", eff.Plugins)
	}

	if section == "" || section == "telemetry" {
		printSection("Telemetry", eff.Telemetry)
	}

	if section == "" || section == "wayfinder" {
		printSection("Wayfinder", eff.Wayfinder)
	}

	return nil
}

// printSection prints a configuration section
func printSection(name string, values map[string]SourcedValue) {
	if len(values) == 0 {
		return
	}

	fmt.Printf("%s:\n", name)
	for key, sv := range values {
		valueStr := formatValue(sv.Value)
		if showSource {
			fmt.Printf("  %-30s %s (%s)\n", key+":", valueStr, sv.Source)
		} else {
			fmt.Printf("  %-30s %s\n", key+":", valueStr)
		}
	}
	fmt.Println()
}

// formatValue formats a value for display
func formatValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val)
	case []string:
		return fmt.Sprintf("%v", val)
	case bool:
		return fmt.Sprintf("%t", val)
	case int:
		return fmt.Sprintf("%d", val)
	case float64:
		return fmt.Sprintf("%.2f", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
