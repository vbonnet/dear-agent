package cmd

import (
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage Engram configuration",
	Long: `Configuration management commands for Engram.

Engram uses a 4-tier configuration hierarchy:
  1. Core    - Embedded defaults (~/.engram/core/config.yaml)
  2. Company - Enterprise settings (/etc/engram/config.yaml)
  3. Team    - Project settings (.engram/config.yaml)
  4. User    - Personal settings (~/.engram/user/config.yaml)

Later tiers override earlier ones. Environment variables and CLI flags
take precedence over all configuration files.

COMMANDS
  show - Display effective configuration with sources

EXAMPLES
  # Show effective configuration
  $ engram config show

  # Show with JSON output
  $ engram config show --json

  # Show specific section
  $ engram config show telemetry

DOCUMENTATION
  See CONFIG-PRECEDENCE.md for detailed precedence rules and examples.`,
}

func init() {
	rootCmd.AddCommand(configCmd)
}
