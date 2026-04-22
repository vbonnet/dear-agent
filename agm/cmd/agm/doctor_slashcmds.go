package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// slashCmdCheckResult holds the result of checking a single slash command file.
type slashCmdCheckResult struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	Source      string `json:"source"` // "user" or plugin name
	Healthy     bool   `json:"healthy"`
	Error       string `json:"error,omitempty"`
	Description string `json:"description,omitempty"`
}

// slashCmdReport is the aggregate health report.
type slashCmdReport struct {
	Total   int                   `json:"total"`
	Healthy int                   `json:"healthy"`
	Errors  int                   `json:"errors"`
	Results []slashCmdCheckResult `json:"results"`
}

var doctorSlashCmdsJSON bool

var doctorSlashCmdsCmd = &cobra.Command{
	Use:   "slash-commands",
	Short: "Check slash command health",
	Long: `Scan user and plugin slash command directories for .md command files,
verify they are readable, parse YAML frontmatter, and check for required
fields (description). Reports a summary of total, healthy, and errored commands.

Also checks whether enabledPlugins in ~/.claude/settings.json reference
directories that actually contain command files.

Examples:
  agm admin doctor slash-commands
  agm admin doctor slash-commands --json`,
	RunE: runDoctorSlashCmds,
}

func init() {
	doctorCmd.AddCommand(doctorSlashCmdsCmd)
	doctorSlashCmdsCmd.Flags().BoolVar(&doctorSlashCmdsJSON, "json", false,
		"Output results as JSON")
}

// runDoctorSlashCmds is the main entry point for the slash-commands doctor check.
func runDoctorSlashCmds(cmd *cobra.Command, args []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	report := &slashCmdReport{}

	// 1. Scan ~/.claude/commands/ for user command .md files
	userCmdsDir := filepath.Join(homeDir, ".claude", "commands")
	scanSlashCmdDir(userCmdsDir, "user", report)

	// 2. Scan plugin command directories from settings.json enabledPlugins
	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	pluginDirs := resolvePluginCommandDirs(settingsPath, homeDir)
	for pluginName, cmdDir := range pluginDirs {
		scanSlashCmdDir(cmdDir, pluginName, report)
	}

	// 3. Output
	if doctorSlashCmdsJSON {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal report: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	// Text output
	fmt.Println("=== Slash Command Health Check ===")
	fmt.Println()

	for _, r := range report.Results {
		if r.Healthy {
			fmt.Printf("  OK  %s (%s)\n", r.Name, r.Source)
		} else {
			fmt.Printf("  ERR %s (%s): %s\n", r.Name, r.Source, r.Error)
		}
	}

	fmt.Println()
	fmt.Printf("Total: %d  Healthy: %d  Errors: %d\n", report.Total, report.Healthy, report.Errors)

	if report.Errors > 0 {
		return fmt.Errorf("slash command health check found %d error(s)", report.Errors)
	}
	return nil
}

// scanSlashCmdDir scans a directory for .md command files and checks each one.
func scanSlashCmdDir(dir, source string, report *slashCmdReport) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		// Directory doesn't exist or isn't readable — not an error for the
		// report; it just means there are no commands from this source.
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		result := checkSlashCmdFile(filePath, source)
		report.Results = append(report.Results, result)
		report.Total++
		if result.Healthy {
			report.Healthy++
		} else {
			report.Errors++
		}
	}
}

// checkSlashCmdFile verifies a single slash command file is readable and has
// valid frontmatter with a description field.
func checkSlashCmdFile(path, source string) slashCmdCheckResult {
	name := strings.TrimSuffix(filepath.Base(path), ".md")
	result := slashCmdCheckResult{
		Path:   path,
		Name:   name,
		Source: source,
	}

	content, err := os.ReadFile(path)
	if err != nil {
		result.Error = fmt.Sprintf("unreadable: %v", err)
		return result
	}

	// Parse frontmatter
	fm, parseErr := parseSlashCmdFrontmatter(content)
	if parseErr != nil {
		result.Error = parseErr.Error()
		return result
	}

	desc, _ := fm["description"].(string)
	if desc == "" {
		// No frontmatter at all is acceptable (plain markdown command),
		// but if frontmatter exists and description is missing, warn.
		if fm != nil && len(fm) > 0 {
			result.Error = "frontmatter present but missing required field: description"
			return result
		}
	}

	result.Description = desc
	result.Healthy = true
	return result
}

// parseSlashCmdFrontmatter extracts YAML frontmatter from a markdown file.
// Returns nil map with nil error if no frontmatter is present.
func parseSlashCmdFrontmatter(content []byte) (map[string]interface{}, error) {
	text := string(content)

	if !strings.HasPrefix(text, "---\n") {
		// No frontmatter — that's fine
		return nil, nil
	}

	endIdx := strings.Index(text[4:], "\n---")
	if endIdx == -1 {
		return nil, fmt.Errorf("unclosed frontmatter block")
	}

	fmYAML := text[4 : 4+endIdx]

	var fm map[string]interface{}
	if err := yaml.Unmarshal([]byte(fmYAML), &fm); err != nil {
		return nil, fmt.Errorf("malformed frontmatter YAML: %w", err)
	}

	return fm, nil
}

// resolvePluginCommandDirs reads settings.json and returns a map of
// plugin-name -> commands-directory for each enabled plugin that has a
// commands/ subdirectory.
func resolvePluginCommandDirs(settingsPath, homeDir string) map[string]string {
	dirs := make(map[string]string)

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return dirs
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return dirs
	}

	plugins, ok := settings["enabledPlugins"]
	if !ok {
		return dirs
	}

	pluginList, ok := plugins.([]interface{})
	if !ok {
		return dirs
	}

	for _, p := range pluginList {
		pluginPath, ok := p.(string)
		if !ok {
			continue
		}

		// Expand ~ if present
		if strings.HasPrefix(pluginPath, "~") {
			pluginPath = filepath.Join(homeDir, pluginPath[1:])
		}

		// Check for commands/ subdirectory
		cmdsDir := filepath.Join(pluginPath, "commands")
		if info, err := os.Stat(cmdsDir); err == nil && info.IsDir() {
			pluginName := filepath.Base(pluginPath)
			dirs[pluginName] = cmdsDir
		}
	}

	return dirs
}
