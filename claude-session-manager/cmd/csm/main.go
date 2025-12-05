package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/config"
)

var (
	cfg        *config.Config
	cfgFile    string
	sessionsDir string
	logLevel   string
)

var rootCmd = &cobra.Command{
	Use:   "csm",
	Short: "Claude Session Manager - Manage Claude AI sessions with ease",
	Long: `csm (Claude Session Manager) helps you manage Claude AI sessions
by providing easy session resumption, discovery, and health monitoring.

Resume sessions by tmux name, workspace ID, or Claude UUID:
  csm resume claude-1
  csm resume github.com-user-repo-main
  csm resume c86ffd41-cbcc-4bfa-8b1f-4da7c83fc3d2`,
}

func init() {
	cobra.OnInitialize(initConfig)

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
