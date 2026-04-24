// Package devlog provides the devlog CLI tool.
package devlog

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/cliframe"
	"github.com/vbonnet/dear-agent/tools/devlog/internal/config"
	"gopkg.in/yaml.v3"
)

var initCmd = &cobra.Command{
	Use:   "init [workspace-name]",
	Short: "Initialize a new devlog workspace",
	Long: `Initialize creates a new devlog workspace with:
  - .devlog/config.yaml (base configuration)
  - .devlog/config.local.yaml.example (template for local overrides)
  - .devlog/README.md (documentation)

If workspace-name is not provided, uses the current directory name.

The generated config.yaml includes:
  - Example repository configuration
  - Example worktree setup
  - Comments explaining all options

After initialization, edit .devlog/config.yaml to configure your repositories,
then run 'devlog sync' to clone repos and create worktrees.`,
	RunE: runInit,
}

var (
	// initForce allows overwriting existing config
	initForce bool
)

func runInit(cmd *cobra.Command, args []string) error {
	flags := GetCommonFlags()
	writer := cliframe.NewWriter(cmd.OutOrStdout(), cmd.ErrOrStderr())
	writer.SetColorEnabled(!flags.NoColor)

	// Determine workspace name
	workspaceName := "my-workspace"
	if len(args) > 0 {
		workspaceName = args[0]
	} else {
		// Use current directory name
		cwd, err := os.Getwd()
		if err == nil {
			workspaceName = filepath.Base(cwd)
		}
	}

	if flags.Verbose {
		writer.Info(fmt.Sprintf("Initializing devlog workspace: %s", workspaceName))
	}

	// Check if .devlog already exists
	devlogDir := ".devlog"
	configPath := filepath.Join(devlogDir, "config.yaml")

	if _, err := os.Stat(configPath); err == nil && !initForce {
		return cliframe.NewError("workspace_exists",
			fmt.Sprintf("Workspace already initialized (config file exists at %s)", configPath)).
			AddSuggestion("Use --force to overwrite existing configuration").
			WithExitCode(cliframe.ExitUsageError)
	}

	// Create .devlog directory
	if err := os.MkdirAll(devlogDir, 0750); err != nil {
		return cliframe.NewError("create_directory_failed",
			"Failed to create .devlog directory").
			WithCause(err).
			WithExitCode(cliframe.ExitIOError)
	}
	writer.Success("Created .devlog directory")

	// Generate example configuration
	exampleConfig := createExampleConfig(workspaceName)

	// Write config.yaml
	data, err := yaml.Marshal(exampleConfig)
	if err != nil {
		return cliframe.NewError("marshal_config_failed",
			"Failed to marshal configuration").
			WithCause(err).
			WithExitCode(cliframe.ExitInternalError)
	}

	// Add header comment to config.yaml
	configContent := `# devlog workspace configuration
#
# This file defines your development workspace with multiple git repositories
# and their worktrees for parallel feature development.
#
# After editing this file, run 'devlog sync' to clone repos and create worktrees.

` + string(data)

	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		return cliframe.NewError("write_config_failed",
			"Failed to write config.yaml").
			WithCause(err).
			WithExitCode(cliframe.ExitIOError)
	}
	writer.Success("Created .devlog/config.yaml")

	// Create config.local.yaml.example
	localExamplePath := filepath.Join(devlogDir, "config.local.yaml.example")
	localExampleContent := `# devlog local configuration (example)
#
# This file demonstrates local overrides that extend the base config.yaml.
# Copy this file to config.local.yaml (which is git-ignored) to use it.
#
# Local config is merged additively with base config:
# - Add new repositories
# - Add worktrees to existing repositories
# - Override workspace metadata (name, description)
#
# Example: Add a local-only repository
# repos:
#   - name: my-local-repo
#     url: https://github.com/me/my-repo.git
#     type: bare
#     worktrees:
#       - name: main
#         branch: main
#
# Example: Add worktrees to an existing repository
# repos:
#   - name: existing-repo  # Must match name in config.yaml
#     worktrees:
#       - name: my-feature
#         branch: feature/my-work
`

	if err := os.WriteFile(localExamplePath, []byte(localExampleContent), 0600); err != nil {
		writer.Warning(fmt.Sprintf("Warning: failed to write config.local.yaml.example: %v", err))
	} else {
		writer.Success("Created .devlog/config.local.yaml.example")
	}

	// Create README.md
	readmePath := filepath.Join(devlogDir, "README.md")
	readmeContent := fmt.Sprintf(`# %s - devlog Workspace

This directory contains configuration for your devlog development workspace.

## Files

- **config.yaml** - Base workspace configuration (commit to git)
- **config.local.yaml** - Local overrides (git-ignored, not committed)
- **config.local.yaml.example** - Template for local configuration

## Quick Start

1. Edit config.yaml to add your repositories:
   ~~~yaml
   repos:
     - name: my-repo
       url: https://github.com/user/repo.git
       type: bare
       worktrees:
         - name: main
           branch: main
         - name: feature
           branch: feature-branch
   ~~~

2. Run sync to clone repos and create worktrees:
   ~~~
   devlog sync
   ~~~

3. (Optional) Create config.local.yaml for personal worktrees:
   ~~~
   cp config.local.yaml.example config.local.yaml
   # Edit config.local.yaml
   devlog sync
   ~~~

## Commands

- **devlog sync** - Clone repos and create worktrees from config
- **devlog status** - Show current workspace state
- **devlog init** - Initialize a new workspace (this command)

## Configuration

See config.yaml for detailed configuration options and examples.
`, workspaceName)

	if err := os.WriteFile(readmePath, []byte(readmeContent), 0600); err != nil {
		writer.Warning(fmt.Sprintf("Warning: failed to write README.md: %v", err))
	} else {
		writer.Success("Created .devlog/README.md")
	}

	// Create .gitignore in .devlog/
	gitignorePath := filepath.Join(devlogDir, ".gitignore")
	gitignoreContent := `# Ignore local configuration (not committed to git)
config.local.yaml

# Ignore editor files
*.swp
*.swo
*~
.DS_Store
`

	if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0600); err != nil { //nolint:gosec // .gitignore is not sensitive
		writer.Warning(fmt.Sprintf("Warning: failed to write .gitignore: %v", err))
	} else {
		writer.Success("Created .devlog/.gitignore")
	}

	// Success summary
	if !flags.Quiet {
		writer.Info("")
		writer.Success(fmt.Sprintf("Initialized devlog workspace: %s", workspaceName))
		writer.Info("")
		writer.Info("Next steps:")
		writer.Info("  1. Edit .devlog/config.yaml to configure your repositories")
		writer.Info("  2. Run 'devlog sync' to clone repos and create worktrees")
		writer.Info("  3. (Optional) Create .devlog/config.local.yaml for personal overrides")
	}

	return nil
}

// createExampleConfig generates an example configuration with helpful comments.
func createExampleConfig(workspaceName string) *config.Config {
	return &config.Config{
		Name:        workspaceName,
		Description: "Development workspace with multiple repositories and worktrees",
		Repos: []config.Repo{
			{
				Name: "example-repo",
				URL:  "https://github.com/user/repo.git",
				Type: config.RepoTypeBare,
				Worktrees: []config.Worktree{
					{
						Name:   "main",
						Branch: "main",
					},
					{
						Name:   "feature",
						Branch: "feature-branch",
					},
				},
			},
		},
	}
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolVar(&initForce, "force", false, "overwrite existing configuration")
}
