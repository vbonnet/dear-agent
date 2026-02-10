package main

import (
	"github.com/spf13/cobra"
)

var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Administrative and maintenance commands",
	Long: `Admin commands handle system maintenance including health checks,
UUID fixing, session cleanup, lock management, and session discovery.

Examples:
  agm admin doctor  # Check system health
  agm admin fix     # Fix UUID associations
  agm admin clean   # Batch cleanup sessions
  agm admin unlock  # Remove stale locks
  agm admin sync    # Discover and sync sessions`,
}

func init() {
	rootCmd.AddCommand(adminCmd)
}
