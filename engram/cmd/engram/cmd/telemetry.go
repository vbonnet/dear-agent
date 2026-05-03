package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/internal/config"
	"github.com/vbonnet/dear-agent/internal/telemetry/analysis"
	"github.com/vbonnet/dear-agent/pkg/table"
	"gopkg.in/yaml.v3"
)

var telemetryCmd = &cobra.Command{
	Use:   "telemetry",
	Short: "Manage telemetry settings",
	Long: `Manage telemetry configuration and view current settings.

Telemetry helps improve Engram by collecting anonymous usage data.
All data is stored locally by default and never sent to external services
unless explicitly configured by your organization.

COMMANDS
  status  - Show whether telemetry is enabled or disabled
  enable  - Enable telemetry collection
  disable - Disable telemetry collection
  show    - Show detailed telemetry configuration

EXAMPLES
  # Check telemetry status
  $ engram telemetry status

  # Enable telemetry
  $ engram telemetry enable

  # Disable telemetry (GDPR opt-out)
  $ engram telemetry disable

  # Show full configuration
  $ engram telemetry show

PRIVACY
  - Data is stored locally in ~/.engram/telemetry/ by default
  - No code content, file paths, or personal data is collected
  - You can disable at any time with 'engram telemetry disable'
  - Your organization may enforce telemetry for compliance

LEARN MORE
  See docs/adr/telemetry-system.md for detailed privacy policy`,
}

var telemetryStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show telemetry enabled/disabled status",
	Long: `Display whether telemetry is currently enabled or disabled.

Also shows if telemetry is enforced by organization policy.

EXAMPLES
  $ engram telemetry status
  Telemetry: Enabled

  $ engram telemetry status
  Telemetry: Disabled (opt-out)

  $ engram telemetry status
  Telemetry: Enabled (enforced by organization)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := config.NewLoader()
		cfg, err := loader.Load()
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		// Check enforcement
		if cfg.Telemetry.Enforce {
			if cfg.Telemetry.Enabled {
				fmt.Println("Telemetry: Enabled (enforced by organization)")
				fmt.Println("\nNote: Your organization requires telemetry for compliance.")
				fmt.Println("Contact your IT administrator to request opt-out.")
			} else {
				// This shouldn't happen (enforce=true but enabled=false)
				fmt.Println("Telemetry: Disabled (configuration error)")
				fmt.Println("\nWarning: Enforcement is set but telemetry is disabled.")
			}
		} else {
			if cfg.Telemetry.Enabled {
				fmt.Println("Telemetry: Enabled")
				fmt.Println("\nTo opt-out, run: engram telemetry disable")
			} else {
				fmt.Println("Telemetry: Disabled")
				fmt.Println("\nTo enable, run: engram telemetry enable")
			}
		}

		return nil
	},
}

var telemetryEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable telemetry collection",
	Long: `Enable telemetry collection and save preference to user config.

Telemetry helps improve Engram by collecting anonymous usage data:
  - Which engrams are most useful
  - Which plugins are used most often
  - Performance metrics (duration, token usage)
  - Error rates and failure patterns

What is NOT collected:
  - Code content or snippets
  - File paths or project names
  - User queries or prompts
  - AI responses or generated text
  - Personal information

All data is stored locally in ~/.engram/telemetry/ unless your
organization has configured a remote collection endpoint.

EXAMPLES
  $ engram telemetry enable
  Telemetry enabled successfully.
  Data will be stored in: ~/.engram/telemetry/`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return setTelemetryEnabled(true)
	},
}

var telemetryDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable telemetry collection (GDPR opt-out)",
	Long: `Disable telemetry collection and save preference to user config.

This is your GDPR opt-out mechanism. After disabling:
  - No new telemetry data will be collected
  - Existing telemetry files are NOT deleted (you control your data)
  - You can re-enable at any time with 'engram telemetry enable'

Note: If your organization enforces telemetry, you cannot disable it
through this command. Contact your IT administrator to request opt-out.

EXAMPLES
  $ engram telemetry disable
  Telemetry disabled successfully.

  $ engram telemetry disable
  Error: Telemetry is enforced by organization policy.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check if telemetry is enforced before disabling
		loader := config.NewLoader()
		cfg, err := loader.Load()
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		if cfg.Telemetry.Enforce {
			//nolint:staticcheck // multi-line CLI-facing help text
			return fmt.Errorf("cannot disable telemetry: enforced by organization policy\n\n" +
				"Your organization requires telemetry for compliance.\n" +
				"Contact your IT administrator to request opt-out.")
		}

		return setTelemetryEnabled(false)
	},
}

var telemetryShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show detailed telemetry configuration",
	Long: `Display the complete telemetry configuration including:
  - Enabled status
  - Storage location
  - Enforcement policy
  - Retention settings

EXAMPLES
  $ engram telemetry show
  Telemetry Configuration:
    Enabled: true
    Enforced: false
    Path: ~/.engram/telemetry/
    Max Size: 100 MB
    Retention: 90 days`,
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := config.NewLoader()
		cfg, err := loader.Load()
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		fmt.Println("Telemetry Configuration:")
		fmt.Printf("  Enabled: %t\n", cfg.Telemetry.Enabled)
		fmt.Printf("  Enforced: %t\n", cfg.Telemetry.Enforce)

		if cfg.Telemetry.Path != "" {
			fmt.Printf("  Path: %s\n", cfg.Telemetry.Path)
		} else {
			fmt.Printf("  Path: ~/.engram/telemetry/ (default)\n")
		}

		if cfg.Telemetry.MaxSizeMB > 0 {
			fmt.Printf("  Max Size: %d MB\n", cfg.Telemetry.MaxSizeMB)
		}

		if cfg.Telemetry.RetentionDays > 0 {
			fmt.Printf("  Retention: %d days\n", cfg.Telemetry.RetentionDays)
		}

		if cfg.Telemetry.Storage != "" {
			fmt.Printf("  Storage: %s\n", cfg.Telemetry.Storage)
		}

		// Show enforcement message if applicable
		if cfg.Telemetry.Enforce {
			fmt.Println("\nNote: Telemetry is enforced by your organization.")
			fmt.Println("You cannot disable it through user configuration.")
		}

		return nil
	},
}

var (
	loadedFormat   string
	loadedDetailed bool
)

var telemetryLoadedCmd = &cobra.Command{
	Use:   "loaded",
	Short: "Show loaded engrams for current session",
	Long: `Show engrams that were loaded into the current Claude Code session
(within the last 30 minutes). Useful for debugging which guidance is
active and logging violation attribution.

USAGE
  engram telemetry loaded [flags]

FLAGS
  --format string    Output format: table or json (default "table")
  --detailed         Show additional fields (load_when, timestamp)
  -h, --help         help for loaded

EXAMPLES
  # Show loaded engrams (table format)
  $ engram telemetry loaded
  Title                          Version      Hash
  bash-command-simplification    v1.0-light   sha256:267cb6e1f30998c6abc123...
  claude-code-tool-usage         v1.0-light   sha256:89abcdef01234567890abc...

  # JSON output for scripting
  $ engram telemetry loaded --format json | jq '.[] | select(.title | contains("bash"))'

  # Detailed output with timestamps
  $ engram telemetry loaded --detailed
  Title                          Version      Hash                                  Load When    Loaded At
  bash-command-simplification    v1.0-light   sha256:267cb6e1f30998c6abc123...      always       2026-01-07T05:10:15Z

OUTPUT FORMAT
  Table: Title, Version, Hash (default)
  JSON:  Array of objects with title, version, hash, loaded_at

NOTES
  - Session detection uses 30-minute window (last 30 min)
  - Requires telemetry enabled (engram telemetry enable)
  - Hash format matches phase_engram_hash in Wayfinder deliverables`,
	RunE: runTelemetryLoaded,
}

func init() {
	telemetryLoadedCmd.Flags().StringVar(&loadedFormat, "format", "table", "Output format (table|json)")
	telemetryLoadedCmd.Flags().BoolVar(&loadedDetailed, "detailed", false, "Show detailed info")
}

func init() {
	rootCmd.AddCommand(telemetryCmd)
	telemetryCmd.AddCommand(telemetryStatusCmd)
	telemetryCmd.AddCommand(telemetryEnableCmd)
	telemetryCmd.AddCommand(telemetryDisableCmd)
	telemetryCmd.AddCommand(telemetryShowCmd)
	telemetryCmd.AddCommand(telemetryLoadedCmd)
}

