package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/cmd/engram/internal/cli"
	"github.com/vbonnet/dear-agent/engram/internal/slashcmd"
)

var slashcmdCmd = &cobra.Command{
	Use:   "slashcmd",
	Short: "Slash command utilities",
	Long:  `Utilities for working with enhanced slash commands.`,
}

var parseCmd = &cobra.Command{
	Use:   "parse COMMAND_FILE",
	Short: "Parse and display slash command metadata",
	Long: `Parse a slash command file and display its frontmatter metadata.

Example:
  engram slashcmd parse ~/.claude/commands/engram-search.md`,
	Args: cobra.ExactArgs(1),
	RunE: runParse,
}

var autocompleteCmd = &cobra.Command{
	Use:   "autocomplete COMMAND_FILE PARAM_NAME",
	Short: "Get autocomplete values for a parameter",
	Long: `Execute autocomplete command for a parameter and return values.

Example:
  engram slashcmd autocomplete ~/.claude/commands/engram-search.md type`,
	Args: cobra.ExactArgs(2),
	RunE: runAutocomplete,
}

func init() {
	rootCmd.AddCommand(slashcmdCmd)
	slashcmdCmd.AddCommand(parseCmd)
	slashcmdCmd.AddCommand(autocompleteCmd)
}

func runParse(cmd *cobra.Command, args []string) error {
	commandFile := args[0]

	// Expand and validate path (prevent path traversal attacks)
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	allowedPaths := []string{
		filepath.Join(home, ".claude", "commands"),
		filepath.Join(home, ".claude"),
	}

	expandedPath := commandFile
	if len(commandFile) > 0 && commandFile[0] == '~' {
		expandedPath = filepath.Join(home, commandFile[1:])
	}

	if err := cli.ValidateSafePath("command-file", expandedPath, allowedPaths); err != nil {
		return err
	}

	commandFile = expandedPath

	// Parse command
	slashCmd, err := slashcmd.ParseCommand(commandFile)
	if err != nil {
		return fmt.Errorf("failed to parse command: %w", err)
	}

	// Display as formatted JSON
	data, err := json.MarshalIndent(slashCmd, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}

	fmt.Println(string(data))
	return nil
}

func runAutocomplete(cmd *cobra.Command, args []string) error {
	commandFile := args[0]
	paramName := args[1]

	// Expand and validate path (prevent path traversal attacks)
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	allowedPaths := []string{
		filepath.Join(home, ".claude", "commands"),
		filepath.Join(home, ".claude"),
	}

	expandedPath := commandFile
	if len(commandFile) > 0 && commandFile[0] == '~' {
		expandedPath = filepath.Join(home, commandFile[1:])
	}

	if err := cli.ValidateSafePath("command-file", expandedPath, allowedPaths); err != nil {
		return err
	}

	commandFile = expandedPath

	// Parse command
	slashCmd, err := slashcmd.ParseCommand(commandFile)
	if err != nil {
		return fmt.Errorf("failed to parse command: %w", err)
	}

	// Get autocomplete values
	values, err := slashCmd.AutocompleteProvider(paramName)
	if err != nil {
		return fmt.Errorf("failed to get autocomplete values: %w", err)
	}

	// Display one per line
	for _, value := range values {
		fmt.Println(value)
	}

	return nil
}
