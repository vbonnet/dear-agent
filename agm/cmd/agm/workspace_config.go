package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	"gopkg.in/yaml.v3"
)

const dearAgentConfigFile = ".dear-agent-workspace.yaml"

// DearAgentWorkspaceConfig represents the workspace-level configuration
// for dear-agent orchestration. This file lives at the workspace root
// alongside .agm/ and defines repos, goals, and review gates.
type DearAgentWorkspaceConfig struct {
	Version     int                        `yaml:"version"`
	Repos       []string                   `yaml:"repos"`
	Goals       map[string]string          `yaml:"goals,omitempty"`
	ReviewGates map[string]ReviewGateConfig `yaml:"review_gates,omitempty"`
}

// ReviewGateConfig defines review requirements for a change type.
type ReviewGateConfig struct {
	Required    bool     `yaml:"required"`
	Reviewers   int      `yaml:"reviewers"`
	CheckSuites []string `yaml:"check_suites,omitempty"`
}

// LoadDearAgentConfig reads .dear-agent-workspace.yaml from the given directory.
func LoadDearAgentConfig(dir string) (*DearAgentWorkspaceConfig, error) {
	path := filepath.Join(dir, dearAgentConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", dearAgentConfigFile, err)
	}

	var config DearAgentWorkspaceConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", dearAgentConfigFile, err)
	}

	if config.Version == 0 {
		config.Version = 1
	}

	return &config, nil
}

// WriteDearAgentConfig writes .dear-agent-workspace.yaml to the given directory.
func WriteDearAgentConfig(dir string, config *DearAgentWorkspaceConfig) error {
	path := filepath.Join(dir, dearAgentConfigFile)
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	header := "# dear-agent workspace configuration\n# See: agm workspace init --help\n\n"
	if err := os.WriteFile(path, []byte(header+string(data)), 0600); err != nil {
		return fmt.Errorf("failed to write %s: %w", dearAgentConfigFile, err)
	}

	return nil
}

// templateDearAgentConfig returns a starter config for workspace init.
func templateDearAgentConfig() *DearAgentWorkspaceConfig {
	return &DearAgentWorkspaceConfig{
		Version: 1,
		Repos:   []string{"./repo-a", "./repo-b"},
		Goals: map[string]string{
			"repo-a": "Primary application code",
			"repo-b": "Shared libraries",
		},
		ReviewGates: map[string]ReviewGateConfig{
			"feature": {
				Required:    true,
				Reviewers:   1,
				CheckSuites: []string{"ci/build", "ci/test"},
			},
			"bugfix": {
				Required:    true,
				Reviewers:   1,
				CheckSuites: []string{"ci/test"},
			},
			"docs": {
				Required:  false,
				Reviewers: 0,
			},
		},
	}
}

var workspaceInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a template .dear-agent-workspace.yaml",
	Long: `Initialize a dear-agent workspace configuration file in the current directory.

Creates a .dear-agent-workspace.yaml with template values for repos, goals,
and review gates. Edit the generated file to match your workspace layout.

The config file defines:
  repos:         List of repository paths relative to workspace root
  goals:         Map of repo name to its purpose/goal
  review_gates:  Review requirements per change type (feature, bugfix, docs, etc.)

Examples:
  agm workspace init              # Create in current directory
  agm workspace init --dir ~/ws   # Create in specified directory`,
	RunE: runWorkspaceInit,
}

var workspaceInitDir string

func runWorkspaceInit(cmd *cobra.Command, _ []string) error {
	dir := workspaceInitDir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	// Expand ~ in path
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to expand home dir: %w", err)
		}
		dir = filepath.Join(home, dir[2:])
	}

	configPath := filepath.Join(dir, dearAgentConfigFile)
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("%s already exists in %s", dearAgentConfigFile, dir)
	}

	config := templateDearAgentConfig()
	if err := WriteDearAgentConfig(dir, config); err != nil {
		return err
	}

	ui.PrintSuccess(fmt.Sprintf("Created %s in %s", dearAgentConfigFile, dir))
	fmt.Printf("\nEdit the file to configure your workspace repos, goals, and review gates.\n")
	return nil
}

var workspaceConfigShowCmd = &cobra.Command{
	Use:   "config",
	Short: "Display dear-agent workspace configuration",
	Long: `Show the dear-agent workspace configuration from .dear-agent-workspace.yaml
in the current directory or workspace root.

Displays repos, goals, and review gate settings.

Examples:
  agm workspace config              # Show config from current directory
  agm workspace config --dir ~/ws   # Show config from specified directory`,
	RunE: runWorkspaceConfigShow,
}

var workspaceConfigShowDir string

func runWorkspaceConfigShow(cmd *cobra.Command, _ []string) error {
	dir := workspaceConfigShowDir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	// Expand ~ in path
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to expand home dir: %w", err)
		}
		dir = filepath.Join(home, dir[2:])
	}

	config, err := LoadDearAgentConfig(dir)
	if err != nil {
		return fmt.Errorf("no workspace config found: %w", err)
	}

	fmt.Printf("\n")
	fmt.Printf("Workspace Config: %s\n", filepath.Join(dir, dearAgentConfigFile))
	fmt.Printf("Version:          %d\n", config.Version)

	// Repos
	fmt.Printf("\nRepos (%d):\n", len(config.Repos))
	for _, repo := range config.Repos {
		goal, hasGoal := config.Goals[filepath.Base(repo)]
		if !hasGoal {
			goal, hasGoal = config.Goals[repo]
		}
		if hasGoal {
			fmt.Printf("  • %s — %s\n", ui.Blue(repo), goal)
		} else {
			fmt.Printf("  • %s\n", ui.Blue(repo))
		}
	}

	// Review gates
	if len(config.ReviewGates) > 0 {
		fmt.Printf("\nReview Gates:\n")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "  TYPE\tREQUIRED\tREVIEWERS\tCHECKS\n")
		for changeType, gate := range config.ReviewGates {
			required := "no"
			if gate.Required {
				required = "yes"
			}
			checks := "-"
			if len(gate.CheckSuites) > 0 {
				checks = strings.Join(gate.CheckSuites, ", ")
			}
			fmt.Fprintf(w, "  %s\t%s\t%d\t%s\n", changeType, required, gate.Reviewers, checks)
		}
		w.Flush()
	}

	fmt.Printf("\n")
	return nil
}

func init() {
	workspaceInitCmd.Flags().StringVar(&workspaceInitDir, "dir", "", "Directory to create config in (default: current directory)")
	workspaceCmd.AddCommand(workspaceInitCmd)

	workspaceConfigShowCmd.Flags().StringVar(&workspaceConfigShowDir, "dir", "", "Directory to read config from (default: current directory)")
	workspaceCmd.AddCommand(workspaceConfigShowCmd)
}
