package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/cli"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/config"
	"github.com/vbonnet/engram/core/pkg/cliutil"
)

var (
	cfg         *config.Config
	cfgFile     string
	sessionsDir string
	logLevel    string
	directory   string
)

var rootCmd = &cobra.Command{
	Use:   "csm",
	Short: "Claude Session Manager - Manage Claude AI sessions with ease",
	Long: `csm (Claude Session Manager) helps you manage Claude AI sessions
by providing easy session resumption, discovery, and health monitoring.

Resume sessions by tmux name, workspace ID, or Claude UUID:
  csm -C ~/project resume claude-1
  csm -C ~/project resume github.com-user-repo-main
  csm -C ~/project resume c86ffd41-cbcc-4bfa-8b1f-4da7c83fc3d2

Global Flags:
  -C, --directory <path>    Working directory (default: current directory)`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Resolve working directory from -C flag
		if directory != "" {
			result, err := cliutil.WorkdirFromFlags(directory, true)
			if err != nil {
				return fmt.Errorf("failed to resolve directory: %w", err)
			}
			cli.SetProjectDirectory(result.Resolved)
		} else {
			// Use current working directory if -C not specified
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}
			cli.SetProjectDirectory(cwd)
		}
		return nil
	},
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&directory, "directory", "C", "", "Working directory")
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.config/csm/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&sessionsDir, "sessions-dir", "", "sessions directory (default: ~/sessions)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "log level (debug, info, warn, error)")
}

func initConfig() {
	var err error
	cfg, err = config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Override with flags
	if sessionsDir != "" {
		cfg.SessionsDir = sessionsDir
	}
	if logLevel != "" {
		cfg.LogLevel = logLevel
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