// setTelemetryEnabled updates the user config file to enable/disable telemetry
func setTelemetryEnabled(enabled bool) error {
	// Get user config path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	userConfigPath := filepath.Join(homeDir, ".engram", "config.yaml")

	// Create .engram directory if it doesn't exist
	engramDir := filepath.Dir(userConfigPath)
	if err := os.MkdirAll(engramDir, 0755); err != nil {
		return fmt.Errorf("failed to create .engram directory: %w", err)
	}

	// Read existing user config (if exists)
	var userConfig config.Config
	if data, err := os.ReadFile(userConfigPath); err == nil {
		// Config exists, parse it
		if err := yaml.Unmarshal(data, &userConfig); err != nil {
			return fmt.Errorf("failed to parse user config: %w", err)
		}
	}

	// Update telemetry setting
	userConfig.Telemetry.Enabled = enabled

	// Write back to file
	data, err := yaml.Marshal(&userConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(userConfigPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write user config: %w", err)
	}

	// Success message
	if enabled {
		fmt.Println("Telemetry enabled successfully.")
		fmt.Println("\nData will be stored in: ~/.engram/telemetry/")
		fmt.Println("To opt-out later, run: engram telemetry disable")
	} else {
		fmt.Println("Telemetry disabled successfully.")
		fmt.Println("\nNo new telemetry data will be collected.")
		fmt.Println("Existing telemetry files have NOT been deleted.")
		fmt.Println("To re-enable, run: engram telemetry enable")
	}

	return nil
}

// LoadedEngram represents an engram loaded in the current session
type LoadedEngram struct {
	Title    string    `json:"title"`
	Version  string    `json:"version"`
	Hash     string    `json:"hash"`
	LoadWhen string    `json:"load_when,omitempty"`
	LoadedAt time.Time `json:"loaded_at"`
}

// runTelemetryLoaded shows loaded engrams for current session
func runTelemetryLoaded(cmd *cobra.Command, args []string) error {
	// Get telemetry path
	telemetryPath := getTelemetryPath()

	// Check file exists
	if _, err := os.Stat(telemetryPath); os.IsNotExist(err) {
		return fmt.Errorf("telemetry file not found\n" +
			"Telemetry may be disabled or not yet initialized.\n\n" +
			"To enable telemetry: engram telemetry enable")
	} else if err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied reading telemetry file: %s", telemetryPath)
		}
		return fmt.Errorf("failed to access telemetry file: %w", err)
	}

	// Parse JSONL
	events, errs := analysis.ParseJSONL(telemetryPath)

	// Filter and extract loaded engrams
	engrams, err := filterLoadedEngrams(events, errs)
	if err != nil {
		return err
	}

	// Handle empty state
	if len(engrams) == 0 {
		fmt.Println("No engrams loaded in current session (last 30 minutes).")
		return nil
	}

	// Format output
	return formatOutput(engrams, loadedFormat, loadedDetailed)
}

// filterLoadedEngrams filters events for engram_loaded type within session window
func filterLoadedEngrams(events <-chan *analysis.TelemetryEvent, errs <-chan error) ([]LoadedEngram, error) {
	sessionCutoff := time.Now().Add(-30 * time.Minute)
	var engrams []LoadedEngram

	// Process events
	for event := range events {
		// Skip non-engram_loaded events
		if event.Type != "engram_loaded" {
			continue
		}

		// Skip events outside session window
		if event.Timestamp.Before(sessionCutoff) {
			continue
		}

		// Extract fields
		engram, err := extractEngramFields(event)
		if err != nil {
			// Skip malformed events (logged but not fatal)
			continue
		}

		engrams = append(engrams, engram)
	}

	// Check for parsing errors
	for err := range errs {
		if err != nil {
			// Log error but continue (parser is resilient)
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
	}

	return engrams, nil
}

// extractEngramFields extracts required fields from telemetry event
func extractEngramFields(event *analysis.TelemetryEvent) (LoadedEngram, error) {
	// Extract required fields with defensive type assertions
	title, ok := event.Data["title"].(string)
	if !ok || title == "" {
		return LoadedEngram{}, fmt.Errorf("missing or invalid title field")
	}

	version, ok := event.Data["version"].(string)
	if !ok || version == "" {
		return LoadedEngram{}, fmt.Errorf("missing or invalid version field")
	}

	hash, ok := event.Data["hash"].(string)
	if !ok || hash == "" {
		return LoadedEngram{}, fmt.Errorf("missing or invalid hash field")
	}

	// Extract optional field
	loadWhen, _ := event.Data["load_when"].(string)

	return LoadedEngram{
		Title:    title,
		Version:  version,
		Hash:     hash,
		LoadWhen: loadWhen,
		LoadedAt: event.Timestamp,
	}, nil
}

// formatOutput formats engrams as table or JSON
func formatOutput(engrams []LoadedEngram, format string, detailed bool) error {
	if format == "json" {
		return formatJSON(engrams)
	}
	return formatTable(engrams, detailed)
}

// formatTable formats engrams as aligned table
func formatTable(engrams []LoadedEngram, detailed bool) error {
	var tbl *table.Table

	if detailed {
		tbl = table.New([]string{"Title", "Version", "Hash", "Load When", "Loaded At"})
		for _, e := range engrams {
			tbl.AddRow(
				e.Title,
				e.Version,
				e.Hash,
				e.LoadWhen,
				e.LoadedAt.Format(time.RFC3339),
			)
		}
	} else {
		tbl = table.New([]string{"Title", "Version", "Hash"})
		for _, e := range engrams {
			tbl.AddRow(e.Title, e.Version, e.Hash)
		}
	}

	return tbl.Print()
}

// formatJSON formats engrams as JSON array
func formatJSON(engrams []LoadedEngram) error {
	output, err := json.MarshalIndent(engrams, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	fmt.Println(string(output))
	return nil
}
