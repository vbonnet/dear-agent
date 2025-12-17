package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/cli"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/config"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/lock"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
)

var (
	cfg              *config.Config
	cfgFile          string
	sessionsDir      string
	logLevel         string
	directory        string
	timeout          time.Duration
	noLock           bool
	skipHealthCheck  bool
	globalLock       *lock.FileLock
	globalHealthCheck *tmux.HealthChecker
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
		// Load configuration first
		var err error
		cfg, err = loadConfigWithFlags()
		if err != nil {
			return err
		}

		// Set global timeout for tmux commands
		if cfg.Timeout.Enabled {
			tmux.SetTimeout(cfg.Timeout.TmuxCommands)
		}

		// Commands that don't need locks (read-only operations)
		lockFreeCommands := map[string]bool{
			"version": true,
			"list":    true,
			"doctor":  true,
			"unlock":  true,
			"backup":  true,
		}

		// Acquire lock if enabled and command requires it
		needsLock := !lockFreeCommands[cmd.Name()]
		if cfg.Lock.Enabled && !noLock && needsLock {
			globalLock, err = lock.New(cfg.Lock.Path)
			if err != nil {
				return err
			}
			if err := globalLock.TryLock(); err != nil {
				return err
			}
		}

		// Initialize health checker
		if cfg.HealthCheck.Enabled && !skipHealthCheck {
			globalHealthCheck = tmux.NewHealthChecker(
				cfg.HealthCheck.CacheDuration,
				cfg.HealthCheck.ProbeTimeout,
			)
		}

		// Resolve working directory from -C flag
		if directory != "" {
			absPath, err := filepath.Abs(directory)
			if err != nil {
				return fmt.Errorf("failed to resolve directory: %w", err)
			}
			cli.SetProjectDirectory(absPath)
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
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		// Release lock
		if globalLock != nil {
			return globalLock.Unlock()
		}
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&directory, "directory", "C", "", "Working directory")
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.config/csm/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&sessionsDir, "sessions-dir", "", "sessions directory (default: ~/sessions)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().DurationVar(&timeout, "timeout", 0, "tmux command timeout (overrides config)")
	rootCmd.PersistentFlags().BoolVar(&noLock, "no-lock", false, "skip lock acquisition (DANGEROUS)")
	rootCmd.PersistentFlags().BoolVar(&skipHealthCheck, "skip-health-check", false, "skip health check")
}

func loadConfigWithFlags() (*config.Config, error) {
	// Load config file or defaults
	configPath := cfgFile
	if configPath == "" {
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".config", "csm", "config.yaml")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}

	// Override with flags
	if sessionsDir != "" {
		cfg.SessionsDir = sessionsDir
	}
	if logLevel != "" {
		cfg.LogLevel = logLevel
	}
	if timeout > 0 {
		cfg.Timeout.TmuxCommands = timeout
	}
	if skipHealthCheck {
		cfg.HealthCheck.Enabled = false
	}

	return cfg, nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
