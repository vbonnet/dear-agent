package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

var installHarnessCmd = &cobra.Command{
	Use:   "install-harness <harness>",
	Short: "Install a coding agent harness (codex, gemini, or opencode)",
	Long: `Install a coding agent harness CLI.

Supports:
  codex     - OpenAI Codex CLI (installed via npm)
  gemini    - Google Gemini CLI (installed via linuxbrew)
  opencode  - Mistral OpenCode CLI (installed via linuxbrew)

The command checks if the harness is already installed before attempting installation.
Output is in JSON format for programmatic use.

Examples:
  agm admin install-harness codex
  agm admin install-harness gemini
  agm admin install-harness opencode
  agm admin install-harness codex --json`,
	Args:      cobra.ExactArgs(1),
	RunE:      runInstallHarness,
	ValidArgs: []string{"codex", "gemini", "opencode"},
}

var installHarnessJSON bool
var installHarnessQuiet bool

func init() {
	adminCmd.AddCommand(installHarnessCmd)
	installHarnessCmd.Flags().BoolVar(&installHarnessJSON, "json", true,
		"Output result as JSON (default: true)")
	installHarnessCmd.Flags().BoolVar(&installHarnessQuiet, "quiet", false,
		"Suppress output (only valid with --json)")
}

func runInstallHarness(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("harness name required")
	}

	harnessStr := args[0]

	// Validate harness type
	harness, err := ops.ValidateHarness(harnessStr)
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Install the harness
	result, err := ops.Install(ctx, harness)
	if err != nil {
		return err
	}

	// Output result
	if installHarnessJSON {
		jsonOutput, err := ops.ResultToJSON(result)
		if err != nil {
			return fmt.Errorf("failed to marshal result to JSON: %w", err)
		}

		if !installHarnessQuiet {
			fmt.Println(jsonOutput)
		}

		// Exit with appropriate code
		if !result.Success {
			os.Exit(1)
		}
	} else {
		// Human-readable output
		if result.Success {
			fmt.Printf("✓ %s\n", result.Message)
			if result.Version != "" {
				fmt.Printf("  Version: %s\n", result.Version)
			}
			if result.Path != "" {
				fmt.Printf("  Path: %s\n", result.Path)
			}
		} else {
			fmt.Fprintf(os.Stderr, "✗ %s\n", result.Message)
			if result.ErrorDetails != "" {
				fmt.Fprintf(os.Stderr, "  Details: %s\n", result.ErrorDetails)
			}
			os.Exit(1)
		}
	}

	return nil
}

// installCodexCmd is a convenience command for installing just Codex
var installCodexCmd = &cobra.Command{
	Use:   "install-codex",
	Short: "Install Codex CLI (shorthand for install-harness codex)",
	Long: `Install OpenAI's Codex CLI.

The Codex CLI will be installed via npm (npm install -g @openai/codex).
Output is in JSON format.

Example:
  agm admin install-codex`,
	Args:  cobra.NoArgs,
	RunE:  runInstallCodex,
	Hidden: true, // Hidden since install-harness is the canonical command
}

func init() {
	adminCmd.AddCommand(installCodexCmd)
	installCodexCmd.Flags().BoolVar(&installHarnessJSON, "json", true,
		"Output result as JSON (default: true)")
	installCodexCmd.Flags().BoolVar(&installHarnessQuiet, "quiet", false,
		"Suppress output (only valid with --json)")
}

func runInstallCodex(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	result := ops.InstallCodex(ctx)
	if result == nil {
		return fmt.Errorf("failed to install Codex")
	}

	// Output result
	if installHarnessJSON {
		jsonOutput, err := ops.ResultToJSON(result)
		if err != nil {
			return fmt.Errorf("failed to marshal result to JSON: %w", err)
		}

		if !installHarnessQuiet {
			fmt.Println(jsonOutput)
		}

		if !result.Success {
			os.Exit(1)
		}
	} else {
		if result.Success {
			fmt.Printf("✓ %s\n", result.Message)
			if result.Version != "" {
				fmt.Printf("  Version: %s\n", result.Version)
			}
			if result.Path != "" {
				fmt.Printf("  Path: %s\n", result.Path)
			}
		} else {
			fmt.Fprintf(os.Stderr, "✗ %s\n", result.Message)
			if result.ErrorDetails != "" {
				fmt.Fprintf(os.Stderr, "  Details: %s\n", result.ErrorDetails)
			}
			os.Exit(1)
		}
	}

	return nil
}
